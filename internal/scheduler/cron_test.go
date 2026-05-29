package scheduler

import (
	"testing"
	"time"
)

func TestParseStar(t *testing.T) {
	s, err := Parse("* * * * *")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for i := 0; i < 60; i++ {
		if !s.Min[i] {
			t.Fatalf("minute %d missing", i)
		}
	}
}

func TestParseStep(t *testing.T) {
	s, err := Parse("*/15 * * * *")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := []int{0, 15, 30, 45}
	for _, w := range want {
		if !s.Min[w] {
			t.Fatalf("step minute %d missing", w)
		}
	}
	if s.Min[7] {
		t.Fatalf("step minute 7 should not match")
	}
}

func TestParseRange(t *testing.T) {
	s, err := Parse("0 9-17 * * 1-5")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Monday 10:00 should match.
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC) // Monday June 1 2026
	if now.Weekday() != time.Monday {
		t.Skipf("test date not Monday: %s", now.Weekday())
	}
	if !s.Matches(now) {
		t.Fatalf("expected match at Mon 10:00")
	}
	// Saturday should not.
	sat := time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC)
	if s.Matches(sat) {
		t.Fatalf("Saturday should not match")
	}
}

func TestNext(t *testing.T) {
	s, _ := Parse("*/5 * * * *")
	now := time.Date(2026, 6, 1, 10, 2, 0, 0, time.UTC)
	next, err := s.Next(now)
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	want := time.Date(2026, 6, 1, 10, 5, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("expected %v, got %v", want, next)
	}
}

func TestInvalid(t *testing.T) {
	_, err := Parse("* * *")
	if err == nil {
		t.Fatal("expected error for 3 fields")
	}
	_, err = Parse("99 * * * *")
	if err == nil {
		t.Fatal("expected error for minute 99")
	}
}
