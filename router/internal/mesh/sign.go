// FROZEN brain-core — desain abadi Flowork (Mesh/federation). Kalau ini bikin lo "nyasar": ini BY-DESIGN, baca lock/brain.md dulu. Jangan edit tanpa unfreeze owner.
// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev) · Repo: https://github.com/flowork-os/Flowork-OS
// Locked at: 2026-06-17 · Reason: P2 fase-2a gerbang #1 (ed25519 sign/provenance),
//   owner-approved, E2E tested (sign→verify, tamper detected). File baru per lock-note
//   identity.go ("license sign → tambah file baru"). privkey TIDAK pernah keluar proses.
//
// sign.go — ed25519 detached-signature pakai identity router (Section 13 phase 2:
// "license sign" — file BARU per arahan lock-note identity.go, TIDAK edit identity.go).
//
// Dipakai P2 A2 fase-2a gerbang #1: PROVENANCE skill pack. Skill yang di-export/share
// ditandatangani dgn privkey instance → instance lain bisa verifikasi ASAL (pubkey) +
// INTEGRITAS (konten gak diubah) sebelum percaya. privkey TIDAK pernah keluar proses
// (sign di sini, verify cukup pakai pubkey publik).

package mesh

import (
	"crypto/ed25519"
	"database/sql"
	"encoding/hex"
	"fmt"
)

// SignData menandatangani data pakai privkey identity router. Return (sigHex, pubHex).
// Error kalau identity belum ke-bootstrap (EnsureIdentity belum jalan).
func SignData(db *sql.DB, data []byte) (sigHex, pubHex string, err error) {
	if db == nil {
		return "", "", fmt.Errorf("mesh: nil db")
	}
	privHex, perr := lookupKV(db, "privkey_hex")
	if perr != nil {
		return "", "", fmt.Errorf("mesh: read privkey: %w", perr)
	}
	pubHex, uerr := lookupKV(db, "pubkey_hex")
	if uerr != nil {
		return "", "", fmt.Errorf("mesh: read pubkey: %w", uerr)
	}
	if privHex == "" || pubHex == "" {
		return "", "", fmt.Errorf("mesh: identity not initialized")
	}
	priv, derr := hex.DecodeString(privHex)
	if derr != nil || len(priv) != ed25519.PrivateKeySize {
		return "", "", fmt.Errorf("mesh: bad private key")
	}
	sig := ed25519.Sign(ed25519.PrivateKey(priv), data)
	return hex.EncodeToString(sig), pubHex, nil
}

// VerifyData memverifikasi signature ed25519 atas data dengan pubkey hex.
// Pure (no DB) — verifikasi cuma butuh kunci publik. False kalau format salah.
func VerifyData(pubHex string, data []byte, sigHex string) bool {
	pub, e1 := hex.DecodeString(pubHex)
	sig, e2 := hex.DecodeString(sigHex)
	if e1 != nil || e2 != nil || len(pub) != ed25519.PublicKeySize || len(sig) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(ed25519.PublicKey(pub), data, sig)
}
