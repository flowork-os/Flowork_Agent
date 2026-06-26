// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package scanner

import (
	"regexp"
	"strings"
)

func init() {
	Auditors["timezone_load_auditor"] = AuditTimezoneLoad
	Auditors["init_order_auditor"] = AuditInitOrder
	Auditors["panic_log_auditor"] = AuditPanicLog
	Auditors["panic_runtime_auditor"] = AuditPanicRuntime
	Auditors["shell_pipe_auditor"] = AuditShellPipe
	Auditors["command_injection_pipe_auditor"] = AuditCommandInjectionPipe
	Auditors["embed_directory_auditor"] = AuditEmbedDirectory
	Auditors["wasm_unsafe_export_auditor"] = AuditWASMUnsafeExport
	Auditors["network_print_auditor"] = AuditNetworkPrint
	Auditors["struct_pack_align_auditor"] = AuditStructPackAlign
}

var loadLocationRE = regexp.MustCompile(`time\.LoadLocation\s*\(\s*"`)

func AuditTimezoneLoad(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if loadLocationRE.MatchString(line) && !strings.Contains(line, "UTC") {
			out = append(out, Finding{
				Auditor:     "timezone_load_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "time.LoadLocation hardcoded zone — fails on systems w/o tzdata",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "embed `_ \"time/tzdata\"` di main, atau prefer time.UTC + offset manual",
			})
		}
	}
	return out
}

func AuditInitOrder(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}

	count := strings.Count(content, "func init()")
	if count > 2 {
		return []Finding{{
			Auditor:     "init_order_auditor",
			Severity:    SevLow,
			FilePath:    filePath,
			LineNumber:  1,
			Message:     "file punya " + intToStr(count) + " init() — order tergantung deklarasi, susah dibaca",
			Snippet:     "",
			Remediation: "consolidate ke 1 init(), atau split ke file terpisah dengan tujuan jelas",
		}}
	}
	return nil
}

var panicLogRE = regexp.MustCompile(`(log\.Fatal|log\.Panic)\s*\(`)

func AuditPanicLog(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if panicLogRE.MatchString(line) {

			if strings.Contains(content, "func main") && i < 50 {
				continue
			}
			out = append(out, Finding{
				Auditor:     "panic_log_auditor",
				Severity:    SevMedium,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "log.Fatal/Panic di library code — calls os.Exit, bypass defer cleanup",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "return error ke caller. log.Fatal ONLY di main()",
			})
		}
	}
	return out
}

var panicCallRE = regexp.MustCompile(`^\s*panic\s*\(`)

func AuditPanicRuntime(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if panicCallRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "panic_runtime_auditor",
				Severity:    SevMedium,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "explicit panic() — crash binary, hindari di production code path",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "return error ke caller. Panic acceptable cuma untuk impossible state (programming bug)",
			})
		}
	}
	return out
}

var shellPipeRE = regexp.MustCompile(`exec\.Command\s*\(\s*"(sh|bash)"\s*,\s*"-c"\s*,`)

func AuditShellPipe(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if shellPipeRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "shell_pipe_auditor",
				Severity:    SevHigh,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "exec via shell -c — shell interpret special chars, injection risk",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "pakai exec.Command direct (no shell): `exec.Command(\"ls\", \"-l\")`",
			})
		}
	}
	return out
}

var cmdInjectPipeRE = regexp.MustCompile(`\|\s*\$\(|\$\(\s*\w+\s*\|`)

func AuditCommandInjectionPipe(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if cmdInjectPipeRE.MatchString(line) && !strings.HasPrefix(strings.TrimSpace(line), "//") {
			out = append(out, Finding{
				Auditor:     "command_injection_pipe_auditor",
				Severity:    SevHigh,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "shell command substitution via pipe — user input executed as command",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "split ke exec.Command + io.Pipe untuk control eksplisit",
			})
		}
	}
	return out
}

var embedDirRE = regexp.MustCompile(`^//go:embed\s+\S+/\*`)

func AuditEmbedDirectory(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if embedDirRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "embed_directory_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "//go:embed <dir>/* — semua file di dir ke-embed (binary size grow)",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "kalau hanya butuh subset, list eksplisit `//go:embed file1 file2 ...` atau pakai gitignore di dir",
			})
		}
	}
	return out
}

var wasmExportRE = regexp.MustCompile(`//go:wasmexport`)

func AuditWASMUnsafeExport(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if wasmExportRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "wasm_unsafe_export_auditor",
				Severity:    SevMedium,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "//go:wasmexport — TinyGo panic setelah _start exit. Hindari kalau ngga butuh",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "pakai command pattern (per-call instantiate) yang Flowork pakai, bukan wasmexport",
			})
		}
	}
	return out
}

var netPrintRE = regexp.MustCompile(`fmt\.(Print|Println|Printf).*os\.Stdout`)

func AuditNetworkPrint(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}

	return nil
}

func AuditStructPackAlign(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}

	return nil
}
