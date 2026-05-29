// Package approvals — Pilar 6.2 P4 DM Pairing Approval (OpenClaw security).
//
// Alih-alih whitelist statis di .env, bot bisa terima DM dari user baru tapi
// diam sampai Ayah approve via `/approve <user_id>`. User yang belum approved
// dapat auto-reply "pending", dan request di-queue ke Ayah via Telegram push.
//
// Storage: `state/approvals/users.json` — dict keyed by user ID (string).
// Fail-safe: file rusak / unreadable = treat as empty (no approved).
//
// Anti-spam: RequestApproval throttle per-user 24h supaya satu user tidak
// bisa flood bridge dengan request berulang.
package approvals

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Status approval user.
type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusDenied   Status = "denied"
)

// Approval adalah satu entri dalam users.json.
type Approval struct {
	UserID      int64     `json:"user_id"`
	Platform    string    `json:"platform"` // "telegram" | "discord"
	Username    string    `json:"username,omitempty"`
	FirstName   string    `json:"first_name,omitempty"`
	Status      Status    `json:"status"`
	RequestedAt time.Time `json:"requested_at"`
	ApprovedAt  time.Time `json:"approved_at,omitempty"`
	ApprovedBy  string    `json:"approved_by,omitempty"` // owner handle
	Note        string    `json:"note,omitempty"`
}

type storeFile struct {
	Version int                 `json:"version"`
	Users   map[string]Approval `json:"users"` // key = <platform>:<userID>
	Updated time.Time           `json:"updated_at"`
}

var mu sync.Mutex // serialize filesystem writes

// ── Public API ─────────────────────────────────────────────────────────

// IsApproved return true kalau user sudah di-approve oleh owner.
// Pending / Denied / Unknown = false. Owner yang sudah di TELEGRAM_OWNER_IDS
// tetap jalan lewat whitelist static — fungsi ini layer dinamis di atas itu.
func IsApproved(workspace, platform string, userID int64) bool {
	store, _ := load(workspace)
	if store == nil || store.Users == nil {
		return false
	}
	rec, ok := store.Users[key(platform, userID)]
	return ok && rec.Status == StatusApproved
}

// RequestApproval catat user baru sebagai pending. Anti-spam: kalau user
// sudah pernah request dalam 24h terakhir, return false (caller tidak perlu
// ping Ayah lagi).
//
// Return: (isNew, existingStatus, err)
func RequestApproval(workspace, platform string, userID int64, username, firstName string) (bool, Status, error) {
	mu.Lock()
	defer mu.Unlock()
	store, _ := load(workspace)
	if store == nil {
		store = &storeFile{Version: 1, Users: map[string]Approval{}}
	}
	if store.Users == nil {
		store.Users = map[string]Approval{}
	}
	k := key(platform, userID)
	if existing, ok := store.Users[k]; ok {
		// Kalau sudah approved / denied, return existing status tanpa update.
		if existing.Status == StatusApproved || existing.Status == StatusDenied {
			return false, existing.Status, nil
		}
		// Still pending — cek anti-spam window 24h.
		if time.Since(existing.RequestedAt) < 24*time.Hour {
			return false, existing.Status, nil
		}
		// Pending > 24h: refresh timestamp.
		existing.RequestedAt = time.Now()
		store.Users[k] = existing
		return false, existing.Status, save(workspace, store)
	}
	store.Users[k] = Approval{
		UserID:      userID,
		Platform:    platform,
		Username:    username,
		FirstName:   firstName,
		Status:      StatusPending,
		RequestedAt: time.Now(),
	}
	return true, StatusPending, save(workspace, store)
}

// Approve ubah status user jadi approved. Idempotent — kalau sudah approved,
// tidak error. Kalau user belum pernah request, create entry baru (owner
// bisa whitelist preemptive).
func Approve(workspace, platform string, userID int64, approvedBy string) error {
	mu.Lock()
	defer mu.Unlock()
	store, _ := load(workspace)
	if store == nil {
		store = &storeFile{Version: 1, Users: map[string]Approval{}}
	}
	if store.Users == nil {
		store.Users = map[string]Approval{}
	}
	k := key(platform, userID)
	rec, ok := store.Users[k]
	if !ok {
		rec = Approval{UserID: userID, Platform: platform, Status: StatusApproved, RequestedAt: time.Now()}
	}
	rec.Status = StatusApproved
	rec.ApprovedAt = time.Now()
	rec.ApprovedBy = approvedBy
	store.Users[k] = rec
	return save(workspace, store)
}

// Deny ubah status jadi denied. Subsequent message dari user ini akan tetap
// di-reject tanpa generate request baru.
func Deny(workspace, platform string, userID int64, approvedBy string) error {
	mu.Lock()
	defer mu.Unlock()
	store, _ := load(workspace)
	if store == nil {
		return fmt.Errorf("approvals: store kosong, tidak ada yang perlu di-deny")
	}
	k := key(platform, userID)
	rec, ok := store.Users[k]
	if !ok {
		return fmt.Errorf("approvals: user %d (%s) tidak ditemukan", userID, platform)
	}
	rec.Status = StatusDenied
	rec.ApprovedAt = time.Now()
	rec.ApprovedBy = approvedBy
	store.Users[k] = rec
	return save(workspace, store)
}

// List semua approval sorted by RequestedAt desc. Untuk dashboard + /status.
func List(workspace string) []Approval {
	store, _ := load(workspace)
	if store == nil || store.Users == nil {
		return nil
	}
	out := make([]Approval, 0, len(store.Users))
	for _, a := range store.Users {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RequestedAt.After(out[j].RequestedAt) })
	return out
}

// Pending subset untuk prompt owner. "Butuh approval sekarang."
func Pending(workspace string) []Approval {
	all := List(workspace)
	out := make([]Approval, 0)
	for _, a := range all {
		if a.Status == StatusPending {
			out = append(out, a)
		}
	}
	return out
}

// ── Internal ────────────────────────────────────────────────────────────

func key(platform string, userID int64) string {
	return fmt.Sprintf("%s:%d", strings.ToLower(platform), userID)
}

func storePath(workspace string) string {
	return filepath.Join(workspace, "state", "approvals", "users.json")
}

func load(workspace string) (*storeFile, error) {
	path := storePath(workspace)
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &storeFile{Version: 1, Users: map[string]Approval{}}, nil
		}
		return nil, err
	}
	var s storeFile
	if err := json.Unmarshal(raw, &s); err != nil {
		// Corrupt file = treat as empty (fail-safe: never crash bot auth).
		return &storeFile{Version: 1, Users: map[string]Approval{}}, nil
	}
	if s.Users == nil {
		s.Users = map[string]Approval{}
	}
	return &s, nil
}

func save(workspace string, s *storeFile) error {
	s.Updated = time.Now()
	path := storePath(workspace)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("approvals: save: mkdir: %w", err)
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("approvals: save: marshal: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return fmt.Errorf("approvals: save: write file: %w", err)
	}
	return os.Rename(tmp, path)
}
