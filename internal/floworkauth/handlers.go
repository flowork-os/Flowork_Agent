// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-31
// Reason: Auth HTTP handlers + middleware. Audit pass — MaxBytesReader body
//   cap, status code eksplisit, whitelist exact-path (anti future-handler
//   bypass) + asset prefix, /api/* → 401 JSON, HTML → redirect. E2E verified.
//
// handlers.go — HTTP endpoint + middleware untuk floworkauth.
//
// Endpoint (shape dicocokkan dengan web/login.html + web/register.html):
//
//	POST /api/auth/register        {name,password}      → 201 {ok,name,role}
//	POST /api/auth/login           {password}           → 200 {name,role} + cookie
//	GET  /api/auth/me                                   → {name,role,authenticated} | {setup_required}
//	POST /api/auth/logout                               → {ok}
//	POST /api/auth/change-password {old,new}            → {ok}   (butuh sesi)
package floworkauth

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
)

// writeJSON — JSON response dengan status code eksplisit.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// RegisterHandler — POST /api/auth/register. Single-owner: register pertama
// jadi owner; register berikutnya → 409.
func (m *Manager) RegisterHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	var body struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body"})
		return
	}
	if err := m.Register(body.Name, body.Password); err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "sudah terdaftar") {
			status = http.StatusConflict
		}
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"ok":   true,
		"name": m.OwnerName(),
		"role": RoleOwner,
	})
}

// LoginHandler — POST /api/auth/login.
func (m *Manager) LoginHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body"})
		return
	}
	token, name, err := m.Login(body.Password)
	if err != nil {
		status := http.StatusUnauthorized
		if err == ErrNoOwner {
			status = http.StatusNotFound // frontend bisa arahkan ke /register
		}
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	m.SetCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "role": RoleOwner})
}

// MeHandler — GET /api/auth/me.
func (m *Manager) MeHandler(w http.ResponseWriter, r *http.Request) {
	if !m.IsSetup() {
		writeJSON(w, http.StatusOK, map[string]any{"setup_required": true})
		return
	}
	name, ok := m.SessionFromRequest(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "not logged in"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":          name,
		"role":          RoleOwner,
		"authenticated": true,
	})
}

// LogoutHandler — POST /api/auth/logout. Idempotent.
func (m *Manager) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(CookieName); err == nil {
		m.Logout(c.Value)
	}
	m.ClearCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ChangePasswordHandler — POST /api/auth/change-password {old,new}. Butuh sesi.
func (m *Manager) ChangePasswordHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST only"})
		return
	}
	if _, ok := m.SessionFromRequest(r); !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "not logged in"})
		return
	}
	var body struct {
		Old string `json:"old"`
		New string `json:"new"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid body"})
		return
	}
	if err := m.ChangePassword(body.Old, body.New); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// Middleware — gate semua route kecuali whitelist. Pasang PALING LUAR.
//
//   - Cookie valid → lanjut.
//   - Cookie invalid + /api/* → 401 JSON.
//   - Cookie invalid + HTML  → 302 ke /register (kalau belum setup) atau
//     /login?next=<path>.
func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if isPublicPath(r) {
			next.ServeHTTP(w, r)
			return
		}
		if _, ok := m.SessionFromRequest(r); ok {
			next.ServeHTTP(w, r)
			return
		}
		// Not authenticated.
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "not logged in"})
			return
		}
		dest := "/login?next=" + r.URL.RequestURI()
		if !m.IsSetup() {
			dest = "/register"
		}
		http.Redirect(w, r, dest, http.StatusFound)
	})
}

// isPublicPath — request yang lewat tanpa cek sesi.
//
// Exact: halaman login/register + endpoint auth + liveness probe.
// Prefix: asset statik yang dibutuhkan halaman login/register (i18n, js, css,
// vendor). /tabs/ TIDAK public — cuma kebuka setelah login.
// Loopback-only: endpoint internal yang dipanggil WASM agent ke API-nya
// sendiri via hostNetFetch (fetchHistory + fetchSelfPrompt). Server bind
// 127.0.0.1, jadi loopback-only aman dari attacker remote.
func isPublicPath(r *http.Request) bool {
	path := r.URL.Path
	switch path {
	case "/login", "/login.html", "/register", "/register.html", "/favicon.ico":
		return true
	case "/api/auth/login", "/api/auth/register", "/api/auth/logout", "/api/auth/me":
		return true
	case "/api/system/health":
		return true
	}
	for _, p := range []string{"/js/", "/css/", "/i18n/", "/vendor/"} {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	// Agent self-call (loopback only): daemon WASM fetch konteks/self-prompt/
	// tool-specs buat di-inject ke LLM + EXEC tool via tools/run. Tanpa ini,
	// agent ga bisa inget history / pake tools (kena 401). Server bind 127.0.0.1
	// jadi loopback-only aman dari remote.
	switch path {
	case "/api/agents/interactions", "/api/agents/self-prompt/render", "/api/agents/tools/specs":
		return r.Method == http.MethodGet && isLocalRequest(r)
	case "/api/agents/tools/run":
		// POST — eksekusi tool. Tetep aman: SandboxRunV3 enforce capability +
		// rate + approval di dalam handler-nya.
		return r.Method == http.MethodPost && isLocalRequest(r)
	case "/api/taskflow/run", "/api/taskflow/category", "/api/taskflow/category/delete":
		// POST/GET — FASE 4/5 Category Task trigger + CRUD. Loopback-only
		// (Mr.Flow/scheduler/owner-local). Server bind 127.0.0.1 → aman remote.
		return isLocalRequest(r)
	case "/api/taskflow/categories", "/api/taskflow/runs", "/api/taskflow/run-detail",
		"/api/taskflow/schedules":
		return r.Method == http.MethodGet && isLocalRequest(r)
	case "/api/taskflow/schedule", "/api/taskflow/schedule/delete":
		return isLocalRequest(r)
	case "/api/plugins/install":
		// Install task pack — loopback-only (owner-local / CLI). Server bind
		// 127.0.0.1 → aman remote. Caps-consent = roadmap Phase 4.
		return r.Method == http.MethodPost && isLocalRequest(r)
	case "/api/mcp/config":
		return r.Method == http.MethodGet && isLocalRequest(r)
	case "/api/agents/skills", "/api/agents/skills/curate":
		// FASE 8 Curator — list/grade/curate skill per-agent (owner-local).
		return isLocalRequest(r)
	}
	return false
}

// isLocalRequest — true kalau request datang dari loopback (127.0.0.1/::1).
// RemoteAddr = TCP peer level, ngga bisa di-spoof client. JANGAN trust
// X-Forwarded-For di sini.
func isLocalRequest(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
