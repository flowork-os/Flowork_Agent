// Package finance — dormancy.go: graceful idle mode saat saldo OpenRouter
// kritis (Protokol Sekoci 2026-04-27).
//
// Beda dengan BudgetGuard yang reject pre-call kalau estimated cost > cap,
// Dormancy adalah "saldo ≈ 0" detection. Saat dormant, daemon warga:
//   - Tidak panggil external LLM (skip provider call entirely)
//   - Return canned message ke caller (UX: status panel tunjuk 'dormant',
//     bukan 'error' merah yang bikin Ayah panik)
//   - Auto-revive saat saldo balik ≥ revive threshold
//
// Dormancy tidak override Mode 2 (LocalOnly) di chain_mode.go. Kalau Ayah
// pakai Mode 2 / Mode 3, dormancy ngga relevan — local llama / brain provider
// jalan normal tanpa OpenRouter. Dormancy SPESIFIK untuk Mode 1 / Mode 4
// dimana cloud path adalah primary.
//
// Default thresholds tunable via env:
//
//	FLOWORK_DORMANT_BALANCE_USD  — saat remaining ≤ ini → dormant (default 0.05)
//	FLOWORK_DORMANT_REVIVE_USD   — saat remaining ≥ ini → revive (default 1.00)
//
// Hysteresis 0.05 → 1.00 cegah flapping kalau Ayah top-up cuma sedikit.

package finance

import (
	"context"
	"os"
	"strconv"
	"sync"
	"time"
)

// Dormancy thresholds — Protokol Sekoci defaults. Override via env.
const (
	DefaultDormantBalanceUSD = 0.05 // saldo ≤ ini = dormant
	DefaultDormantReviveUSD  = 1.00 // saldo ≥ ini = revive (hysteresis)
)

// DormancyState — current state. Atomic transition via DormancyMonitor.
type DormancyState string

const (
	DormancyAwake   DormancyState = "awake"   // saldo cukup, normal operation
	DormancyDormant DormancyState = "dormant" // saldo kritis, skip external LLM
)

// DormancyMonitor wraps BudgetGuard.refreshUsed() untuk derive remaining
// saldo, tracking state transition awake↔dormant. Thread-safe.
type DormancyMonitor struct {
	guard         *BudgetGuard
	mu            sync.RWMutex
	state         DormancyState
	balanceUSD    float64   // last observed remaining balance
	dormantThresh float64
	reviveThresh  float64
	lastChange    time.Time
}

// NewDormancyMonitor wires monitor ke shared BudgetGuard. Thresholds dari env
// atau pakai default Protokol Sekoci.
func NewDormancyMonitor(guard *BudgetGuard) *DormancyMonitor {
	dormant := DefaultDormantBalanceUSD
	revive := DefaultDormantReviveUSD
	if v := os.Getenv("FLOWORK_DORMANT_BALANCE_USD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			dormant = f
		}
	}
	if v := os.Getenv("FLOWORK_DORMANT_REVIVE_USD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= dormant {
			revive = f
		}
	}
	return &DormancyMonitor{
		guard:         guard,
		state:         DormancyAwake,
		dormantThresh: dormant,
		reviveThresh:  revive,
	}
}

// sharedDormancy adalah process-level singleton, mirror BudgetGuard.Shared().
var (
	sharedDormancy     *DormancyMonitor
	sharedDormancyOnce sync.Once
)

// SharedDormancy return singleton tied ke Shared() guard. Aman dipanggil
// dari banyak tempat — caller tidak perlu pegang reference.
func SharedDormancy() *DormancyMonitor {
	sharedDormancyOnce.Do(func() {
		sharedDormancy = NewDormancyMonitor(Shared())
	})
	return sharedDormancy
}

// IsDormant cek state current. Trigger refresh saldo dari OpenRouter (cached
// 60s di guard.refreshUsed). Return false kalau ada error poll — gracefully
// fail-open supaya error transient ngga pingin daemon mati.
//
// Hysteresis logic:
//   - state=awake + remaining ≤ dormantThresh → transition ke dormant
//   - state=dormant + remaining ≥ reviveThresh → transition ke awake
//   - di antara dua threshold → state stay (no flap)
func (d *DormancyMonitor) IsDormant(ctx context.Context) bool {
	if d == nil || d.guard == nil {
		return false
	}
	// Trigger refresh — internally cached 60s, aman spam.
	_ = d.guard.refreshUsed(ctx)

	d.guard.mu.Lock()
	used := d.guard.usedToday
	cap := d.guard.DailyCapUSD
	d.guard.mu.Unlock()

	// Remaining derived dari daily cap, BUKAN raw OpenRouter balance — karena
	// rate limiter pakai cap sebagai authoritative limit. Kalau OpenRouter
	// kasih credit besar tapi cap kita kecil, dormancy fire saat cap habis.
	// Behavior konsisten dengan BudgetGuard.WouldExceed.
	remaining := cap - used
	if remaining < 0 {
		remaining = 0
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.balanceUSD = remaining

	switch d.state {
	case DormancyAwake:
		if remaining <= d.dormantThresh {
			d.state = DormancyDormant
			d.lastChange = time.Now()
		}
	case DormancyDormant:
		if remaining >= d.reviveThresh {
			d.state = DormancyAwake
			d.lastChange = time.Now()
		}
	}
	return d.state == DormancyDormant
}

// State returns current state without triggering refresh — cheap getter
// untuk status panel atau metric tick.
func (d *DormancyMonitor) State() DormancyState {
	if d == nil {
		return DormancyAwake
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.state
}

// Snapshot ringkasan state untuk dashboard/status panel.
type DormancySnapshot struct {
	State          DormancyState `json:"state"`
	BalanceUSD     float64       `json:"balance_usd"`
	DormantThresh  float64       `json:"dormant_threshold_usd"`
	ReviveThresh   float64       `json:"revive_threshold_usd"`
	LastChangeUnix int64         `json:"last_change_unix,omitempty"`
}

// GetSnapshot non-blocking, no refresh — read latest cached state.
func (d *DormancyMonitor) GetSnapshot() DormancySnapshot {
	if d == nil {
		return DormancySnapshot{State: DormancyAwake}
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	s := DormancySnapshot{
		State:         d.state,
		BalanceUSD:    d.balanceUSD,
		DormantThresh: d.dormantThresh,
		ReviveThresh:  d.reviveThresh,
	}
	if !d.lastChange.IsZero() {
		s.LastChangeUnix = d.lastChange.Unix()
	}
	return s
}

// DormantMessage adalah canned response yang diberikan ke caller saat dormant.
// Format ramah ke user — bukan error message teknis. Caller (provider) inject
// ini sebagai response.Content saat skip LLM call.
//
// Mengandung emoji 💤 untuk visual signal di GUI/Telegram (bukan red badge).
func DormantMessage() string {
	return "💤 Mode dorman aktif — saldo OpenRouter kritis. Saya tidur sebentar, " +
		"nungguin Ayah top-up. Brain DB tetap inget semua, saat saldo balik ≥ $1.00 " +
		"saya bangun otomatis. (Set FLOWORK_PROVIDER_MODE=2 di .env untuk Local-only " +
		"mode kalau mau pakai Qwen lokal sambil dorman.)"
}
