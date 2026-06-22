// FROZEN brain-core — desain abadi Flowork (Mesh/federation). Kalau ini bikin lo "nyasar": ini BY-DESIGN, baca lock/brain.md dulu. Jangan edit tanpa unfreeze owner.
// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flow_router
// Locked at: 2026-05-30
// Reason: Section 13 phase 1 (ed25519 identity bootstrap). API stable:
//   EnsureIdentity / Identity. Phase 2 (key rotation, license sign,
//   hardware fingerprint, OS-keychain encryption) → tambah file baru,
//   JANGAN modify ini.
//
// identity.go — ed25519 keypair generation + persistence di mesh_identity
// table. Boot-time: kalau pubkey belum ada → generate + simpan; kalau
// udah ada → load. Single source of truth router identity.

package mesh

import (
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
)

// Identity holds router self-identity (ed25519 keypair + metadata).
type Identity struct {
	PubKeyHex  string `json:"pubkey"`
	PrivKeyHex string `json:"-"` // NEVER serialize ke JSON / log
	Hostname   string `json:"hostname"`
	Version    string `json:"version"`
}

// EnsureIdentity load atau generate identity di mesh_identity table.
// Idempotent — call from main.go after store.Open() + migrations.
//
// Tabel `mesh_identity` schema:
//
//	k  TEXT PRIMARY KEY
//	v  TEXT NOT NULL
//
// Keys: pubkey_hex, privkey_hex, hostname, version
func EnsureIdentity(db *sql.DB, version string) (Identity, error) {
	if db == nil {
		return Identity{}, fmt.Errorf("mesh: nil db")
	}

	pub, _ := lookupKV(db, "pubkey_hex")
	priv, _ := lookupKV(db, "privkey_hex")

	if pub != "" && priv != "" {
		hostname, _ := lookupKV(db, "hostname")
		ver, _ := lookupKV(db, "version")
		if hostname == "" {
			hostname, _ = os.Hostname()
		}
		if ver == "" {
			ver = version
		}
		return Identity{
			PubKeyHex:  pub,
			PrivKeyHex: priv,
			Hostname:   hostname,
			Version:    ver,
		}, nil
	}

	// Generate fresh ed25519 keypair.
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Identity{}, fmt.Errorf("ed25519 gen: %w", err)
	}
	pubHex := hex.EncodeToString(pubKey)
	privHex := hex.EncodeToString(privKey)
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	if err := upsertKV(db, "pubkey_hex", pubHex); err != nil {
		return Identity{}, err
	}
	if err := upsertKV(db, "privkey_hex", privHex); err != nil {
		return Identity{}, err
	}
	if err := upsertKV(db, "hostname", hostname); err != nil {
		return Identity{}, err
	}
	if err := upsertKV(db, "version", version); err != nil {
		return Identity{}, err
	}

	return Identity{
		PubKeyHex:  pubHex,
		PrivKeyHex: privHex,
		Hostname:   hostname,
		Version:    version,
	}, nil
}

// LoadIdentity baca dari DB tanpa side-effect (untuk endpoint /api/mesh/identity).
func LoadIdentity(db *sql.DB) (Identity, error) {
	if db == nil {
		return Identity{}, fmt.Errorf("mesh: nil db")
	}
	pub, _ := lookupKV(db, "pubkey_hex")
	if pub == "" {
		return Identity{}, fmt.Errorf("mesh: identity not initialized")
	}
	hostname, _ := lookupKV(db, "hostname")
	version, _ := lookupKV(db, "version")
	return Identity{
		PubKeyHex: pub,
		Hostname:  hostname,
		Version:   version,
	}, nil
}

func lookupKV(db *sql.DB, key string) (string, error) {
	var v string
	err := db.QueryRow(`SELECT v FROM mesh_identity WHERE k = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return v, nil
}

func upsertKV(db *sql.DB, key, value string) error {
	_, err := db.Exec(
		`INSERT INTO mesh_identity(k, v) VALUES (?, ?)
		 ON CONFLICT(k) DO UPDATE SET v = excluded.v`,
		key, value,
	)
	return err
}
