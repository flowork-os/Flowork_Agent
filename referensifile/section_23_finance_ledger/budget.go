// Package finance — budget.go: per-call BudgetGuard untuk semua LLM calls.
//
// Beda dengan RateLimiter (ratelimit.go) yang cap MintSubKey $20/hari
// dengan sliding daily quota, BudgetGuard ini pre-call safety yang lebih
// KETAT ($0.50/hari default) untuk melindungi crypto wallet Flowork dari
// runaway loop.
//
// Pilar 3.2 Autonomy Skills (FUTURE_PLAN.md): "Tanpa ini, 1 loop runaway
// = wallet habis". Kritis untuk crypto-survival arsitektur Ayah (wallet
// 0xd129...).
//
// Cara pakai:
//
//	guard := finance.NewBudgetGuard()
//	if err := guard.CheckBudget(estimatedCost); err != nil {
//	    if errors.Is(err, finance.ErrBudgetExceeded) {
//	        // Switch ke free model (qwen3-coder:free) atau stop
//	        model = os.Getenv("FREE_AGENT_MODEL")
//	    }
//	    return err
//	}
//	// ... call LLM ...
//	guard.Record(actualCost)
//
// Polling /auth/key otomatis (cache 60 detik) supaya UsedToday sinkron
// dengan actual OpenRouter balance — meskipun process restart.
package finance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// ErrBudgetExceeded sinyal caller harus switch ke free model atau stop.
var ErrBudgetExceeded = errors.New("finance: budget exceeded (daily atau per-task)")

// Default caps — conservative untuk survival mode. Override via env.
//
// IMPORTANT: OpenRouter /auth/key endpoint mengembalikan **lifetime usage**
// dari key (total sejak key dibuat), BUKAN daily usage. Field `usedToday`
// di BudgetGuard dipakai sebagai nama untuk field status, tapi nilainya
// sebenarnya mengikuti poll lifetime. Interpretasi:
//
//   - `DailyCapUSD` conservative threshold — kalau lifetime usage key
//     sudah melewati ini, agent harus switch ke free model (safety signal)
//   - Untuk true daily tracking: merge dengan RateLimiter.dailyState (future)
//     atau create dedicated key harian via FinanceMinister.MintSubKey
//     dengan `limit` field.
//
// Default $5.00 realistic observed baseline (Opus/Claude workload 2026-04).
// Ayah bisa override via FLOWORK_BUDGET_DAILY_USD env kalau mau ketat.
//
// rc-claude-rescue 2026-04-20: PerTaskCap naik dari $0.20 → $1.00 karena
// Claude Opus/Sonnet pada scan-style request (50k+ input tokens) secara
// realistis butuh ~$0.30-$0.50 per call. Cap $0.20 reject setiap /scan
// command pre-HTTP, user melihat "limit" padahal OpenRouter saldo OK.
// Daily cap $5 tetap jaga runaway (max 5 tasks/day di tier Claude Opus).
const (
	DefaultBudgetDailyUSD   = 5.00
	DefaultBudgetPerTaskUSD = 1.00
	pollCacheDuration       = 60 * time.Second
	openrouterAuthKeyURL    = "https://openrouter.ai/api/v1/auth/key"
)

