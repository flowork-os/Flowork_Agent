// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
)

type AuthSession struct {
	ID         string    `json:"id"`
	Token      string    `json:"token,omitempty"`
	UserID     string    `json:"userId"`
	CreatedAt  time.Time `json:"createdAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
	LastSeenAt time.Time `json:"lastSeenAt,omitempty"`
	IP         string    `json:"ip,omitempty"`
	UserAgent  string    `json:"userAgent,omitempty"`
}

const defaultSessionTTL = 7 * 24 * time.Hour

func CreateSession(d *sql.DB, userID, ip, ua string) (*AuthSession, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	s := &AuthSession{
		ID:        uuid.NewString(),
		Token:     hex.EncodeToString(tokenBytes),
		UserID:    userID,
		CreatedAt: now,
		ExpiresAt: now.Add(defaultSessionTTL),
		IP:        ip,
		UserAgent: ua,
	}
	_, err := d.Exec(`INSERT INTO authSessions (id, token, userId, createdAt, expiresAt, lastSeenAt, ip, userAgent)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.Token, s.UserID, s.CreatedAt.Format(time.RFC3339), s.ExpiresAt.Format(time.RFC3339), s.CreatedAt.Format(time.RFC3339), s.IP, s.UserAgent)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func GetSessionByToken(d *sql.DB, token string) (*AuthSession, error) {
	row := d.QueryRow(`SELECT id, token, COALESCE(userId, ''), createdAt, expiresAt, COALESCE(lastSeenAt, ''), COALESCE(ip, ''), COALESCE(userAgent, '') FROM authSessions WHERE token = ?`, token)
	var s AuthSession
	var ca, ea, la string
	err := row.Scan(&s.ID, &s.Token, &s.UserID, &ca, &ea, &la, &s.IP, &s.UserAgent)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	s.CreatedAt, _ = time.Parse(time.RFC3339, ca)
	s.ExpiresAt, _ = time.Parse(time.RFC3339, ea)
	if la != "" {
		s.LastSeenAt, _ = time.Parse(time.RFC3339, la)
	}
	if time.Now().UTC().After(s.ExpiresAt) {
		_, _ = d.Exec(`DELETE FROM authSessions WHERE id = ?`, s.ID)
		return nil, nil
	}
	return &s, nil
}

func TouchSession(d *sql.DB, id string) error {
	_, err := d.Exec(`UPDATE authSessions SET lastSeenAt = ? WHERE id = ?`, time.Now().UTC().Format(time.RFC3339), id)
	return err
}

func DeleteSession(d *sql.DB, token string) error {
	_, err := d.Exec(`DELETE FROM authSessions WHERE token = ?`, token)
	return err
}

func PurgeExpiredSessions(d *sql.DB) error {
	_, err := d.Exec(`DELETE FROM authSessions WHERE expiresAt < ?`, time.Now().UTC().Format(time.RFC3339))
	return err
}
