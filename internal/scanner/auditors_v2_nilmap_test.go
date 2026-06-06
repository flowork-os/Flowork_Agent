package scanner

import "testing"

// Regресi: nil_map_write_auditor harus ngenalin guard idiom `if x == nil { x = map[...]{} }`
// (false-positive yang bikin 224 critical palsu di crew agents) TAPI tetep nangkep
// write nil-map yang BENERAN ga di-guard.
func TestAuditNilMapWrite_GuardIsNotFlagged(t *testing.T) {
	// Pola persis dari crew agents (agents/*/main.go) — ADA guard, AMAN.
	guarded := `package main
func f(notifyChatID string, raw string) {
	var args map[string]any
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &args)
	}
	if args == nil {
		args = map[string]any{}
	}
	args["notify_chat_id"] = notifyChatID
}`
	if got := AuditNilMapWrite("x.go", guarded); len(got) != 0 {
		t.Fatalf("guarded write keflag false-positive: %d finding(s): %+v", len(got), got)
	}
}

func TestAuditNilMapWrite_RealBugStillCaught(t *testing.T) {
	// Nil map, write LANGSUNG tanpa init — ini BENERAN panic. Harus tetep keflag.
	buggy := `package main
func f() {
	var m map[string]int
	m["x"] = 1
}`
	got := AuditNilMapWrite("x.go", buggy)
	if len(got) != 1 {
		t.Fatalf("real nil-map-write bug ga ketangkep: expected 1, got %d: %+v", len(got), got)
	}
	if got[0].Severity != SevCritical {
		t.Fatalf("severity salah: %v", got[0].Severity)
	}
}

func TestAuditNilMapWrite_ComparisonNotFlagged(t *testing.T) {
	// `inner[k] == ""` itu BACA (komparasi), AMAN di nil map — jangan keflag.
	// Pola asli dari internal/settingsapi/youtube.go:77.
	cmp := `package main
func f(raw string) {
	var inner map[string]string
	_ = json.Unmarshal([]byte(raw), &inner)
	if inner["client_id"] == "" || inner["client_secret"] == "" {
		return
	}
}`
	if got := AuditNilMapWrite("x.go", cmp); len(got) != 0 {
		t.Fatalf("komparasi (==) keflag false-positive sbg write: %+v", got)
	}
}

func TestAuditNilMapWrite_MakeReInitNotFlagged(t *testing.T) {
	// Re-init pakai make() juga harus dikenali aman.
	ok := `package main
func f() {
	var m map[string]int
	m = make(map[string]int)
	m["x"] = 1
}`
	if got := AuditNilMapWrite("x.go", ok); len(got) != 0 {
		t.Fatalf("make() re-init keflag false-positive: %+v", got)
	}
}