// BudgetGuard adalah pre-call safety net. Thread-safe.
type BudgetGuard struct {
	DailyCapUSD   float64 // hard daily cap (e.g. 0.50)
	PerTaskCapUSD float64 // hard per-call cap (e.g. 0.05)
	FreeFallback  bool    // true = caller diharapkan switch ke free model (bukan stop)

	mu            sync.Mutex
	openrouterKey string
	lastPoll      time.Time
	usedToday     float64
	reserved      float64 // pre-reserved from concurrent CheckBudget calls (TOCTOU fix)
	pollErr       error   // last poll error (untuk observability)
	httpClient    *http.Client
	resolver      LimitsResolver // rc91: optional per-model/per-agent cap override

	// rc-budget-polish 2026-04-20: daily reset tracking.
	// OpenRouter /auth/key return lifetime usage, bukan daily. Untuk
	// derive daily, kita snapshot lifetime di start of day (`dayStartUsage`)
	// dan compute `usedToday = lifetime - dayStartUsage`. Saat day change,
	// snapshot re-anchored dan usedToday reset ke 0 (atau small delta).
	dayStartUsage float64 // lifetime usage saat day-of-year current di-enter
	dayStartDay   int     // day-of-year (1-366) ketika dayStartUsage di-capture
	snapshotPath  string  // file path untuk persist day snapshot (survive restart)

	// rc-p0-finalize 2026-04-20: alert tracking untuk telegram notif.
	// Bitset — alertBit80 / alertBit100 — supaya tiap threshold cuma fire
	// sekali per hari. Reset bareng dayStartDay re-anchor.
	// BUG-FIX: Changed from int to uint8 to prevent overflow on 32-bit systems
	alertedToday uint8

	// alertFn dipasang via SetAlertFn — wraps notify.AlertOwnerFireForget.
	// Plugin pattern supaya finance package tidak bergantung ke notify package
	// (avoid import cycle / keep finance ringan).
	alertFn func(message string)

	// sfg prevents thundering herd on cache expiry: only one HTTP poll at a
	// time; concurrent callers coalesce onto the in-flight request (BUG-022).
	sfg singleflight.Group
}

// dayNow returns day-of-year UTC.
func dayNow() int { return time.Now().UTC().YearDay() }

// daySnapshot untuk persistence — survive restart supaya baseline tidak hilang
// mid-day (kalau hilang, tracker = 0 dan cap bisa bocor kedua kali).
//
// AlertedThresholds: bitset thresholds yang sudah pernah fire alert hari ini
// supaya gak duplicate notif. Bit 0 = 80%, bit 1 = 100%.
type daySnapshot struct {
	Day               int     `json:"day"`
	DayStartUsage     float64 `json:"day_start_usage"`
	LastSaved         string  `json:"last_saved"`
	AlertedThresholds uint8   `json:"alerted_thresholds,omitempty"`
}

// Threshold bits — extensible kalau mau tambah 50%, 90%, dst.
const (
	alertBit80  = 1 << 0
	alertBit100 = 1 << 1
)

func (g *BudgetGuard) loadSnapshot() {
	if g.snapshotPath == "" {
		return
	}
	raw, err := os.ReadFile(g.snapshotPath)
	if err != nil {
		return // no prior snapshot, akan di-init pada poll pertama
	}
	var s daySnapshot
	if json.Unmarshal(raw, &s) != nil {
		return
	}
	// Only honor snapshot kalau masih di hari yang sama
	if s.Day == dayNow() && s.DayStartUsage >= 0 {
		g.dayStartUsage = s.DayStartUsage
		g.dayStartDay = s.Day
		g.alertedToday = s.AlertedThresholds
	}
}

func (g *BudgetGuard) saveSnapshot() {
	if g.snapshotPath == "" {
		return
	}
	g.mu.Lock()
	s := daySnapshot{
		Day:               g.dayStartDay,
		DayStartUsage:     g.dayStartUsage,
		LastSaved:         time.Now().UTC().Format(time.RFC3339),
		AlertedThresholds: g.alertedToday,
	}
	// BUG-RACE-FIX: Marshal under lock to prevent TOCTOU corruption
	raw, err := json.Marshal(s)
	g.mu.Unlock()
	if err != nil {
		return
	}
	// Best-effort atomic write: tmp + rename
	tmp := g.snapshotPath + ".tmp"
	if err := os.MkdirAll(filepathDir(g.snapshotPath), 0755); err == nil {
		// Sprint 3.5d (BUG-W57 fix): mode 0o600 — budget data sensitive.
		if err := os.WriteFile(tmp, raw, 0o600); err == nil {
			_ = os.Rename(tmp, g.snapshotPath)
		}
	}
}

// filepathDir tanpa depend ke path/filepath (keep finance pure).
func filepathDir(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[:i]
		}
	}
	return "."
}

