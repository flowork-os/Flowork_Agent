// === LOCKED FILE (soft) ===
// Status: STABLE — DO NOT MODIFY without owner approval. (LOCKED ≠ FREEZE: boleh diedit dgn izin.)
// Owner: Aola Sahidin (Mr.Dev / awenk audico) · Locked at: 2026-06-16 (owner-approved sprint).
// Reason: R7 core-apply B1+B2+B3. VERIFIED E2E (dev, haiku strong): guard additive + error edukasi;
//   sandbox worktree→codegen→test-gate(build+vet, GOWORK=off, anti-RCE no `go test`)→STAGE/commit;
//   AUTO: re-probe model asli→commit lokal→auto-push (token KV). Self-edit+self-publish core OTONOM
//   berhasil (b0b37b7 ke local remote). Imun boot-rollback di watchdog (organ independen, survive
//   walau Flowork mati; jaga ROUTER+agent). Additive-only (papan abadi); edit-existing→STAGE/v2.
// 2026-06-27 (F3 roadmap-evolusi, owner-directed): + GUARD RUANG-SARAF — usulan kind core-edit
//   (fix/refactor/doc/test) ditolak di muka via NerveProposalVet (nerve_proposal_ext.go) → arahin
//   ke switch/data/modul / lapor butuh_tombol. Additive ~5 baris; evolusi mode OFF = nol efek live.
//   Build/vet/test/freeze PASS. Dok: lock/peta-saraf.md.
//
// selfevolve_coreapply.go — R7 SELF-EVOLUTION fase-2b: CORE-APPLY engine (🔴 DEV-only).
// Owner-approved 2026-06-16 (diskusi panjang: additive-only auto-push + edit-stage + error
// edukasi + boot-rollback). Sisi main: nyuntik kemampuan "ubah core" ke agentmgr.EvolveCoreApplyHandler.
//
// B1 (file ini, STAGE-only): proposal core → GUARD (cuma 'NEW:' file additive boleh; edit
// existing/LOCKED/delete = DITOLAK + error edukasi) → git-worktree sandbox → LLM codegen file
// baru → test-gate (build+vet+test di sandbox) → diff DISTAGE buat review owner. NOL commit.
// Sandbox selalu dibuang (rollback) di akhir — repo asli ga pernah disentuh di B1.
//
// Prinsip "papan abadi": evolusi core = NAMBAH organ (file baru), BUKAN bongkar papan yg berdiri.
// Mutasi letal (edit existing yg lolos test tapi ngerusak runtime) nyebar ke semua user lewat
// auto-update → additive-only = guard biar otonomi gak bunuh diri. Edit-existing dibuka di v2.

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"flowork-gui/internal/agentdb"
	"flowork-gui/internal/agentmgr"
	"flowork-gui/internal/kernel/loader"
	"flowork-gui/internal/kernelhost"
)

