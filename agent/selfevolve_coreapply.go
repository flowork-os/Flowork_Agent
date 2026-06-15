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
)

// evolveCoreApplier — rakit EvolveCoreApplier (di-inject ke handler). B1 = STAGE-only.
func evolveCoreApplier() agentmgr.EvolveCoreApplier {
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
		content, model, cerr := evolveCodegenFile(ctx, rel, p)
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

		// ── Diff buat STAGE (review owner) ──────────────────────────────────────────
		diff := evolveWorktreeDiff(ctx, wt, rel)
		return agentmgr.EvolveCoreResult{
			Staged: true, TargetFile: rel, Diff: diff, TestOutput: trimStr(testOut, 2000), Model: model,
			Note: "lolos test-gate",
		}, nil
	}
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
		b.WriteString("$ go " + strings.Join(s, " ") + "\n")
		b.WriteString(strings.TrimSpace(string(out)))
		b.WriteString("\n")
		if err != nil {
			b.WriteString("→ GAGAL: " + err.Error() + "\n")
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
func evolveCodegenFile(ctx context.Context, rel string, p agentdb.EvolveProposal) (content, model string, err error) {
	model = coderModel("")
	lang := "Go"
	if strings.HasSuffix(rel, ".js") {
		lang = "JavaScript"
	}
	sys := "Lo engineer " + lang + " Flowork. Tulis SATU file " + lang + " LENGKAP, idiomatik, dan PASTI compile " +
		"untuk path yang diminta. ATURAN KERAS: file BARU & ADDITIVE — JANGAN asumsikan ngedit/ngehapus file lain, " +
		"JANGAN bikin fungsi yg bentrok nama global. Ikuti gaya kode sekitar, komentar secukupnya. " +
		"Output HANYA isi file mentah (tanpa ``` fence, tanpa penjelasan)."
	user := "Path file baru: " + rel + "\nKind: " + p.Kind + "\nTujuan: " + p.Goal + "\nRationale: " + p.Rationale
	res, e := routerChat(ctx, model, []map[string]any{
		{"role": "system", "content": sys},
		{"role": "user", "content": user},
	}, nil, 4000)
	if e != nil {
		return "", model, e
	}
	return evolveStripFence(res.Content), model, nil
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