// LimitsResolver adalah dependency-injection point supaya BudgetGuard bisa
// dapat per-model / per-agent cap dari policy.yaml tanpa harus import cycle
// dengan `internal/policy`. Caller (main.go) set resolver di startup setelah
// policy.Shared(workspace) loaded.
//
// Return zero untuk "tidak ada override — pakai default guard".
type LimitsResolver func(model, agent string) (dailyCap, perTaskCap float64, forceFree bool)

// SetLimitsResolver dipasang sekali di startup, biasanya setelah workspace +
// policy.Shared() sudah siap. Aman dipanggil ulang (misal saat reload).
func (g *BudgetGuard) SetLimitsResolver(fn LimitsResolver) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.resolver = fn
}

// SetAlertFn pasang callback alert (typically notify.AlertOwnerFireForget).
// Dipanggil di startup. Plugin pattern — finance tidak import notify untuk
// hindari cycle + jaga finance pure.
func (g *BudgetGuard) SetAlertFn(fn func(message string)) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.alertFn = fn
}

// CheckBudgetFor adalah varian CheckBudget yang consult LimitsResolver kalau
// terpasang. Caller (provider) boleh pakai ini saat mereka tahu model + agent,
// sehingga policy per-model/per-agent berlaku. Fallback: kalau resolver nil,
// perilaku identik dengan CheckBudget.
// CheckBudgetFor adalah varian CheckBudget yang consult LimitsResolver kalau
// terpasang. Caller (provider) boleh pakai ini saat mereka tahu model + agent,
// sehingga policy per-model/per-agent berlaku. Fallback: kalau resolver nil,
// perilaku identik dengan CheckBudget.
//
// TOCTOU fix: sama seperti CheckBudget — check dan reserve atomik di bawah lock.
func (g *BudgetGuard) CheckBudgetFor(ctx context.Context, model, agent string, estimatedUSD float64) error {
	// rc177 fix: free-tier model (`:free` suffix) cost = $0. Skip budget
	// guard ENTIRELY untuk free models — sebelumnya estimateCostUSD kasih
	// reservation positive, plus current usedToday > daily ($5.07/$5.00)
	// terus reject free call meski actual cost $0.
	// Free model = unlimited karena ga charge user wallet.
	if strings.Contains(strings.ToLower(model), ":free") {
		return nil
	}

	_ = g.refreshUsed(ctx)

	g.mu.Lock()
	defer g.mu.Unlock()

	daily := g.DailyCapUSD
	perTask := g.PerTaskCapUSD
	forceFree := false
	if g.resolver != nil {
		d, p, ff := g.resolver(model, agent)
		if d > 0 {
			daily = d
		}
		if p > 0 {
			perTask = p
		}
		forceFree = ff
	}

	if forceFree {
		return fmt.Errorf("%w: agent=%q model=%q policy.force_free=true",
			ErrBudgetExceeded, agent, model)
	}
	if estimatedUSD > perTask {
		return fmt.Errorf("%w: per-task=$%.4f/$%.2f (model=%s agent=%s)",
			ErrBudgetExceeded, estimatedUSD, perTask, model, agent)
	}
	if g.usedToday+g.reserved+estimatedUSD > daily {
		return fmt.Errorf("%w: daily=$%.4f/$%.2f (model=%s agent=%s)",
			ErrBudgetExceeded, g.usedToday+g.reserved, daily, model, agent)
	}
	g.reserved += estimatedUSD
	return nil
}

// Shared mengembalikan proses-level singleton BudgetGuard.
//
// Gunakan ini dari provider path (rc81) supaya /api/budget/status di dashboard
// mencerminkan usage yang sama dengan guard yang benar-benar mem-block LLM
// calls. NewBudgetGuard masih dipertahankan untuk test / instance terpisah.
func Shared() *BudgetGuard {
	sharedGuardOnce.Do(func() {
		sharedGuard = NewBudgetGuard()
	})
	return sharedGuard
}

var (
	sharedGuard     *BudgetGuard
	sharedGuardOnce sync.Once
)