// evolveCoreApplier — rakit EvolveCoreApplier (di-inject ke handler). B1 = STAGE-only.
func evolveCoreApplier(host *kernelhost.Host) agentmgr.EvolveCoreApplier {
	return func(ctx context.Context, p agentdb.EvolveProposal, auto bool) (agentmgr.EvolveCoreResult, error) {
		var res agentmgr.EvolveCoreResult
		target := strings.TrimSpace(p.TargetFile)

		root, rerr := evolveRepoRoot()
		if rerr != nil {
			return res, fmt.Errorf("cari repo root: %w", rerr)
		}

		// ── GUARD additive-only (B1) ────────────────────────────────────────────────
		// Cuma 'NEW:<path>' (nambah file baru) yg boleh. Selain itu = aksi bahaya → error edukasi.
		blocked := func(detail string) agentmgr.EvolveCoreResult {
			return agentmgr.EvolveCoreResult{
				Blocked: true, TargetFile: target,
				Educational: agentmgr.EvolveEduWarning, Note: detail,
			}
		}
		// ── F3: KUNCI KE RUANG SARAF (peta-saraf.md) ────────────────────────────────
		// Usulan WAJIB salah satu saluran saraf: SWITCH (saklar GUI) / DATA (skill) / MODUL
		// (.fwpack). Kind edit-kode-inti (fix/refactor/doc/test) = di LUAR saraf → tolak +
		// arahin ke saklar/data/modul atau lapor butuh_tombol (F4). Inti beku ga pernah disentuh.
		if v := NerveProposalVet(p.Kind, target); !v.Allowed {
			// F4: AI mentok → LAPOR ke antrian owner (bukan bongkar inti). Owner nambah saklar.
			recordButuhTombol(target, p.Rationale, p.Kind, v.Channel)
			return blocked("Usulan di LUAR RUANG SARAF (channel=" + v.Channel + "). " + v.Reason +
				" — pakai saklar/data/modul, atau lapor butuh_tombol ke owner."), nil
		}
		if !strings.HasPrefix(target, "NEW:") {
			return blocked("Proposal ini MENGEDIT/menyentuh file core yang sudah ada (" + target + "). " +
				"B1 cuma boleh NAMBAH file baru (NEW:). Edit existing → STAGE manual / v2."), nil
		}
		rel := strings.TrimSpace(strings.TrimPrefix(target, "NEW:"))
		clean, ok := evolveSafeRepoPath(root, rel)
		if !ok {
			return blocked("Path target di luar repo / tidak valid (anti path-traversal): " + rel), nil
		}
		// Proposal target_file relatif ke SELF-MAP (= modul agent, mis "internal/agentdb/x.go"),
		// BUKAN repo-root. Resolve ke path repo-relatif yg bener (mis "agent/internal/agentdb/x.go")
		// biar file mendarat di package yg bener + test-gate nemu modulnya. (Bug ketauan dari E2E.)
		rel = evolveResolveTarget(root, clean)
		abs := filepath.Join(root, rel)
		if st, err := os.Stat(abs); err == nil && !st.IsDir() {
			return blocked("File " + rel + " SUDAH ADA — 'NEW' = nambah file baru, bukan menimpa yang aktif. " +
				"Menimpa file aktif bisa melukai diri sendiri + warga lain."), nil
		}
		if evolveFileLocked(abs) { // (file NEW belum ada; jaga-jaga kalau path nyamar)
			return blocked("Target menyentuh file LOCKED: " + rel), nil
		}

		// ── Sandbox git-worktree (repo asli TIDAK disentuh) ─────────────────────────
		wt, cleanup, werr := evolveWorktreeAdd(ctx, root)
		if werr != nil {
			return res, fmt.Errorf("buat worktree sandbox: %w", werr)
		}
		defer cleanup() // ROLLBACK: worktree remove + rm -rf, apapun yg terjadi

		// ── LLM codegen file baru ───────────────────────────────────────────────────
		content, model, cerr := evolveCodegenFile(ctx, host, rel, p)
		if cerr != nil {
			return res, fmt.Errorf("codegen: %w", cerr)
		}
		if strings.TrimSpace(content) == "" {
			return res, fmt.Errorf("codegen balik kosong")
		}
		if strings.HasSuffix(rel, ".go") && !strings.Contains(content, "package ") {
			return res, fmt.Errorf("codegen .go tanpa deklarasi package — ditolak")
		}

		// Tulis file ke SANDBOX (bukan repo asli).
		wtFile := filepath.Join(wt, rel)
		if err := os.MkdirAll(filepath.Dir(wtFile), 0o755); err != nil {
			return res, fmt.Errorf("mkdir sandbox: %w", err)
		}
		if err := os.WriteFile(wtFile, []byte(content), 0o644); err != nil {
			return res, fmt.Errorf("tulis file sandbox: %w", err)
		}

		// ── Test-gate (build+vet+test) di sandbox — HAKIM deterministik ──────────────
		testOut, pass := evolveTestGate(ctx, wt, rel)
		if !pass {
			// Codegen ngehasilin kode yg gak lolos → JANGAN distage. Sandbox dibuang (defer).
			// Ini juga pelajaran (kodemu belum bener) — handler catat via karma fail.
			return res, fmt.Errorf("test-gate GAGAL (kode belum lolos build/vet/test):\n%s", trimStr(testOut, 1500))
		}

		// ── Diff (dari sandbox) ─────────────────────────────────────────────────────
		diff := evolveWorktreeDiff(ctx, wt, rel)

		// ── AUTO (B2): re-probe model asli → commit LOKAL. Manual → STAGE (B1). ──────
		// Gate auto (dev+mode=auto+karma+model) udah dicek handler. Di sini lapis terakhir:
		// re-probe FRESH (anti failover-ke-lokal di tengah operasi) sebelum nyentuh main.
		if auto {
			strong, note := evolveReprobeStrong(ctx)
			if !strong {
				// Model jatuh lemah/lokal mid-operasi → JANGAN commit core. Turunin ke STAGE.
				return agentmgr.EvolveCoreResult{
					Staged: true, TargetFile: rel, Diff: diff, Content: content, TestOutput: trimStr(testOut, 2000), Model: model,
					Note: "re-probe model GAGAL (" + note + ") → diturunin ke STAGE, ga auto-commit core",
				}, nil
			}
			msg := "evolve(auto): tambah " + rel + " — " + trimStr(strings.TrimSpace(p.Rationale), 80)
			hash, cerr := evolveCommitFile(ctx, root, rel, content, msg)
			if cerr != nil {
				return res, fmt.Errorf("commit lokal gagal: %w", cerr)
			}
			// B3 AUTO-PUSH: kalau owner aktifin + token ada → self-publish ke upstream. Gagal
			// push ≠ fatal (commit lokal tetep ada; boot-rollback jaga sisi user). Token dari KV.
			pushed, pnote := evolveMaybePush(ctx, root)
			return agentmgr.EvolveCoreResult{
				Committed: true, Pushed: pushed, TargetFile: rel, Diff: diff,
				TestOutput: trimStr(testOut, 2000), Model: model,
				Note: "auto-commit LOKAL " + hash + " (re-probe OK). Push: " + pnote,
			}, nil
		}
		return agentmgr.EvolveCoreResult{
			Staged: true, TargetFile: rel, Diff: diff, Content: content, TestOutput: trimStr(testOut, 2000), Model: model,
			Note: "lolos test-gate",
		}, nil
	}
}

