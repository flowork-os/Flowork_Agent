package finance

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/teetah2402/flowork/internal/fsutil"
)

const (
	DailyCapUSD      = 20.0        // $20/hari hard cap
	CooldownDuration = time.Minute // 1 menit cooldown setelah cap tercapai
)

// dailyState adalah persistent tracker harian yang disimpan ke file JSON.
type dailyState struct {
	Date      string    `json:"date"` // "2006-01-02" UTC
	SpentUSD  float64   `json:"spent_usd"`
	CoolUntil time.Time `json:"cool_until"` // zero = tidak cooldown
}

// RateLimiter menjaga daily cap dan cooldown untuk MintSubKey.
// Thread-safe via mu.
type RateLimiter struct {
	mu        sync.Mutex
	statePath string
	state     dailyState
}

// NewRateLimiter membuat RateLimiter yang load state dari disk (atau fresh kalau belum ada).
func NewRateLimiter() *RateLimiter {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".flowork", "finance_daily.json")
	r := &RateLimiter{statePath: path}
	r.load()
	return r
}

func (r *RateLimiter) load() {
	data, err := os.ReadFile(r.statePath)
	if err != nil {
		return // file belum ada = fresh state
	}
	if err := json.Unmarshal(data, &r.state); err != nil {
		fmt.Fprintf(os.Stderr, "finance: corrupt rate limiter state, resetting: %v\n", err)
	}
}

func (r *RateLimiter) save() {
	_ = os.MkdirAll(filepath.Dir(r.statePath), 0700)
	data, _ := json.Marshal(r.state)
	_ = fsutil.WriteFileAtomic(r.statePath, data, 0600)
}

func todayUTC() string {
	return time.Now().UTC().Format("2006-01-02")
}

// Check validates request against daily cap and cooldown.
// Returns error jika over cap atau masih cooldown.
func (r *RateLimiter) Check(usdAmount float64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Reset di hari baru
	if r.state.Date != todayUTC() {
		r.state = dailyState{Date: todayUTC()}
	}

	// Cek cooldown aktif
	if !r.state.CoolUntil.IsZero() && time.Now().Before(r.state.CoolUntil) {
		remaining := time.Until(r.state.CoolUntil).Round(time.Second)
		return fmt.Errorf("finance: rate limit — cooldown aktif, tunggu %s (daily cap $%.2f tercapai)", remaining, DailyCapUSD)
	}

	// Cek apakah request ini akan melewati daily cap
	if r.state.SpentUSD+usdAmount > DailyCapUSD {
		r.state.CoolUntil = time.Now().Add(CooldownDuration)
		r.save()
		return fmt.Errorf("finance: DAILY CAP TERCAPAI — sudah $%.2f dari $%.2f hari ini; cooldown %s",
			r.state.SpentUSD, DailyCapUSD, CooldownDuration)
	}

	return nil
}

// Record mencatat pengeluaran yang sudah berhasil.
func (r *RateLimiter) Record(usdAmount float64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state.Date != todayUTC() {
		r.state = dailyState{Date: todayUTC()}
	}
	r.state.SpentUSD += usdAmount
	r.save()
}

// DailySpent mengembalikan total pengeluaran hari ini (thread-safe).
func (r *RateLimiter) DailySpent() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state.Date != todayUTC() {
		return 0
	}
	return r.state.SpentUSD
}
