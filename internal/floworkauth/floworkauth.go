// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-31
// Reason: Auth single-owner. Audit pass — bcrypt DefaultCost, session token
//   crypto/rand 256-bit, cookie HttpOnly+SameSite=Lax, no secret logging,
//   expired-session auto-purge. E2E verified (register/login/logout/change).
//
// Package floworkauth — single-owner password auth untuk flowork-gui.
//
// Model (sesuai keputusan owner): SATU owner (Mr.Dev). Register pertama =
// set password owner. Login = password doang → session cookie. Tidak ada
// multi-user, tidak ada verifikasi Telegram.
//
// Penyimpanan:
//   - password hash (bcrypt) → floworkdb.secrets[owner_password_hash]
//   - owner display name      → floworkdb.kv[owner_name]
//   - sesi aktif              → in-memory map (hilang saat restart, by design)
//
// Keamanan:
//   - bcrypt cost default (10) — lambat-on-purpose, anti brute force.
//   - token sesi: crypto/rand 32 byte → hex (256-bit, unguessable).
//   - cookie HttpOnly + SameSite=Lax (anti XSS read + CSRF dasar).
package floworkauth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"flowork-gui/internal/floworkdb"

	"golang.org/x/crypto/bcrypt"
)

const (
	// CookieName — nama cookie sesi.
	CookieName = "flowork_session"
	// sessionTTL — umur sesi.
	sessionTTL = 7 * 24 * time.Hour
	// keyPasswordHash — key di tabel secrets.
	keyPasswordHash = "owner_password_hash"
	// keyOwnerName — key di tabel kv.
	keyOwnerName = "owner_name"
	// RoleOwner — satu-satunya role di model single-owner.
	RoleOwner = "owner"
	// minPasswordLen — minimal panjang password.
	minPasswordLen = 6
)

// ErrNoOwner — belum ada owner ter-register (butuh setup).
var ErrNoOwner = errors.New("owner belum di-setup")

// session — entri sesi in-memory.
type session struct {
	name    string
	expires time.Time
}

// Manager — auth state. Di-back oleh floworkdb.Store untuk password hash.
type Manager struct {
	store    *floworkdb.Store
	mu       sync.Mutex
	sessions map[string]session
}

// NewManager bikin Manager.
func NewManager(store *floworkdb.Store) *Manager {
	return &Manager{store: store, sessions: map[string]session{}}
}

// HashPassword — bcrypt hash.
func HashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// IsSetup — apakah owner sudah ter-register.
func (m *Manager) IsSetup() bool {
	h, _ := m.store.GetSecret(keyPasswordHash)
	return strings.TrimSpace(h) != ""
}

// OwnerName — nama owner (default "Mr.Dev" kalau kosong).
func (m *Manager) OwnerName() string {
	n, _ := m.store.GetKV(keyOwnerName)
	if strings.TrimSpace(n) == "" {
		return "Mr.Dev"
	}
	return n
}

// Register — set password owner pertama kali. Error kalau owner sudah ada.
func (m *Manager) Register(name, password string) error {
	if m.IsSetup() {
		return errors.New("owner sudah terdaftar — pakai login")
	}
	if len(password) < minPasswordLen {
		return errors.New("password minimal 6 karakter")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Mr.Dev"
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	if err := m.store.SetSecret(keyPasswordHash, hash); err != nil {
		return err
	}
	return m.store.SetKV(keyOwnerName, name)
}

// ChangePassword — verifikasi old, set new. Butuh owner sudah setup.
func (m *Manager) ChangePassword(oldPw, newPw string) error {
	hash, err := m.store.GetSecret(keyPasswordHash)
	if err != nil {
		return err
	}
	if strings.TrimSpace(hash) == "" {
		return ErrNoOwner
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(oldPw)) != nil {
		return errors.New("password lama salah")
	}
	if len(newPw) < minPasswordLen {
		return errors.New("password baru minimal 6 karakter")
	}
	newHash, err := HashPassword(newPw)
	if err != nil {
		return err
	}
	return m.store.SetSecret(keyPasswordHash, newHash)
}

// Login — verifikasi password, bikin sesi, return (token, ownerName, error).
func (m *Manager) Login(password string) (string, string, error) {
	hash, err := m.store.GetSecret(keyPasswordHash)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(hash) == "" {
		return "", "", ErrNoOwner
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return "", "", errors.New("password salah")
	}
	token, err := newToken()
	if err != nil {
		return "", "", err
	}
	name := m.OwnerName()
	m.mu.Lock()
	m.sessions[token] = session{name: name, expires: time.Now().Add(sessionTTL)}
	m.mu.Unlock()
	return token, name, nil
}

// Logout — drop sesi by token.
func (m *Manager) Logout(token string) {
	if token == "" {
		return
	}
	m.mu.Lock()
	delete(m.sessions, token)
	m.mu.Unlock()
}

// Validate — cek token, return ownerName kalau valid. Auto-purge expired.
func (m *Manager) Validate(token string) (string, bool) {
	if token == "" {
		return "", false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[token]
	if !ok {
		return "", false
	}
	if time.Now().After(s.expires) {
		delete(m.sessions, token)
		return "", false
	}
	return s.name, true
}

// SessionFromRequest — ambil + validasi sesi dari cookie.
func (m *Manager) SessionFromRequest(r *http.Request) (string, bool) {
	c, err := r.Cookie(CookieName)
	if err != nil {
		return "", false
	}
	return m.Validate(c.Value)
}

// SetCookie — pasang cookie sesi.
func (m *Manager) SetCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(sessionTTL),
		MaxAge:   int(sessionTTL.Seconds()),
	})
}

// ClearCookie — hapus cookie sesi (logout).
func (m *Manager) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// newToken — 256-bit random hex token.
func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