// evolveReprobeStrong — re-probe model asli FRESH (bypass cache capCache) tepat sebelum commit
// core. Anti failover-ke-lokal di tengah operasi: token cloud bisa habis SETELAH gate lolos →
// codegen jalan di model lemah → re-probe ini nangkep + batal commit. ~90s, tapi auto-commit langka.
func evolveReprobeStrong(ctx context.Context) (bool, string) {
	c, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	m := evoCoderModel() // probe model yg evo-coder BENERAN pakai (GUI per-agent), bukan default global
	r := runCapabilityEval(c, m)
	return r.Passed, m + ": " + r.Detail
}

// evolveCommitFile — tulis file baru ke REPO ASLI + git add + commit ke LOCAL main (NO push).
// Path-scoped (commit cuma rel) → file dirty lain di working tree ga ikut. Author = organisme.
// Balikin short-hash commit. B2: lokal doang; push = B3 (token setting page).
func evolveCommitFile(ctx context.Context, root, rel, content, msg string) (string, error) {
	abs := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", fmt.Errorf("mkdir: %w", err)
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("tulis: %w", err)
	}
	if out, err := exec.CommandContext(ctx, "git", "-C", root, "add", "--", rel).CombinedOutput(); err != nil {
		return "", fmt.Errorf("git add: %v: %s", err, trimStr(string(out), 200))
	}
	commit := exec.CommandContext(ctx, "git", "-C", root, "commit", "-m", msg, "--", rel)
	commit.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Flowork Evolusi", "GIT_AUTHOR_EMAIL=evolusi@flowork.local",
		"GIT_COMMITTER_NAME=Flowork Evolusi", "GIT_COMMITTER_EMAIL=evolusi@flowork.local")
	if out, err := commit.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git commit: %v: %s", err, trimStr(string(out), 200))
	}
	hash, _ := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "--short", "HEAD").Output()
	return strings.TrimSpace(string(hash)), nil
}

