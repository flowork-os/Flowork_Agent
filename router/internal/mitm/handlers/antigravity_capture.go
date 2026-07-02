// antigravity_capture.go — SIBLING ext (deletable, NON-frozen): override handler
// MITM "antigravity" biar SEBELUM reroute, TANGKEP Bearer token + header client
// ASLI dari app Antigravity (yg lewat MITM) → di-reuse buat Flow-B (mr-flow manggil
// gemini via executor). Plug-and-play (prinsip abadi owner): hapus file ini →
// registry balik ke handler frozen (reroute polos), NOL capture, core utuh.
//
// init() file ini jalan SETELAH antigravity.go (urut nama file) → override entri
// registry "antigravity". rerouteToRouter dipanggil apa adanya (se-package).
// 📄 Dok: FLowork_os/lock/antigravity.md
package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

// AntigravityCaptureHook — SEAM: di-set router main (non-frozen) buat persist
// token+header+model. Default nil = ga nangkep apa-apa (aman). ANTI-HARDCODE:
// model di-CONTEK dari body request app (bukan dipatok di kode).
var AntigravityCaptureHook func(auth string, clientHeaders map[string]string, model string)

// antigravityCaptureHeaders — header client yg dicontek dari app asli (buat
// dipakai executor pas manggil Google, biar lolos validasi client Google).
var antigravityCaptureHeaders = []string{
	"User-Agent", "X-Client-Name", "X-Client-Version",
	"X-Ide-Version", "X-Client-Metadata", "X-Machine-Session-Id",
}

type antigravityCaptureHandler struct{}

func (antigravityCaptureHandler) Name() string { return "antigravity" }

func (antigravityCaptureHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if AntigravityCaptureHook != nil {
		if auth := r.Header.Get("Authorization"); auth != "" {
			hdr := map[string]string{}
			for _, k := range antigravityCaptureHeaders {
				if v := r.Header.Get(k); v != "" {
					hdr[k] = v
				}
			}
			// CONTEK model dari body (anti-hardcode). Body dibaca lalu DIKEMBALIIN
			// utuh biar reroute tetep jalan normal.
			model := ""
			if r.Body != nil {
				if raw, err := io.ReadAll(io.LimitReader(r.Body, 8<<20)); err == nil {
					_ = r.Body.Close()
					r.Body = io.NopCloser(bytes.NewReader(raw))
					var probe struct {
						Model string `json:"model"`
					}
					if json.Unmarshal(raw, &probe) == nil {
						model = probe.Model
					}
				}
			}
			AntigravityCaptureHook(auth, hdr, model)
		}
	}
	rerouteToRouter(w, r, "/v1/chat/completions")
}

func init() { Register(antigravityCaptureHandler{}) }
