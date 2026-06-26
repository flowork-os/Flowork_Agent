// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package mitm

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/flowork-os/flowork_Router/internal/mitm/handlers"
)

type Server struct {
	addr    string
	cm      *CertManager
	handler http.Handler
	httpSrv *http.Server
}

func NewServer(addr string, cm *CertManager, handler http.Handler) *Server {
	if handler == nil {
		handler = defaultHandler()
	}
	return &Server{addr: addr, cm: cm, handler: handler}
}

func (s *Server) Start() error {
	if s.cm == nil {
		return errors.New("Server.cm is nil")
	}
	cfg := &tls.Config{
		MinVersion:     tls.VersionTLS12,
		GetCertificate: s.cm.GetCertificate,
	}
	s.httpSrv = &http.Server{
		Addr:              s.addr,
		Handler:           s.handler,
		TLSConfig:         cfg,
		ReadHeaderTimeout: 30 * time.Second,
	}
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", s.addr, err)
	}
	tlsLn := tls.NewListener(ln, cfg)
	return s.httpSrv.Serve(tlsLn)
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpSrv == nil {
		return nil
	}
	return s.httpSrv.Shutdown(ctx)
}

func defaultHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		tool := GetToolForHost(r.Host)
		if tool == "" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotImplemented)
			_, _ = w.Write([]byte(`{"error":"flow_router MITM: host not in TargetHosts","host":"` + r.Host + `"}`))
			return
		}
		h := handlers.Get(tool)
		if h == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotImplemented)
			_, _ = w.Write([]byte(`{"error":"flow_router MITM: no handler registered for tool","tool":"` + tool + `","host":"` + r.Host + `"}`))
			return
		}
		h.Handle(w, r)
	})
}