// evolveRepoRoot — root repo git (toplevel). Dari dir binary (portable, no-hardcode).
func evolveRepoRoot() (string, error) {
	base := ""
	if exe, err := os.Executable(); err == nil {
		base = filepath.Dir(exe)
	} else if wd, werr := os.Getwd(); werr == nil {
		base = wd
	}
	out, err := exec.Command("git", "-C", base, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse (base=%s): %w", base, err)
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", fmt.Errorf("repo root kosong")
	}
	return root, nil
}

// evolveSafeRepoPath — validasi rel ada DI DALAM root (anti path-traversal / absolute).
// Balikin path relatif ternormalisasi + ok.
func evolveSafeRepoPath(root, rel string) (string, bool) {
	rel = strings.TrimSpace(rel)
	if rel == "" || filepath.IsAbs(rel) || strings.Contains(rel, "..") || strings.ContainsRune(rel, 0) {
		return "", false // null-byte (C1): tolak biar ga nyelip lewat cek string
	}
	clean := filepath.Clean(rel)
	if clean == "." || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) || strings.HasPrefix(clean, "-") {
		return "", false // leading-dash (B1): cegah ke-treat sbg git flag kalau caller masa depan lupa "--"
	}
	// Pastikan join masih di bawah root.
	abs := filepath.Join(root, clean)
	rootAbs, _ := filepath.Abs(root)
	if !strings.HasPrefix(abs, rootAbs+string(os.PathSeparator)) {
		return "", false
	}
	return clean, true
}

// evolveResolveTarget — proposal target relatif self-map (= modul agent) → path REPO-relatif
// yg bener. Cari modul (top-level dir ber-go.mod) yg folder induk rel-nya udah ada (mis
// "internal/agentdb" ada di bawah "agent/" → "agent/internal/agentdb/x.go"). Portable: scan
// dir, no-hardcode nama modul. Ga ketemu → balikin as-is (folder bener-bener baru).
func evolveResolveTarget(root, rel string) string {
	isDir := func(p string) bool { st, err := os.Stat(p); return err == nil && st.IsDir() }
	parent := filepath.Dir(rel)
	// Udah bener repo-relatif? (folder induk ada langsung di root, atau file top-level.)
	if parent == "." || isDir(filepath.Join(root, parent)) {
		return rel
	}
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m := e.Name()
		if _, err := os.Stat(filepath.Join(root, m, "go.mod")); err != nil {
			continue
		}
		if isDir(filepath.Join(root, m, parent)) {
			return filepath.Join(m, rel)
		}
	}
	return rel
}

// evolveFileLocked — file bertanda LOCKED (soft/hard) di header? Jangan disentuh self-evolution.
func evolveFileLocked(abs string) bool {
	f, err := os.Open(abs)
	if err != nil {
		return false // file ga ada (NEW) = ga locked
	}
	defer f.Close()
	buf := make([]byte, 600)
	n, _ := f.Read(buf)
	head := string(buf[:n])
	return strings.Contains(head, "LOCKED FILE") || strings.Contains(head, "=== LOCKED")
}

// evolveWorktreeAdd — bikin git-worktree detached di temp dir. Balikin (path, cleanup, err).
// cleanup = ROLLBACK: worktree remove --force + rm. Idempoten, aman dipanggil via defer.
func evolveWorktreeAdd(ctx context.Context, root string) (string, func(), error) {
	tmp, err := os.MkdirTemp("", "flowork-evolve-wt-")
	if err != nil {
		return "", func() {}, err
	}
	// worktree add --detach <tmp> HEAD — snapshot HEAD, ga ganggu branch/main.
	cmd := exec.CommandContext(ctx, "git", "-C", root, "worktree", "add", "--detach", tmp, "HEAD")
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(tmp)
		return "", func() {}, fmt.Errorf("worktree add: %v: %s", err, trimStr(string(out), 300))
	}
	cleanup := func() {
		// Pakai context.Background: cleanup harus jalan walau ctx request udah cancel/timeout.
		rm := exec.Command("git", "-C", root, "worktree", "remove", "--force", tmp)
		_, _ = rm.CombinedOutput()
		_ = os.RemoveAll(tmp)
		_ = exec.Command("git", "-C", root, "worktree", "prune").Run()
	}
	return tmp, cleanup, nil
}

