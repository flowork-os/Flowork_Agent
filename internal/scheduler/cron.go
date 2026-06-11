// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30 (re-locked 2026-06-11)
// 2026-06-11 OWNER-APPROVED BUG FIX: Spec.Matches forced now.UTC() before testing
//   minute/hour, so a Schedule/Trigger given LOCAL wall-clock time (type_time uses
//   time.Now()) was matched against UTC — every cron fired 7h off (WIB) i.e. while
//   the machine was usually off, so jobs never ran. Matches now uses `now` as
//   passed; callers choose tz (Section-18 sends now.UTC(), trigger sends local).
// Reason: Section 18 phase 1 standard 5-field cron parser. Field order:
//   minute hour day-of-month month day-of-week. Format support: `*`,
//   number `5`, range `1-5`, step `*/N`, list `1,3,5`. Phase 2 (L/W/#
//   syntax, seconds field, natural language) → tambah file baru.
//
// cron.go — Section 18 phase 1: minimal 5-field cron parser + matcher.

package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Spec — parsed cron. Each field = set of allowed integers.
type Spec struct {
	Min    map[int]bool // 0-59
	Hour   map[int]bool // 0-23
	Day    map[int]bool // 1-31
	Month  map[int]bool // 1-12
	DOW    map[int]bool // 0-6 (0=Sun) — convention: 7→0
	Source string       // original cron string
}

// Parse — 5-field standard. Return Spec atau error.
func Parse(cron string) (Spec, error) {
	cron = strings.TrimSpace(cron)
	if cron == "" {
		return Spec{}, fmt.Errorf("cron empty")
	}
	parts := strings.Fields(cron)
	if len(parts) != 5 {
		return Spec{}, fmt.Errorf("cron expects 5 fields, got %d (%q)", len(parts), cron)
	}
	min, err := expandField(parts[0], 0, 59)
	if err != nil {
		return Spec{}, fmt.Errorf("minute: %w", err)
	}
	hour, err := expandField(parts[1], 0, 23)
	if err != nil {
		return Spec{}, fmt.Errorf("hour: %w", err)
	}
	day, err := expandField(parts[2], 1, 31)
	if err != nil {
		return Spec{}, fmt.Errorf("day: %w", err)
	}
	mon, err := expandField(parts[3], 1, 12)
	if err != nil {
		return Spec{}, fmt.Errorf("month: %w", err)
	}
	dow, err := expandField(parts[4], 0, 7)
	if err != nil {
		return Spec{}, fmt.Errorf("dow: %w", err)
	}
	// Normalize Sunday: 7 → 0.
	if dow[7] {
		dow[0] = true
		delete(dow, 7)
	}
	return Spec{Min: min, Hour: hour, Day: day, Month: mon, DOW: dow, Source: cron}, nil
}

// expandField — convert one field (e.g. "*/5") to set[int].
func expandField(field string, lo, hi int) (map[int]bool, error) {
	out := map[int]bool{}
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// Step: "*/N" or "a-b/N"
		step := 1
		if i := strings.Index(part, "/"); i >= 0 {
			stepStr := part[i+1:]
			n, err := strconv.Atoi(stepStr)
			if err != nil || n <= 0 {
				return nil, fmt.Errorf("invalid step %q", stepStr)
			}
			step = n
			part = part[:i]
		}
		start, end := lo, hi
		if part == "*" || part == "" {
			// keep lo..hi
		} else if i := strings.Index(part, "-"); i >= 0 {
			a, errA := strconv.Atoi(part[:i])
			b, errB := strconv.Atoi(part[i+1:])
			if errA != nil || errB != nil {
				return nil, fmt.Errorf("invalid range %q", part)
			}
			if a < lo || b > hi || a > b {
				return nil, fmt.Errorf("range %d-%d out of bounds [%d,%d]", a, b, lo, hi)
			}
			start, end = a, b
		} else {
			n, err := strconv.Atoi(part)
			if err != nil || n < lo || n > hi {
				return nil, fmt.Errorf("value %q out of [%d,%d]", part, lo, hi)
			}
			if step == 1 {
				out[n] = true
				continue
			}
			start, end = n, hi
		}
		for v := start; v <= end; v += step {
			out[v] = true
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("empty field")
	}
	return out, nil
}

// Matches — true kalau `now` due. Minute-resolution. Cocok di TIMEZONE caller:
// kita pakai field `now` apa adanya (BUG fix 2026-06-11, owner-approved: dulu paksa
// `now.UTC()` → cron Schedule/Trigger yg dikasih waktu LOKAL malah dicocokin di UTC
// = meleset 7 jam, jadwal gak pernah fire). Pemanggil yg mau UTC kirim now.UTC()
// (Section-18 scheduler); yg mau lokal kirim time.Now() (trigger type_time).
func (s Spec) Matches(now time.Time) bool {
	t := now
	if !s.Min[t.Minute()] || !s.Hour[t.Hour()] || !s.Month[int(t.Month())] {
		return false
	}
	// Special rule: kalau Day field == "*" tapi DOW != "*" (atau sebaliknya),
	// match kalau salah satu match. Standard cron: OR semantics.
	dayMatch := s.Day[t.Day()]
	dowMatch := s.DOW[int(t.Weekday())]
	// Tiebreaker: kalau keduanya wildcard penuh, both true.
	dayAll := isFullRange(s.Day, 1, 31)
	dowAll := isFullRange(s.DOW, 0, 6)
	if dayAll && dowAll {
		return true
	}
	if dayAll {
		return dowMatch
	}
	if dowAll {
		return dayMatch
	}
	return dayMatch || dowMatch
}

func isFullRange(m map[int]bool, lo, hi int) bool {
	for i := lo; i <= hi; i++ {
		if !m[i] {
			return false
		}
	}
	return true
}

// Next — return next firing time setelah `after`. Brute-force step-by-minute
// untuk simplicity. Cap search 366*24*60 minute (≈ 1 tahun) — kalau lebih,
// return zero time + error.
func (s Spec) Next(after time.Time) (time.Time, error) {
	t := after.Truncate(time.Minute).Add(time.Minute)
	cap := 366 * 24 * 60
	for i := 0; i < cap; i++ {
		if s.Matches(t) {
			return t, nil
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("no next fire within 1 year")
}
