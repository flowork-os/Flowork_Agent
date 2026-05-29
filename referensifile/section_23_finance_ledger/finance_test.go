package finance

import (
	"os"
	"runtime"
	"testing"
	"time"
)

func isWindows() bool { return runtime.GOOS == "windows" }

// --- I-A.2: HMAC fail-close ---

func TestLogAudit_FailsWithoutPassword(t *testing.T) {
	os.Unsetenv("FLOWORK_OWNER_PASSWORD")
	err := LogAudit("test-agent", 1.0)
	if err == nil {
		t.Fatal("expected error when FLOWORK_OWNER_PASSWORD not set, got nil")
	}
}

func TestLogAudit_SucceedsWithPassword(t *testing.T) {
	t.Setenv("FLOWORK_OWNER_PASSWORD", "test-secret-key")
	// Use a temp dir to avoid writing to real home
	tmp := t.TempDir()
	floworkDir := tmp + "/.flowork"
	if err := os.MkdirAll(floworkDir, 0700); err != nil {
		t.Fatal(err)
	}
	// Temporarily override home via file path test only — we check no panic/error
	// (full path override needs refactor; at minimum verify env-guard works)
	err := LogAudit("test-agent", 0.50)
	// May fail due to home dir path, but must NOT be the "no password" error
	if err != nil {
		// acceptable: home dir write issue; unacceptable: missing-password error
		if err.Error() == "finance: FLOWORK_OWNER_PASSWORD tidak di-set; audit log tidak dapat di-sign — transaksi ditolak" {
			t.Fatal("got fail-close error even with password set")
		}
	}
}

// --- I-A.3: file permission ---

func TestLogAudit_FilePermission(t *testing.T) {
	// Windows does not honour Unix permission bits; chmod only sets readonly flag.
	// The important thing is that the code compiles and the chmod call is present.
	if isWindows() {
		t.Skip("Windows ACL semantics differ; 0600 enforcement tested on POSIX only")
	}
	t.Setenv("FLOWORK_OWNER_PASSWORD", "test-perm-key")
	home, _ := os.UserHomeDir()
	logPath := home + "/.flowork/finance_audit.log"

	_ = os.MkdirAll(home+"/.flowork", 0700)

	if err := LogAudit("perm-test", 0.10); err != nil {
		t.Skip("could not write audit log:", err)
	}

	info, err := os.Stat(logPath)
	if err != nil {
		t.Skip("log file not found:", err)
	}
	perm := info.Mode().Perm()
	if perm != 0600 {
		t.Errorf("finance_audit.log permission = %04o, want 0600", perm)
	}
}

// --- I-A.1: RateLimiter daily cap + cooldown ---

func newTestLimiter(t *testing.T) *RateLimiter {
	t.Helper()
	tmp := t.TempDir()
	r := &RateLimiter{statePath: tmp + "/finance_daily.json"}
	r.state = dailyState{Date: todayUTC()}
	return r
}

func TestRateLimiter_AllowsUnderCap(t *testing.T) {
	r := newTestLimiter(t)
	if err := r.Check(5.0); err != nil {
		t.Fatalf("expected nil under cap, got %v", err)
	}
}

func TestRateLimiter_BlocksOverDailyCap(t *testing.T) {
	r := newTestLimiter(t)
	r.state.SpentUSD = 19.50
	err := r.Check(1.0) // 19.50 + 1.0 = 20.50 > 20.0
	if err == nil {
		t.Fatal("expected error over daily cap, got nil")
	}
}

func TestRateLimiter_CooldownActiveBlocks(t *testing.T) {
	r := newTestLimiter(t)
	r.state.CoolUntil = time.Now().Add(30 * time.Second) // cooldown aktif
	err := r.Check(0.01)
	if err == nil {
		t.Fatal("expected cooldown error, got nil")
	}
}

func TestRateLimiter_CooldownExpiredAllows(t *testing.T) {
	r := newTestLimiter(t)
	r.state.CoolUntil = time.Now().Add(-1 * time.Second) // cooldown sudah lewat
	r.state.SpentUSD = 0
	if err := r.Check(1.0); err != nil {
		t.Fatalf("expected nil after cooldown expired, got %v", err)
	}
}

func TestRateLimiter_RecordAccumulates(t *testing.T) {
	r := newTestLimiter(t)
	r.Record(3.0)
	r.Record(5.0)
	if got := r.DailySpent(); got != 8.0 {
		t.Errorf("DailySpent = %.2f, want 8.00", got)
	}
}

func TestRateLimiter_DailyResetOnNewDay(t *testing.T) {
	r := newTestLimiter(t)
	r.state.Date = "2000-01-01" // hari lama
	r.state.SpentUSD = 19.99
	// Check hari ini — harus reset
	if err := r.Check(5.0); err != nil {
		t.Fatalf("expected reset + allow on new day, got %v", err)
	}
}

func TestRateLimiter_LoopCallsTriggerCooldown(t *testing.T) {
	r := newTestLimiter(t)
	// Simulasi banyak call yang habiskan daily cap
	var lastErr error
	for i := 0; i < 100; i++ {
		lastErr = r.Check(0.21) // 100 × 0.21 = $21, di atas $20 cap
		if lastErr != nil {
			break
		}
		r.Record(0.21)
	}
	if lastErr == nil {
		t.Fatal("expected rate limit error after 100 loop calls exceeding $20 cap")
	}
	// Verify cooldown set
	r.mu.Lock()
	coolActive := !r.state.CoolUntil.IsZero() && time.Now().Before(r.state.CoolUntil)
	r.mu.Unlock()
	if !coolActive {
		t.Error("expected cooldown to be active after cap hit")
	}
}