// evolveModuleDir — dir modul Go (nearest go.mod ancestor) buat rel di dalam wtRoot. "" kalau ga ada.
func evolveModuleDir(wtRoot, rel string) string {
	dir := filepath.Dir(filepath.Join(wtRoot, rel))
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir || len(parent) < len(wtRoot) {
			return ""
		}
		dir = parent
	}
}

// evolveTestGate — build+vet di modul yg memuat rel (di sandbox). Balikin (output, lolos).
// Non-Go (mis. .js) → ga ada modul → skip gate Go (cuma cek file non-kosong di caller).
//
// 🔒 KEAMANAN (security review CRITICAL A1): SENGAJA gak jalanin `go test`. `go test` MENGEKSEKUSI
// kode LLM (init()/TestMain/TestXxx) sebagai proses server = RCE kalau model nulis/keinjeksi kode
// jahat. `go build` + `go vet` dua-duanya MENG-COMPILE (vet ikut compile file _test.go buat analisa)
// TANPA mengeksekusi apa-apa → tetep nangkep error tipe/import/symbol. Verifikasi runtime (jalanin
// test) dilakukan SETELAH owner approve staged diff (Milestone C) + boot-rollback sbg jaring akhir.
// Auto-mode (B2+) yg pengen jalanin test butuh sandbox OS-level (user/namespace terpisah) — future.
func evolveTestGate(ctx context.Context, wtRoot, rel string) (string, bool) {
	mod := evolveModuleDir(wtRoot, rel)
	if mod == "" {
		return "(non-Go / no go.mod — test-gate Go di-skip; file additive)", true
	}
	steps := [][]string{
		{"build", "./..."},
		{"vet", "./..."},
	}
	var b strings.Builder
	for _, s := range steps {
		c, cancel := context.WithTimeout(ctx, 8*time.Minute)
		cmd := exec.CommandContext(c, "go", s...)
		cmd.Dir = mod
		// GOWORK=off: build modul worktree STANDALONE (pakai go.mod-nya sendiri), bukan go.work
		// repo utama → isolasi sandbox murni. Agent module ga punya `replace` → build standalone OK.
		cmd.Env = append(os.Environ(), "GOWORK=off")
		out, err := cmd.CombinedOutput()
		cancel()
		fmt.Fprintf(&b, "$ go %s\n", strings.Join(s, " "))
		b.WriteString(strings.TrimSpace(string(out)))
		b.WriteString("\n")
		if err != nil {
			fmt.Fprintf(&b, "→ GAGAL: %s\n", err.Error())
			return b.String(), false
		}
		b.WriteString("→ OK\n")
	}
	return b.String(), true
}

// evolveWorktreeDiff — diff file baru di sandbox (intent-to-add biar untracked muncul di diff).
func evolveWorktreeDiff(ctx context.Context, wtRoot, rel string) string {
	_ = exec.CommandContext(ctx, "git", "-C", wtRoot, "add", "-N", "--", rel).Run()
	out, _ := exec.CommandContext(ctx, "git", "-C", wtRoot, "diff", "--", rel).CombinedOutput()
	return string(out)
}