// NewBudgetGuard bikin guard dengan defaults + env overrides.
//
// Env vars:
//
//	FLOWORK_BUDGET_DAILY_USD    — daily cap (default 0.50)
//	FLOWORK_BUDGET_PER_TASK_USD — per-call cap (default 0.05)
//	FLOWORK_BUDGET_FREE_FALLBACK — set "0" untuk disable fallback (default enable)
//	FLOWORK_BUDGET_SNAPSHOT     — path untuk daily snapshot persistence
//	                                (default: state/budget/daily.json relatif cwd)
//	OPENROUTER_API_KEY — dipakai untuk /auth/key poll
func NewBudgetGuard() *BudgetGuard {
	g := &BudgetGuard{
		DailyCapUSD:   DefaultBudgetDailyUSD,
		PerTaskCapUSD: DefaultBudgetPerTaskUSD,
		FreeFallback:  true,
		openrouterKey: os.Getenv("OPENROUTER_API_KEY"),
		httpClient:    &http.Client{Timeout: 10 * time.Second},
		snapshotPath:  "state/budget/daily.json",
	}
	if v := os.Getenv("FLOWORK_BUDGET_DAILY_USD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			g.DailyCapUSD = f
		}
	}
	if v := os.Getenv("FLOWORK_BUDGET_PER_TASK_USD"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			g.PerTaskCapUSD = f
		}
	}
	if os.Getenv("FLOWORK_BUDGET_FREE_FALLBACK") == "0" {
		g.FreeFallback = false
	}
	if v := os.Getenv("FLOWORK_BUDGET_SNAPSHOT"); v != "" {
		g.snapshotPath = v
	}
	g.loadSnapshot()
	return g
}

// WouldExceed returns true kalau mengeksekusi task dengan estimatedUSD
// akan melewati daily cap atau per-task cap. Caller inspect untuk putuskan
// switch ke free tier atau stop.
func (g *BudgetGuard) WouldExceed(estimatedUSD float64) bool {
	if estimatedUSD < 0 {
		estimatedUSD = 0
	}
	if estimatedUSD > g.PerTaskCapUSD {
		return true
	}
	g.mu.Lock()
	used := g.usedToday + g.reserved
	g.mu.Unlock()
	return used+estimatedUSD > g.DailyCapUSD
}

// CheckBudget adalah entry point. Return ErrBudgetExceeded kalau melewati
// threshold. Caller WAJIB handle error — tanpa ini guard tidak efektif.
//
// estimatedUSD = perkiraan biaya call (bisa dari estimator token count
// × rate-per-million dari internal/router.PickModel metadata).
//
// TOCTOU fix: check dan reserve dilakukan atomik di bawah lock yang sama,
// sehingga concurrent goroutines tidak bisa double-spend budget.
// Caller wajib panggil Record(actual) setelah call selesai, atau
// ReleaseReservation(estimated) jika call gagal sebelum dimulai.
func (g *BudgetGuard) CheckBudget(ctx context.Context, estimatedUSD float64) error {
	_ = g.refreshUsed(ctx)

	g.mu.Lock()
	defer g.mu.Unlock()
	if estimatedUSD > g.PerTaskCapUSD {
		return fmt.Errorf("%w: per-task=$%.4f/$%.2f, daily=$%.4f/$%.2f",
			ErrBudgetExceeded, estimatedUSD, g.PerTaskCapUSD, g.usedToday+g.reserved, g.DailyCapUSD)
	}
	if g.usedToday+g.reserved+estimatedUSD > g.DailyCapUSD {
		return fmt.Errorf("%w: daily=$%.4f/$%.2f, per-task=$%.4f/$%.2f",
			ErrBudgetExceeded, g.usedToday+g.reserved, g.DailyCapUSD, estimatedUSD, g.PerTaskCapUSD)
	}
	g.reserved += estimatedUSD
	return nil
}

