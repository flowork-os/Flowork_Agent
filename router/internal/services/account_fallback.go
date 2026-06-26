// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package services

import (
	"strings"
	"time"
)

var BackoffConfig = struct {
	Base     time.Duration
	Max      time.Duration
	MaxLevel int
}{
	Base:     1 * time.Second,
	Max:      4 * time.Minute,
	MaxLevel: 8,
}

const TransientCooldown = 30 * time.Second

type ErrorRule struct {
	Text       string
	Status     int
	Backoff    bool
	CooldownMs int64
}

var (
	cooldownShort = int64(15 * time.Second / time.Millisecond)
	cooldownMed   = int64(60 * time.Second / time.Millisecond)
	cooldownLong  = int64(5 * time.Minute / time.Millisecond)
)

var ErrorRules = []ErrorRule{

	{Text: "no credentials", CooldownMs: cooldownLong},
	{Text: "request not allowed", CooldownMs: cooldownShort},
	{Text: "improperly formed request", CooldownMs: cooldownLong},
	{Text: "rate limit", Backoff: true},
	{Text: "too many requests", Backoff: true},
	{Text: "quota exceeded", Backoff: true},
	{Text: "capacity", Backoff: true},
	{Text: "overloaded", Backoff: true},

	{Status: 401, CooldownMs: cooldownLong},
	{Status: 402, CooldownMs: cooldownLong},
	{Status: 403, CooldownMs: cooldownLong},
	{Status: 404, CooldownMs: cooldownLong},
	{Status: 429, Backoff: true},
	{Status: 500, CooldownMs: cooldownShort},
	{Status: 502, CooldownMs: cooldownShort},
	{Status: 503, CooldownMs: cooldownShort},
	{Status: 504, CooldownMs: cooldownShort},
}

type FallbackDecision struct {
	ShouldFallback  bool
	Cooldown        time.Duration
	NewBackoffLevel int
}

func GetQuotaCooldown(backoffLevel int) time.Duration {
	if backoffLevel <= 1 {
		return BackoffConfig.Base
	}
	pow := 1 << (backoffLevel - 1)
	d := BackoffConfig.Base * time.Duration(pow)
	if d > BackoffConfig.Max {
		return BackoffConfig.Max
	}
	return d
}

func CheckFallbackError(status int, errorText string, backoffLevel int) FallbackDecision {
	lower := strings.ToLower(errorText)
	for _, rule := range ErrorRules {
		matched := false
		if rule.Text != "" && lower != "" && strings.Contains(lower, rule.Text) {
			matched = true
		}
		if !matched && rule.Status != 0 && rule.Status == status {
			matched = true
		}
		if !matched {
			continue
		}
		if rule.Backoff {
			nl := backoffLevel + 1
			if nl > BackoffConfig.MaxLevel {
				nl = BackoffConfig.MaxLevel
			}
			return FallbackDecision{ShouldFallback: true, Cooldown: GetQuotaCooldown(nl), NewBackoffLevel: nl}
		}
		return FallbackDecision{ShouldFallback: true, Cooldown: time.Duration(rule.CooldownMs) * time.Millisecond, NewBackoffLevel: backoffLevel}
	}
	return FallbackDecision{ShouldFallback: true, Cooldown: TransientCooldown, NewBackoffLevel: backoffLevel}
}

func IsAccountUnavailable(unavailableUntil time.Time) bool {
	if unavailableUntil.IsZero() {
		return false
	}
	return time.Now().Before(unavailableUntil)
}

func GetUnavailableUntil(cooldown time.Duration) time.Time {
	return time.Now().Add(cooldown)
}

func GetEarliestRateLimitedUntil(times []time.Time) (time.Time, bool) {
	var earliest time.Time
	found := false
	now := time.Now()
	for _, t := range times {
		if t.IsZero() || !t.After(now) {
			continue
		}
		if !found || t.Before(earliest) {
			earliest = t
			found = true
		}
	}
	return earliest, found
}