// evolveCodegenFile — 1 LLM call: tulis ISI file baru (idiomatik, compile) dari proposal.
// routerChat (bukan forced-tool: kita mau raw file content) → strip fence kalau ada.
// evoCoderModel — model PER-AGENT evo-coder dari GUI Settings (kv router_model). Ini yg BENER
// dilaporin & di-probe: codegen jalan lewat AGENT evo-coder yg pakai model GUI-nya sendiri
// (mis. opus), BUKAN default global. Owner 2026-06-20: kebenaran model = GUI per-agent.
// Fallback coderModel("") (= GUI default global) kalau evo-coder belum di-set.
func evoCoderModel() string {
	// PATH BENER: agent store = <AgentsDir>/evo-coder.fwagent/workspace/state.db (sama kayak
	// host openAgentStore → Resolve(id, agentFolder)). Resolve(id,"") SALAH (ga nemu .fwagent →
	// fallback path kosong → store kosong → ke-report flowork-brain padahal GUI opus).
	dir := filepath.Join(loader.AgentsDir(), "evo-coder.fwagent")
	if st, e := agentdb.Open(agentdb.Resolve("evo-coder", dir)); e == nil {
		defer st.Close()
		if m := st.GetRouterModel(); m != "" {
			return m
		}
	}
	return coderModel("")
}

// evolveReviewBackstop — BUKAN batas kualitas, cuma rem anti-runaway (owner 2026-06-20: "evolusi
// jangan dibatasin, nanti mati di tengah"). Stop NORMAL = reviewer 'LOLOS' (konvergen) atau
// no-progress (temuan sama 2x = coder mentok). Backstop tinggi cuma jaga kalau reviewer rewel
// ga pernah LOLOS biar token ga kebakar tak hingga. GA ada wall-clock — loop hidup selama progress.
const evolveReviewBackstop = 30