// ReleaseReservation membatalkan reservasi budget jika LLM call gagal sebelum
// dimulai (network error, context cancel sebelum request terkirim, dll).
// Jika call berhasil, gunakan Record() — jangan panggil keduanya.
func (g *BudgetGuard) ReleaseReservation(estimatedUSD float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.reserved -= estimatedUSD
	if g.reserved < 0 {
		g.reserved = 0
	}
}

// Record settle reservasi dan tambah actualUSD ke usedToday. Dipanggil
// SETELAH call LLM sukses. Thread-safe.
// Jangan panggil Record DAN ReleaseReservation untuk call yang sama.
//
// rc-p0-finalize 2026-04-20: post-record threshold alert. Kalau usedToday
// crosses 80% atau 100% pertama kali hari ini, fire alertFn (typically
// notify.AlertOwnerFireForget). Tracked via alertedToday bitset, persist
// via daySnapshot supaya restart tidak duplicate alert.
func (g *BudgetGuard) Record(actualUSD float64) {
	g.mu.Lock()
	// Settle: release reservation (estimasi) dan record actual usage.
	if g.reserved >= actualUSD {
		g.reserved -= actualUSD
	} else {
		g.reserved = 0
	}
	g.usedToday += actualUSD

	// Cek thresholds — capture snapshot variables untuk fire-after-unlock
	pct := 0.0
	if g.DailyCapUSD > 0 {
		pct = (g.usedToday / g.DailyCapUSD) * 100
	}
	cross80 := pct >= 80 && (g.alertedToday&alertBit80) == 0
	cross100 := pct >= 100 && (g.alertedToday&alertBit100) == 0
	if cross80 {
		g.alertedToday |= alertBit80
	}
	if cross100 {
		g.alertedToday |= alertBit100
	}
	usedNow := g.usedToday
	capNow := g.DailyCapUSD
	fn := g.alertFn
	needSave := cross80 || cross100
	g.mu.Unlock()

	if needSave {
		g.saveSnapshot()
	}
	if fn == nil {
		return
	}
	if cross100 {
		fn(fmt.Sprintf("🚨 *Flowork Budget HIT 100%%*\n\nDaily cap *$%.2f* tercapai (used *$%.4f*).\nLLM call selanjutnya akan di-reject sampai 00:00 UTC atau cap dinaikkan.\nAyah perlu cek dashboard atau set `FREE_AGENT_MODEL` aktif.",
			capNow, usedNow))
	} else if cross80 {
		fn(fmt.Sprintf("⚠️ *Flowork Budget 80%%*\n\nUsed *$%.4f* / *$%.2f* (%.1f%%).\nMasih ada %.4f, tapi pace makin cepat — pertimbangkan switch ke free-tier untuk task ringan.",
			usedNow, capNow, pct, capNow-usedNow))
	}
}

// Used returns jumlah USD dipakai hari ini (kombinasi local tracker + poll).
// Thread-safe. Untuk dashboard panel + observability.
func (g *BudgetGuard) Used() float64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.usedToday
}

// refreshUsed poll OpenRouter /auth/key untuk dapat real balance.
// Cache 60 detik untuk hindari spam. singleflight dedups concurrent callers
// to prevent thundering herd on cache expiry (BUG-022).
func (g *BudgetGuard) refreshUsed(ctx context.Context) error {
	g.mu.Lock()
	if time.Since(g.lastPoll) < pollCacheDuration {
		g.mu.Unlock()
		return nil // cache hit
	}
	// Re-read env setiap poll — guard biasanya di-konstruksi package-init
	// (sebelum main() call config.LoadDotEnv), jadi env baru tersedia
	// di request-time.
	key := g.openrouterKey
	if key == "" {
		key = os.Getenv("OPENROUTER_API_KEY")
		g.openrouterKey = key
	}
	g.mu.Unlock()

	// Coalesce concurrent refresh calls into one HTTP request.
	_, err, _ := g.sfg.Do("refresh", func() (any, error) {
		return nil, g.doRefresh(ctx, key)
	})
	return err
}

