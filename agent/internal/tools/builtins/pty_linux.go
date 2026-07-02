//go:build linux

// pty_linux.go — SIBLING ext (⚠️ FROZEN 2026-07-02 seizin owner — stabil+live): buka /dev/ptmx + start proses
// di bawah PTY (Linux). Pakai golang.org/x/sys/unix (udah dep transitif — NOL modul
// baru). Bagian platform-spesifik dari pty_session.go. 📄 Dok: lock/pty-exec.md
package builtins

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func startPTYSession(id, command, workdir string) (*ptySession, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, fmt.Errorf("buka /dev/ptmx: %w", err)
	}
	// unlockpt (TIOCSPTLCK = 0) + ptsname (TIOCGPTN → N → /dev/pts/N).
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		_ = master.Close()
		return nil, fmt.Errorf("unlockpt: %w", err)
	}
	n, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		_ = master.Close()
		return nil, fmt.Errorf("ptsname: %w", err)
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		_ = master.Close()
		return nil, fmt.Errorf("buka slave pts: %w", err)
	}

	argv := []string{"/bin/sh", "-i"} // kosong = shell interaktif
	if command != "" {
		argv = []string{"/bin/sh", "-c", command}
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	if workdir != "" {
		cmd.Dir = workdir
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	// Setsid + Setctty: proses jadi session leader dgn slave sbg controlling TTY.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		_ = slave.Close()
		_ = master.Close()
		return nil, fmt.Errorf("start proses: %w", err)
	}
	_ = slave.Close() // parent ga butuh slave; child udah pegang

	s := &ptySession{id: id, cmd: cmd, master: master, lastUse: time.Now()}
	// Reader: pompa output master → buffer sampai proses exit (EOF).
	go func() {
		buf := make([]byte, 4096)
		for {
			nr, er := master.Read(buf)
			if nr > 0 {
				s.appendOutput(buf[:nr])
			}
			if er != nil {
				s.mu.Lock()
				s.done = true
				s.mu.Unlock()
				_ = cmd.Wait()
				return
			}
		}
	}()
	return s, nil
}
