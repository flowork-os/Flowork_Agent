// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package handlers

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"time"
)

const maxRerouteBody = 50 << 20

var rerouteClient = &http.Client{Timeout: 10 * time.Minute}

func init() { Register(&antigravityHandler{}) }

type antigravityHandler struct{}

func (a *antigravityHandler) Name() string { return "antigravity" }

func (a *antigravityHandler) Handle(w http.ResponseWriter, r *http.Request) {
	rerouteToRouter(w, r, "/v1/chat/completions")
}

func rerouteToRouter(w http.ResponseWriter, r *http.Request, routerPath string) {
	defer r.Body.Close()
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxRerouteBody))
	if err != nil {
		var mb *http.MaxBytesError
		if errors.As(err, &mb) {
			http.Error(w, "request body exceeds limit", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "read request body", http.StatusBadRequest)
		return
	}

	target := "http://127.0.0.1:2402" + routerPath
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target, copyReader(body))
	if err != nil {
		http.Error(w, "build reroute request", http.StatusInternalServerError)
		return
	}
	req.Header = r.Header.Clone()
	req.Header.Del("Host")
	resp, err := rerouteClient.Do(req)
	if err != nil {
		http.Error(w, "upstream unreachable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func copyReader(b []byte) io.Reader {
	if len(b) == 0 {
		return http.NoBody
	}

	return bytes.NewReader(b)
}