func (g *BudgetGuard) doRefresh(ctx context.Context, key string) error {

	if key == "" {
		g.mu.Lock()
		g.pollErr = errors.New("OPENROUTER_API_KEY not set")
		g.mu.Unlock()
		return g.pollErr
	}

	// BUG-FIX: Add explicit timeout to prevent indefinite hangs
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, openrouterAuthKeyURL, nil)
	if err != nil {
		return fmt.Errorf("finance: refreshUsed: http request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		g.mu.Lock()
		g.pollErr = err
		g.mu.Unlock()
		return fmt.Errorf("finance: refreshUsed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		g.mu.Lock()
		g.pollErr = fmt.Errorf("openrouter /auth/key HTTP %d", resp.StatusCode)
		g.mu.Unlock()
		return g.pollErr
	}
	var parsed struct {
		Data struct {
			Usage   float64 `json:"usage"`
			Limit   float64 `json:"limit"`
			IsFreeT bool    `json:"is_free_tier"`
			RateLim any     `json:"rate_limit"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		g.mu.Lock()
		g.pollErr = err
		g.mu.Unlock()
		return fmt.Errorf("finance: refreshUsed: %w", err)
	}

	g.mu.Lock()
	// rc-budget-polish 2026-04-20: derive daily usage dari lifetime delta.
	// Pertama kali atau day-of-year berubah → re-anchor baseline.
	nowDay := dayNow()
	lifetime := parsed.Data.Usage
	needSave := false
	if g.dayStartDay == 0 || g.dayStartDay != nowDay {
		// Day boundary hit (atau fresh start). Snapshot baseline + reset alert bits.
		g.dayStartUsage = lifetime
		g.dayStartDay = nowDay
		g.alertedToday = 0 // rc-p0-finalize: alert state reset bareng day rollover
		needSave = true
	}
	// Sanity: kalau lifetime turun (snapshot corrupt / OpenRouter reset),
	// re-anchor conservatively supaya tracker tidak negative.
	if lifetime < g.dayStartUsage {
		g.dayStartUsage = lifetime
		needSave = true
	}
	g.usedToday = lifetime - g.dayStartUsage
	if g.usedToday < 0 {
		g.usedToday = 0
	}
	g.lastPoll = time.Now()
	g.pollErr = nil
	g.mu.Unlock()
	if needSave {
		g.saveSnapshot()
	}
	return nil
}

// Status snapshot untuk UI dashboard / /api/budget endpoint.
type Status struct {
	DailyCap      float64 `json:"daily_cap_usd"`
	PerTaskCap    float64 `json:"per_task_cap_usd"`
	UsedToday     float64 `json:"used_today_usd"`
	Reserved      float64 `json:"reserved_usd"` // pending concurrent reservations
	Remaining     float64 `json:"remaining_usd"`
	PercentUsed   float64 `json:"percent_used"`
	FreeFallback  bool    `json:"free_fallback_enabled"`
	LastPollAgo   string  `json:"last_poll_ago,omitempty"`
	LastPollError string  `json:"last_poll_error,omitempty"`
}

// GetStatus ringkas untuk dashboard.
func (g *BudgetGuard) GetStatus() Status {
	g.mu.Lock()
	defer g.mu.Unlock()
	effective := g.usedToday + g.reserved
	remaining := g.DailyCapUSD - effective
	if remaining < 0 {
		remaining = 0
	}
	pct := 0.0
	if g.DailyCapUSD > 0 {
		pct = (effective / g.DailyCapUSD) * 100
	}
	s := Status{
		DailyCap:     g.DailyCapUSD,
		PerTaskCap:   g.PerTaskCapUSD,
		UsedToday:    g.usedToday,
		Reserved:     g.reserved,
		Remaining:    remaining,
		PercentUsed:  pct,
		FreeFallback: g.FreeFallback,
	}
	if !g.lastPoll.IsZero() {
		s.LastPollAgo = time.Since(g.lastPoll).Round(time.Second).String()
	}
	if g.pollErr != nil {
		s.LastPollError = g.pollErr.Error()
	}
	return s
}
