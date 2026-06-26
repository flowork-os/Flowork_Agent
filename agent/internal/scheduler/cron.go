// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Spec struct {
	Min    map[int]bool
	Hour   map[int]bool
	Day    map[int]bool
	Month  map[int]bool
	DOW    map[int]bool
	Source string
}

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

	if dow[7] {
		dow[0] = true
		delete(dow, 7)
	}
	return Spec{Min: min, Hour: hour, Day: day, Month: mon, DOW: dow, Source: cron}, nil
}

func expandField(field string, lo, hi int) (map[int]bool, error) {
	out := map[int]bool{}
	for _, part := range strings.Split(field, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

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

func (s Spec) Matches(now time.Time) bool {
	t := now
	if !s.Min[t.Minute()] || !s.Hour[t.Hour()] || !s.Month[int(t.Month())] {
		return false
	}

	dayMatch := s.Day[t.Day()]
	dowMatch := s.DOW[int(t.Weekday())]

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
