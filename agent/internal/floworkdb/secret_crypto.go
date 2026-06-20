// === LOCKED FILE (soft) === Status: STABLE — owner-approved 2026-06-20 (fix kritis enkripsi).
// LOCKED ≠ FREEZE (boleh diedit dgn izin owner). Reason: enkripsi-at-rest kredensial. Jangan
// ubah format enc:v1:/exclude-list/fail-safe tanpa izin — salah = secret unreadable / lockout.
package floworkdb

// secret_crypto.go — enkripsi-at-rest buat tabel `secrets` (owner 2026-06-20: "fix kritis").
//
// Threat yang DILINDUNGI: file flowork.db ke-copy/ke-backup SENDIRIAN (tanpa key file) →
// token/API-key/privkey di dalemnya ga kebaca (ciphertext). Threat yang TIDAK terlindungi:
// kalau attacker dapet SELURUH folder data (DB + .secret_key sekaligus) → bisa decrypt.
// Buat itu butuh key di luar (OS keyring / HSM) — trade-off vs auto-push otonom yg butuh
// key tanpa interaksi owner. Pilihan: key file chmod 0600 sebelah DB (pragmatis, jujur).
//
// FAIL-SAFE (jangan sampai nge-lock-out / crash): key ga bisa dibuat/dibaca → enkripsi
// MATI, secret disimpan plaintext (passthrough). Backward-compat: value lama tanpa prefix
// `enc:v1:` dibaca apa adanya. owner_password_hash SENGAJA ga dienkripsi (one-way hash,
// enkripsi ga nambah keamanan + satu-satunya jalur lockout) — lihat SetSecret/GetSecret.

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const secretEncPrefix = "enc:v1:"

// plaintextSecretKeys — key yang SENGAJA ga dienkripsi (anti-lockout). owner_password_hash
// udah one-way hash; enkripsi ga nambah proteksi tapi nambah risiko ke-lock-out kalau key
// file ilang. Sumber nama: floworkauth.keyPasswordHash.
var plaintextSecretKeys = map[string]bool{"owner_password_hash": true}

var (
	secretKeyOnce sync.Once
	secretKey     []byte // 32 byte; nil = enkripsi MATI (fail-safe plaintext)
)

func loadSecretKey() []byte {
	secretKeyOnce.Do(func() {
		path := filepath.Join(filepath.Dir(Path()), ".secret_key")
		if b, err := os.ReadFile(path); err == nil && len(b) == 32 {
			secretKey = b
			return
		}
		// Generate sekali. JANGAN pakai key non-persisten (kalau gagal nulis = value
		// ke-enkripsi bakal unreadable abis restart) → kalau gagal persist, enkripsi MATI.
		k := make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, k); err != nil {
			return
		}
		if err := os.WriteFile(path, k, 0o600); err != nil {
			return
		}
		secretKey = k
	})
	return secretKey
}

// secretEnc — plaintext → `enc:v1:<base64(nonce||ciphertext)>`. Key mati / value kosong → passthrough.
func secretEnc(plain string) string {
	key := loadSecretKey()
	if key == nil || plain == "" || strings.HasPrefix(plain, secretEncPrefix) {
		return plain
	}
	gcm, err := newGCM(key)
	if err != nil {
		return plain
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return plain
	}
	ct := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return secretEncPrefix + base64.StdEncoding.EncodeToString(ct)
}

// secretDec — kebalikan secretEnc. Tanpa prefix = plaintext lama (passthrough). Prefix tapi
// key ilang / korup → "" (secret jadi unusable, TAPI ga crash; auth-hash ga kena krn di-exclude).
func secretDec(stored string) string {
	if !strings.HasPrefix(stored, secretEncPrefix) {
		return stored
	}
	key := loadSecretKey()
	if key == nil {
		return ""
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, secretEncPrefix))
	if err != nil {
		return ""
	}
	gcm, err := newGCM(key)
	if err != nil {
		return ""
	}
	ns := gcm.NonceSize()
	if len(raw) < ns {
		return ""
	}
	pt, err := gcm.Open(nil, raw[:ns], raw[ns:], nil)
	if err != nil {
		return ""
	}
	return string(pt)
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