// evolveNorm — normalisasi teks temuan buat banding antar-ronde (deteksi mentok). Lowercase +
// rapatkan whitespace biar beda wording minor ga dianggap "progress" palsu.
func evolveNorm(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// Budget waktu loop codegen+review dipasok ctx dari handler (async, background) — lihat
// agentmgr.evolveCoreApplyBudget. Loop di sini cuma hormati ctx (ga bikin timeout sendiri).

// evolveCoderGen — 1x invoke evo-coder → kode bersih (fence dibuang). ok=false kalau error/invalid.
func evolveCoderGen(ctx context.Context, host *kernelhost.Host, spec, lang string) (string, bool) {
	raw, e := host.InvokeAgentMessage(ctx, "evo-coder", spec, "evolve-coder")
	if e != nil {
		return "", false
	}
	code := evolveStripFence(strings.TrimSpace(extractReply(raw)))
	ok := code != "" && (lang == "JavaScript" || strings.Contains(code, "package "))
	return code, ok
}

// evolveReviewCode — evo-reviewer audit kode (keamanan korpus hacking + kualitas). Balik:
//   "lolos"  → reviewer nyatakan bersih & aman.
//   "temuan" + findings → ada celah/cacat, harus di-fix.
//   ""       → reviewer error/ambigu → caller JANGAN blok (test-gate + stage manusia jaga).
func evolveReviewCode(ctx context.Context, host *kernelhost.Host, spec, code string) (string, string) {
	prompt := "SPEC:\n" + spec + "\n\n=== KODE HASIL CODER (AUDIT keamanan korpus-hacking + kualitas) ===\n" + code
	raw, e := host.InvokeAgentMessage(ctx, "evo-reviewer", prompt, "evolve-reviewer")
	if e != nil {
		return "", ""
	}
	reply := strings.TrimSpace(extractReply(raw))
	up := strings.ToUpper(reply)
	if strings.Contains(up, "TEMUAN") {
		return "temuan", reply
	}
	if strings.Contains(up, "LOLOS") {
		return "lolos", ""
	}
	return "", "" // ambigu → ga blok
}

func evolveCodegenFile(ctx context.Context, host *kernelhost.Host, rel string, p agentdb.EvolveProposal) (content, model string, err error) {
	model = evoCoderModel()
	lang := "Go"
	if strings.HasSuffix(rel, ".js") {
		lang = "JavaScript"
	}
	spec := "Path file baru: " + rel + "\nBahasa: " + lang + "\nKind: " + p.Kind + "\nTujuan: " + p.Goal +
		"\nRationale: " + p.Rationale + "\n\nTulis SATU file " + lang + " LENGKAP, idiomatik, PASTI compile, " +
		"ADDITIVE (file BARU; JANGAN edit/hapus/LOCKED; no fungsi bentrok global). Output HANYA isi file mentah."

	// OPSI A + LOOP REVIEWER↔FIXER (owner 2026-06-20, konsep Looper): evo-coder GENERATE →
	// evo-reviewer AUDIT (keamanan korpus 800rb hacking + kualitas/integrasi/schema) → kalau
	// ada TEMUAN, balik ke coder buat FIX → ulang sampai 'LOLOS' atau mentok round. Nutup celah
	// test-gate (build+vet ga nangkep cacat runtime/keamanan). Reviewer error → ga blok (andelin
	// test-gate + stage review manusia). Harness yg apply ke sandbox (agent ga pegang fs repo).
	if host != nil {
		code, ok := evolveCoderGen(ctx, host, spec, lang)
		if ok {
			label := model + " (evo-coder)"
			prevFindings, noProgress, fixes := "", 0, 0
			for round := 0; round < evolveReviewBackstop; round++ {
				verdict, findings := evolveReviewCode(ctx, host, spec, code)
				if verdict == "lolos" {
					return code, fmt.Sprintf("%s +review✓(%d fix)", label, fixes), nil
				}
				if verdict != "temuan" {
					break // reviewer error/ambigu → jangan blok evolusi (test-gate + manusia jaga)
				}
				// NO-PROGRESS: temuan sama persis kayak ronde lalu = coder mentok ga bisa fix.
				// Stop 2x berturut biar ga muter (bukan wall-clock; loop hidup selama ADA progress).
				if n := evolveNorm(findings); n == prevFindings {
					if noProgress++; noProgress >= 2 {
						return code, fmt.Sprintf("%s +review✗(mentok temuan sama, %d fix)", label, fixes), nil
					}
				} else {
					noProgress, prevFindings = 0, n
				}
				fixSpec := spec + "\n\n=== TEMUAN REVIEWER (PERBAIKI SEMUA, output ULANG file LENGKAP) ===\n" +
					findings + "\n\n=== KODE SEKARANG ===\n" + code
				fixed, ok2 := evolveCoderGen(ctx, host, fixSpec, lang)
				if !ok2 {
					break
				}
				code, fixes = fixed, fixes+1
			}
			return code, fmt.Sprintf("%s +review✗(backstop %d, %d fix)", label, evolveReviewBackstop, fixes), nil
		}
	}

	// Fallback: codegen hardcoded (routerChat) — sama kayak sebelumnya.
	sys := "Lo engineer " + lang + " Flowork. Tulis SATU file " + lang + " LENGKAP, idiomatik, dan PASTI compile " +
		"untuk path yang diminta. ATURAN KERAS: file BARU & ADDITIVE — JANGAN asumsikan ngedit/ngehapus file lain, " +
		"JANGAN bikin fungsi yg bentrok nama global. Ikuti gaya kode sekitar, komentar secukupnya. " +
		"Output HANYA isi file mentah (tanpa ``` fence, tanpa penjelasan)."
	res, e := routerChatSafe(ctx, model, []map[string]any{
		{"role": "system", "content": sys},
		{"role": "user", "content": spec},
	}, nil, 4000)
	if e != nil {
		return "", model, e
	}
	return evolveStripFence(res.Content), model + " (fallback)", nil
}

// evolveStripFence — buang ```lang ... ``` kalau model bandel ngebungkus.
func evolveStripFence(s string) string {
	t := strings.TrimSpace(s)
	if !strings.HasPrefix(t, "```") {
		return t
	}
	if i := strings.IndexByte(t, '\n'); i >= 0 {
		t = t[i+1:]
	}
	if j := strings.LastIndex(t, "```"); j >= 0 {
		t = t[:j]
	}
	return strings.TrimSpace(t)
}
