// cmdsem.go — command SEMANTICS classifier (P1, tool_standard.md).
//
// The locked `bash` tool (shell.go) blocks danger by SUBSTRING match, which both
// false-negatives (`rm  -rf  /`, `${IFS}`, env indirection) and false-positives
// (`echo "rm -rf /"`). This classifies a command by STRUCTURE instead: normalize →
// split into simple commands → tokenize → judge the program + its args.
//
// Deliberately LEAN: WASM + rlimit + the cap gate already contain the blast radius,
// so this targets the high-severity, irreversible operations substring matching
// leaks on — not every conceivable misuse. New file (shell.go is owner-LOCKED:
// "tambah file baru, JANGAN modify ini"). Pure + offline → unit-tested in cmdsem_test.go.
package builtins

import (
	"regexp"
	"strings"
)

// readOnlyProg — programs that only read/observe; a command made only of these is
// classified read-only (feeds P2 permission tiering too).
var readOnlyProg = map[string]bool{
	"ls": true, "cat": true, "head": true, "tail": true, "grep": true, "egrep": true,
	"fgrep": true, "find": true, "echo": true, "printf": true, "pwd": true, "stat": true,
	"file": true, "wc": true, "which": true, "whoami": true, "id": true, "date": true,
	"env": true, "printenv": true, "ps": true, "df": true, "du": true, "uname": true,
	"hostname": true, "uptime": true, "free": true, "sort": true, "uniq": true, "cut": true,
	"awk": true, "sed": true, "diff": true, "cmp": true, "basename": true, "dirname": true,
	"realpath": true, "readlink": true, "tree": true, "less": true, "more": true, "tac": true,
	"sha256sum": true, "md5sum": true, "test": true, "true": true, "false": true, "stty": true,
	"git": true, // git is mostly read in agent use; mutations still go through the cap gate
}

var ifsRe = regexp.MustCompile(`\$\{?IFS\}?`)
var spaceRe = regexp.MustCompile(`\s+`)

// isForkBomb detects the self-referential function-bomb shape on the whitespace-
// stripped line (checked BEFORE splitting on operators, which would shred it):
// a function body that pipes into itself and backgrounds, e.g. :(){ :|:& };:
func isForkBomb(norm string) bool {
	s := strings.ReplaceAll(norm, " ", "")
	return strings.Contains(s, "(){") && strings.Contains(s, "|") &&
		strings.Contains(s, "&") && strings.Contains(s, "};")
}

