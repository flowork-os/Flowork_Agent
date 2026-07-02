# pty-exec — exec INTERAKTIF lewat pseudo-terminal

Tool set `pty_start` / `pty_send` / `pty_read` / `pty_close`: agent bisa jalanin
program yang butuh terminal beneran (REPL python/sqlite, TUI, shell interaktif) —
start sesi, ketik input, baca output berkelanjutan, tutup. Roadmap "buka lock".

## Arsitektur
- File (semua SIBLING deletable NON-frozen, NOL sentuh builtins.go frozen):
  - `agent/internal/tools/builtins/pty_session.go` — 4 tool + registry sesi + reaper.
  - `pty_linux.go` (//go:build linux) — buka /dev/ptmx via `golang.org/x/sys/unix`
    (TIOCSPTLCK unlockpt + TIOCGPTN ptsname) → start `/bin/sh -c cmd` (atau `-i`)
    dgn slave sbg controlling TTY (Setsid+Setctty). Reader goroutine pompa output.
  - `pty_other.go` (//go:build !linux) — stub error sopan (PTY cuma Linux).
- **Dependency: NOL modul baru.** `x/sys` udah dep transitif (v0.45.0) → dipromosiin
  jadi direct (go mod tidy, go.sum nol berubah). Sengaja BUKAN creack/pty (nambah
  modul = risiko gagal rebuild pas auto-update user offline, §7.2).

## Keamanan (§8.2)
- **Default OFF** — switch `FLOWORK_PTY=1` buat nyalain (tool ga ke-register kalau OFF).
  Powerful primitive (shell persisten) → opt-in sadar, GUI = kebenaran.
- Perintah start + TIAP `pty_send` di-guard `classifyCommand` (shell_guard) — rm -rf
  system/home, fork bomb, curl|sh, dst DIBLOK sama kayak tool `shell`.
- Kerja dikurung `cmd.Dir = workspace` agent (FromSharedDir). Capability `exec:shell`.
- Batas: max 8 sesi barengan, idle > 10 menit → auto-reap, output ring 256KB/sesi.

## Sesi
- `pty_start(command?, wait_ms?)` → {session_id, output, done}. command kosong = shell interaktif.
- `pty_send(session_id, input, newline?, wait_ms?)` → {output, done}. Enter otomatis kecuali newline=false.
- `pty_read(session_id, wait_ms?)` → {output, done}. Ambil output numpuk (proses yg terus nyetak).
- `pty_close(session_id)` → {closed, final_output}. WAJIB tutup sesi yg ga dipake.

## Switch / freeze
- `FLOWORK_PTY` (default OFF). Status file: FROZEN 2026-07-02 (seizin owner, teruji live).

## QC
build agent+router (linux) + cross-build windows (stub) hijau · vet hijau · unit test
(start+send+read+close roundtrip via `cat`, shell interaktif eksekusi `echo`, guard blok
`rm -rf /` di start & send, sesi-ga-ada error) PASS · full builtins test nol regresi ·
TestKernelFreeze ga kesenggol.
