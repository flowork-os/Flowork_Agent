package loket

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// These tests lock the isolation guarantees the eternal kernel makes. Each one
// reproduces a concrete escape and asserts the kernel now refuses it.

// SSRF must not be bypassable via a redirect from an allowed host to an internal
// one. Binds the first hop on a non-loopback address so it passes the early check,
// then 302s to a loopback service the guard must still block at dial time.
func TestSSRF_RedirectToLoopbackBlocked(t *testing.T) {
	internalHit := false
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		internalHit = true
		_, _ = w.Write([]byte("SECRET-INTERNAL-DATA"))
	}))
	defer internal.Close()

	lanIP := firstNonLoopbackIPv4(t)
	ln, err := net.Listen("tcp", lanIP+":0")
	if err != nil {
		t.Skipf("cannot bind non-loopback addr %s: %v", lanIP, err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, internal.URL, http.StatusFound)
	})}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	args, _ := json.Marshal(map[string]any{"url": "http://" + ln.Addr().String() + "/"})
	_, _ = httpFetch(context.Background(), "mod", args)
	if internalHit {
		t.Fatal("SSRF guard bypassed: redirect reached the loopback service")
	}
}

// The guard must treat private ranges and the cloud-metadata endpoint as internal.
func TestSSRF_BlockedIPRanges(t *testing.T) {
	blocked := []string{"127.0.0.1", "169.254.169.254", "10.0.0.1", "192.168.1.1", "172.16.0.1", "::1", "fe80::1"}
	for _, h := range blocked {
		ip := net.ParseIP(h)
		if ip == nil {
			t.Fatalf("bad test ip %q", h)
		}
		if !isBlockedIP(ip) {
			t.Errorf("isBlockedIP(%s) = false; want true (SSRF target)", h)
		}
	}
	allowed := []string{"8.8.8.8", "1.1.1.1", "93.184.216.34"}
	for _, h := range allowed {
		if isBlockedIP(net.ParseIP(h)) {
			t.Errorf("isBlockedIP(%s) = true; want false (public host)", h)
		}
	}
}

// fs.* must not follow a symlink that leaves the module folder.
func TestFS_SymlinkEscapeBlocked(t *testing.T) {
	base := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("OUTSIDE-SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(base, "escape")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	sp := &syscallProviders{deps: Deps{ModuleDir: func(string) (string, error) { return base, nil }}}

	// read through the symlink — must be refused
	args, _ := json.Marshal(map[string]any{"path": "escape/secret.txt"})
	if out, err := sp.fsRead(context.Background(), "mod", args); err == nil {
		t.Errorf("fs.read followed symlink out of folder: %s", string(out))
	}
	// write through the symlink (new file under escaped parent) — must be refused
	args, _ = json.Marshal(map[string]any{"path": "escape/pwn.txt", "content": "x"})
	if _, err := sp.fsWrite(context.Background(), "mod", args); err == nil {
		t.Error("fs.write followed symlink out of folder")
	}
	// a normal in-folder path must still work
	args, _ = json.Marshal(map[string]any{"path": "ok.txt", "content": "hello"})
	if _, err := sp.fsWrite(context.Background(), "mod", args); err != nil {
		t.Errorf("in-folder write wrongly blocked: %v", err)
	}
}

// callerID must reject a malformed/empty id even with the right secret, and accept
// a well-formed one.
func TestCallerID_Validation(t *testing.T) {
	svc := &Service{loopbackSecret: "s"}
	mk := func(secret, caller string) *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/api/kernel/call", nil)
		r.Header.Set("X-Flowork-Secret", secret)
		r.Header.Set("X-Flowork-Caller", caller)
		return r
	}
	if got := svc.callerID(mk("s", "../etc")); got != "" {
		t.Errorf("accepted traversal-shaped id %q", got)
	}
	if got := svc.callerID(mk("s", "")); got != "" {
		t.Errorf("accepted empty id %q", got)
	}
	if got := svc.callerID(mk("wrong", "mr-flow")); got != "" {
		t.Errorf("accepted caller with wrong secret %q", got)
	}
	if got := svc.callerID(mk("s", "mr-flow-next")); got != "mr-flow-next" {
		t.Errorf("rejected valid id, got %q", got)
	}
}

func firstNonLoopbackIPv4(t *testing.T) string {
	t.Helper()
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Skipf("no interface addrs: %v", err)
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if v4 := ipnet.IP.To4(); v4 != nil {
				return v4.String()
			}
		}
	}
	t.Skip("no non-loopback IPv4 interface available")
	return ""
}