// normalizeCmd defuses the cheap evasions: ${IFS}/$IFS → space, collapse runs of
// whitespace, trim. (Case preserved; program comparison lowercases.)
func normalizeCmd(s string) string {
	s = ifsRe.ReplaceAllString(s, " ")
	s = spaceRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// splitSegments breaks a line into simple commands on the shell control operators
// (; | & && || newline), so each program + its args is judged on its own.
func splitSegments(s string) []string {
	repl := s
	for _, op := range []string{"&&", "||", "|", ";", "\n", "&"} {
		repl = strings.ReplaceAll(repl, op, "\x00")
	}
	parts := strings.Split(repl, "\x00")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// tokenize splits a simple command on spaces, stripping matching quotes. Good enough
// to read the program + flag/target shape (not a full shell parser).
func tokenize(seg string) []string {
	raw := strings.Fields(seg)
	out := make([]string, 0, len(raw))
	for _, t := range raw {
		t = strings.Trim(t, `"'`)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func baseName(p string) string {
	if i := strings.LastIndexAny(p, "/\\"); i >= 0 {
		p = p[i+1:]
	}
	return strings.ToLower(p)
}

// dangerousRoots — targets that make a recursive delete catastrophic.
func dangerousTarget(t string) bool {
	t = strings.Trim(t, `"'`)
	switch t {
	case "/", "/*", "~", "~/", "$HOME", "${HOME}", "*", ".", "..", "/.":
		return true
	}
	// absolute system roots
	for _, r := range []string{"/etc", "/usr", "/bin", "/boot", "/lib", "/var", "/sys", "/dev", "/root", "/home"} {
		if t == r || t == r+"/" || t == r+"/*" {
			return true
		}
	}
	return false
}

func hasRecursiveForce(args []string) bool {
	rec := false
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			continue
		}
		la := strings.ToLower(a)
		if la == "--recursive" || la == "--force" {
			if la == "--recursive" {
				rec = true
			}
			continue
		}
		// combined short flags like -rf / -fr / -Rf
		if strings.ContainsAny(la, "r") && strings.HasPrefix(la, "-") && !strings.HasPrefix(la, "--") {
			rec = true
		}
	}
	return rec
}

// dangerous judges one simple command (program + args + raw segment).
func dangerous(prog string, args []string, seg string) (bool, string) {
	if strings.HasPrefix(prog, "mkfs") {
		return true, "filesystem format (" + prog + ") is destructive"
	}
	switch prog {
	case "sudo", "su", "doas", "pkexec":
		return true, "privilege escalation (" + prog + ") is not allowed"
	case "shutdown", "reboot", "halt", "poweroff":
		return true, "power control must use the system_power tool, not bash"
	case "init", "telinit":
		for _, a := range args {
			if a == "0" || a == "6" {
				return true, "runlevel change (init " + a + ") — use system_power"
			}
		}
	case "rm":
		if hasRecursiveForce(args) {
			for _, a := range args {
				if strings.HasPrefix(a, "-") {
					continue
				}
				if dangerousTarget(a) {
					return true, "recursive delete of a system/home root (" + a + ")"
				}
			}
		}
	case "dd":
		for _, a := range args {
			if strings.HasPrefix(strings.ToLower(a), "of=/dev/") {
				return true, "dd writing to a raw device (" + a + ")"
			}
		}
	case "mkfs", "fdisk", "parted", "wipefs":
		return true, "filesystem/partition operation (" + prog + ") is destructive"
	case "chmod":
		for _, a := range args {
			if a == "777" || a == "-R" && contains(args, "777") {
				return true, "chmod 777 / recursive permission wipe"
			}
		}
		if contains(args, "-R") && contains(args, "777") {
			return true, "recursive chmod 777"
		}
	case "chown":
		if contains(args, "-R") {
			for _, a := range args {
				if dangerousTarget(a) {
					return true, "recursive chown of a system root"
				}
			}
		}
	}
	// redirect to a raw block device: "> /dev/sdX"
	low := strings.ToLower(seg)
	if redirectToDevice.MatchString(low) {
		return true, "redirecting output onto a raw block device"
	}
	// reading private keys / shadow (exfiltration risk)
	if strings.Contains(seg, "id_rsa") || strings.Contains(low, "/etc/shadow") {
		return true, "accessing private keys / shadow file"
	}
	return false, ""
}

var redirectToDevice = regexp.MustCompile(`>\s*/dev/(sd|nvme|hd|disk|mmcblk)`)

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

// pipeToShell detects "<download> | sh|bash" — piping a fetched script straight into
// a shell (the classic curl|sh foot-gun); the agent should webfetch + inspect first.
func pipeToShell(norm string) bool {
	if !strings.Contains(norm, "|") {
		return false
	}
	segs := strings.Split(norm, "|")
	for i := 1; i < len(segs); i++ {
		toks := tokenize(strings.TrimSpace(segs[i]))
		if len(toks) == 0 {
			continue
		}
		switch baseName(toks[0]) {
		case "sh", "bash", "zsh", "dash", "ksh":
			// only flag when the upstream is a network fetch
			up := strings.ToLower(segs[i-1])
			if strings.Contains(up, "curl") || strings.Contains(up, "wget") || strings.Contains(up, "fetch") {
				return true
			}
		}
	}
	return false
}

// classifyCommand is the entry point: (blocked, reason, readOnly).
func classifyCommand(raw string) (bool, string, bool) {
	norm := normalizeCmd(raw)
	if norm == "" {
		return false, "", true
	}
	if isForkBomb(norm) {
		return true, "fork bomb", false
	}
	if pipeToShell(norm) {
		return true, "pipe a network download straight into a shell — webfetch + inspect first", false
	}
	readOnly := true
	for _, seg := range splitSegments(norm) {
		toks := tokenize(seg)
		if len(toks) == 0 {
			continue
		}
		prog := baseName(toks[0])
		if !readOnlyProg[prog] || strings.Contains(seg, ">") {
			readOnly = false // unknown program, or a write redirect
		}
		if blocked, reason := dangerous(prog, toks[1:], seg); blocked {
			return true, reason, false
		}
	}
	return false, "", readOnly
}
