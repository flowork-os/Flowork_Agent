// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const encPrefix = "enc:v1:"

var (
	secretKeyOnce  sync.Once
	secretKeyBytes []byte
)

func secretKey() []byte {
	secretKeyOnce.Do(func() {
		path := filepath.Join(dataDir(), "secret.key")
		if b, err := os.ReadFile(path); err == nil && len(b) >= 32 {
			secretKeyBytes = b[:32]
			return
		}
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {

			log.Printf("SECURITY WARNING: crypto/rand failed generating secret key (%v) — secrets-at-rest encryption is DEGRADED (zero key). Fix entropy source.", err)
			secretKeyBytes = make([]byte, 32)
			return
		}
		_ = os.MkdirAll(dataDir(), 0o700)
		_ = os.WriteFile(path, key, 0o600)
		secretKeyBytes = key
	})
	return secretKeyBytes
}

func gcm() (cipher.AEAD, error) {
	block, err := aes.NewCipher(secretKey())
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func EncryptSecret(plain string) string {
	if plain == "" || strings.HasPrefix(plain, encPrefix) {
		return plain
	}
	g, err := gcm()
	if err != nil {
		return plain
	}
	nonce := make([]byte, g.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return plain
	}
	ct := g.Seal(nonce, nonce, []byte(plain), nil)
	return encPrefix + base64.RawStdEncoding.EncodeToString(ct)
}

func DecryptSecret(stored string) string {
	if !strings.HasPrefix(stored, encPrefix) {
		return stored
	}
	raw, err := base64.RawStdEncoding.DecodeString(stored[len(encPrefix):])
	if err != nil {
		return stored
	}
	g, err := gcm()
	if err != nil || len(raw) < g.NonceSize() {
		return stored
	}
	nonce, ct := raw[:g.NonceSize()], raw[g.NonceSize():]
	pt, err := g.Open(nil, nonce, ct, nil)
	if err != nil {
		return stored
	}
	return string(pt)
}
