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

import "net/http"

// AntigravityCaptureHook — SEAM: di-set router main (non-frozen) buat persist
// token+header. Default nil = ga nangkep apa-apa (aman).
var AntigravityCaptureHook func(auth string, clientHeaders map[string]string)

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
			AntigravityCaptureHook(auth, hdr)
		}
	}
	rerouteToRouter(w, r, "/v1/chat/completions")
}

func init() { Register(antigravityCaptureHandler{}) }
