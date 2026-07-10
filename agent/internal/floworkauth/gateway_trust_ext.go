// gateway_trust_ext.go — TRUSTED-GATEWAY auth (owner 2026-07-10, MERGE GUI). NON-FROZEN sibling.
//
// Router (:2402) = SATU otoritas login. Router proxy /agent-api menandatangani tiap request
// dengan HMAC-SHA256 pakai SHARED SECRET (${FLOWORK_SIDECAR}/data/gateway.key). File ini
// memverifikasi tanda tangan itu → request dipercaya sbg owner (dipanggil dari loopbackAllowExt
// di allow_seam.go). Signature TIDAK bisa dipalsu JS/drive-by (butuh rahasia). Anti-replay: ts ±60s.
//
// Koloni HANYA membaca kunci (Router yang generate). Kunci absen / signature invalid → false
// (perilaku lama utuh = 401). Hapus file → trust mati, koloni tetap hidup.
package floworkauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	gwTrustOnce sync.Once
	gwTrustKey  []byte
)

func gatewayTrustKeyPath() string {
	if p := os.Getenv("FLOWORK_GW_KEY_FILE"); p != "" {
		return p
	}
	if sc := os.Getenv("FLOWORK_SIDECAR"); sc != "" {
		return filepath.Join(sc, "data", "gateway.key")
	}
	// Fallback exe-relative: <...>/FLowork_os/agent/bin/exe -> Documents/FLOWORK/SIDECAR/...
	if exe, err := os.Executable(); err == nil {
		cand := filepath.Join(filepath.Dir(exe), "..", "..", "..", "FLOWORK", "SIDECAR", "data", "gateway.key")
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	return ""
}

func gatewayTrustKey() []byte {
	gwTrustOnce.Do(func() {
		p := gatewayTrustKeyPath()
		if p == "" {
			return
		}
		if raw, err := os.ReadFile(p); err == nil {
			if b, err := hex.DecodeString(strings.TrimSpace(string(raw))); err == nil && len(b) >= 32 {
				gwTrustKey = b
			}
		}
	})
	return gwTrustKey
}

// gatewaySignatureValid — true kalau request bawa tanda tangan gateway yang sah (HMAC cocok +
// ts segar). Dipanggil loopbackAllowExt (allow_seam.go) sebagai jalur trust owner-via-gateway.
func gatewaySignatureValid(r *http.Request) bool {
	sig := r.Header.Get("X-Flowork-Gw-Sig")
	ts := r.Header.Get("X-Flowork-Gw-Ts")
	if sig == "" || ts == "" {
		return false
	}
	key := gatewayTrustKey()
	if len(key) == 0 {
		return false
	}
	tsi, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return false
	}
	if d := time.Now().Unix() - tsi; d > 60 || d < -60 {
		return false // anti-replay / clock skew guard
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(ts + "\n" + r.Method + "\n" + r.URL.Path))
	want := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(want), []byte(sig))
}
