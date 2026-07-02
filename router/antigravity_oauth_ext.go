// antigravity_oauth_ext.go — OAuth connector Antigravity (cara 9router), NON-frozen.
// 📄 Dok: FLowork_os/lock/antigravity.md
//
// KENAPA OAuth (bukan MITM): app Antigravity pakai DNS-over-HTTPS → /etc/hosts
// hijack ga ngefek, MITM mentok (known issue decolua/9router #1356). OAuth login
// langsung = robust + ga ganggu app.
//
// ANTI-HARDCODE (prinsip owner): client_id + client_secret di-BACA RUNTIME dari
// binary `language_server` app Antigravity (auto-discover path), BUKAN dipatok di
// kode. Google rotate? re-extract. Token disimpen di DB (OAuth record + KV).
// Plug-and-play: hapus file → connector ilang, core utuh.
//
// Flow: /api/oauth/antigravity/start → PKCE + auth URL + listener :51121 →
// user login Google → callback → exchange code → loadCodeAssist (project) → simpen.
// Executor pull token via antigravityValidToken (auto-refresh saat expired).
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/flowork-os/flowork_Router/internal/store"
)

const (
	antigravityAuthEndpoint   = "https://accounts.google.com/o/oauth2/v2/auth"
	antigravityTokenEndpoint  = "https://oauth2.googleapis.com/token"
	antigravityRedirectURI    = "http://localhost:51121/oauth-callback"
	antigravityCallbackAddr   = "127.0.0.1:51121"
	antigravityLoadCodeAssist = "https://cloudcode-pa.googleapis.com/v1internal:loadCodeAssist"
	antigravityRefreshKV      = "antigravity:refresh"
	antigravityExpiryKV       = "antigravity:expiry"
	antigravityProjectKV      = "antigravity:project"
	antigravitySecretKV       = "antigravity:secret"
	// default client_id resmi Antigravity — OVERRIDABLE via switch
	// FLOWORK_ANTIGRAVITY_CLIENT_ID (GUI). Bukan lock: default yg bisa diganti.
	defaultAntigravityClientID = "1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com"
)

// oauthHTTP — client buat token exchange/refresh + loadCodeAssist (timeout wajar).
var oauthHTTP = &http.Client{Timeout: 30 * time.Second}

func init() {
	// Colok token-provider ke seam antigravity_ext.go (decoupled — hapus file ini
	// → seam nil → core tetep build).
	antigravityTokenProvider = antigravityValidToken
	RegisterExtraRoute(func(mux *http.ServeMux) {
		mux.HandleFunc("/api/oauth/antigravity/start", AntigravityOAuthStartHandler)
		mux.HandleFunc("/api/oauth/antigravity/status", AntigravityOAuthStatusHandler)
	})
}

// AntigravityOAuthStatusHandler — GET: status koneksi (login? project? expiry?).
func AntigravityOAuthStatusHandler(w http.ResponseWriter, r *http.Request) {
	d, err := store.Open()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	rec, _ := store.GetOAuthToken(d, "antigravity")
	connected := rec != nil && rec.AccessToken != ""
	_, _, cfgErr := extractOAuthConfig()
	writeJSON(w, http.StatusOK, map[string]any{
		"connected":      connected,
		"project":        kvGet(d, antigravityProjectKV),
		"hasRefresh":     kvGet(d, antigravityRefreshKV) != "",
		"appDetected":    cfgErr == nil,
		"appDetectError": errStr(cfgErr),
	})
}

func errStr(e error) string {
	if e == nil {
		return ""
	}
	return e.Error()
}

var antigravityScopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/userinfo.profile",
	"https://www.googleapis.com/auth/cclog",
	"https://www.googleapis.com/auth/experimentsandconfigs",
}

// ── extract constant runtime (anti-hardcode) ────────────────────────────────

var (
	oauthCfgOnce sync.Once
	oauthCID     string
	oauthSecrets []string
)

func languageServerCandidates() []string {
	home, _ := os.UserHomeDir()
	var out []string
	if p := strings.TrimSpace(os.Getenv("FLOWORK_ANTIGRAVITY_BIN")); p != "" {
		out = append(out, p)
	}
	out = append(out,
		filepath.Join(home, "Downloads", "Antigravity-x64", "resources", "bin", "language_server"),
		filepath.Join(home, ".local", "share", "Antigravity", "resources", "bin", "language_server"),
		"/opt/Antigravity/resources/bin/language_server",
		"/usr/share/antigravity/resources/bin/language_server",
	)
	return out
}

var (
	cidRe    = regexp.MustCompile(`[0-9]{6,}-[a-z0-9]{20,}\.apps\.googleusercontent\.com`)
	secretRe = regexp.MustCompile(`GOCSPX-[A-Za-z0-9_-]{28}`)
)

func extractOAuthConfig() (string, []string, error) {
	oauthCfgOnce.Do(func() {
		// client_id: switch GUI menang (data editable owner). Kosong → default
		// resmi Antigravity (OVERRIDABLE — pola switch: ada default, bisa diganti
		// GUI kalau Google rotate). Binary punya BANYAK client_id (gemini-cli/SDK)
		// yg ga bisa dibedain byte-distance → anchor di config, bukan tebak.
		oauthCID = strings.TrimSpace(os.Getenv("FLOWORK_ANTIGRAVITY_CLIENT_ID"))
		if oauthCID == "" {
			oauthCID = defaultAntigravityClientID
		}
		// secret: di-extract dari binary app (ga ada di tempat lain). Coba semua.
		for _, p := range languageServerCandidates() {
			data, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			cid, secrets := extractOAuthFromBytes(data)
			if len(secrets) > 0 {
				oauthSecrets = secrets
			}
			// Kalau switch kosong, pakai hasil extract (best-effort) sbg fallback.
			if oauthCID == "" && cid != "" {
				oauthCID = cid
			}
			if len(oauthSecrets) > 0 {
				return
			}
		}
	})
	if oauthCID == "" || len(oauthSecrets) == 0 {
		return "", nil, fmt.Errorf("OAuth Antigravity ga siap — app ke-install? client_id via switch FLOWORK_ANTIGRAVITY_CLIENT_ID, secret di-extract dari language_server (set FLOWORK_ANTIGRAVITY_BIN kalau path beda)")
	}
	return oauthCID, oauthSecrets, nil
}

// extractOAuthFromBytes — binary bisa punya BANYAK client_id (gemini-cli, sdk,
// dst). Yang BENER buat Antigravity = client_id yg KO-LOKASI (deket) sama secret
// GOCSPX. Anti-salah-ambil: pilih client_id terdekat ke secret pertama; kumpulin
// semua secret. Fallback: client_id pertama kalau ga ada yg deket.
func extractOAuthFromBytes(data []byte) (string, []string) {
	secLocs := secretRe.FindAllIndex(data, -1)
	seen := map[string]bool{}
	var secrets []string
	for _, loc := range secLocs {
		s := string(data[loc[0]:loc[1]])
		if !seen[s] {
			seen[s] = true
			secrets = append(secrets, s)
		}
	}
	cidLocs := cidRe.FindAllIndex(data, -1)
	if len(cidLocs) == 0 || len(secLocs) == 0 {
		// fallback: apa adanya
		cid := ""
		if len(cidLocs) > 0 {
			cid = string(data[cidLocs[0][0]:cidLocs[0][1]])
		}
		return cid, secrets
	}
	// client_id dgn jarak MINIMUM ke secret mana pun = pasangan Antigravity.
	bestCID, bestDist := "", 1<<62
	for _, c := range cidLocs {
		cidVal := string(data[c[0]:c[1]])
		for _, s := range secLocs {
			dist := c[0] - s[1]
			if dist < 0 {
				dist = s[0] - c[1]
			}
			if dist < 0 {
				dist = -dist
			}
			if dist < bestDist {
				bestDist, bestCID = dist, cidVal
			}
		}
	}
	return bestCID, secrets
}

// ── PKCE + state + callback server ──────────────────────────────────────────

type oauthPending struct {
	verifier string
	created  time.Time
}

var (
	oauthPendMu sync.Mutex
	oauthPend   = map[string]oauthPending{}
	oauthSrvMu  sync.Mutex
	oauthSrv    *http.Server
)

func randB64(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func openBrowser(u string) {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		c = exec.Command("open", u)
	case "windows":
		c = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		c = exec.Command("xdg-open", u)
	}
	_ = c.Start()
}

// AntigravityOAuthStartHandler — GET: mulai OAuth, balikin authUrl.
func AntigravityOAuthStartHandler(w http.ResponseWriter, r *http.Request) {
	cid, _, err := extractOAuthConfig()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": err.Error()})
		return
	}
	verifier := randB64(48)
	state := randB64(16)
	oauthPendMu.Lock()
	oauthPend[state] = oauthPending{verifier: verifier, created: time.Now()}
	oauthPendMu.Unlock()

	startAntigravityCallbackServer()

	q := url.Values{}
	q.Set("client_id", cid)
	q.Set("response_type", "code")
	q.Set("redirect_uri", antigravityRedirectURI)
	q.Set("scope", strings.Join(antigravityScopes, " "))
	q.Set("code_challenge", pkceChallenge(verifier))
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	q.Set("access_type", "offline")
	q.Set("prompt", "consent")
	authURL := antigravityAuthEndpoint + "?" + q.Encode()

	go openBrowser(authURL)
	writeJSON(w, http.StatusOK, map[string]any{
		"authUrl": authURL,
		"hint":    "buka authUrl di browser, login Google + izinin. Otomatis balik ke :51121.",
	})
}

func startAntigravityCallbackServer() {
	oauthSrvMu.Lock()
	defer oauthSrvMu.Unlock()
	if oauthSrv != nil {
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth-callback", antigravityOAuthCallback)
	oauthSrv = &http.Server{Addr: antigravityCallbackAddr, Handler: mux}
	go func() { _ = oauthSrv.ListenAndServe() }()
}

func stopAntigravityCallbackServer() {
	oauthSrvMu.Lock()
	defer oauthSrvMu.Unlock()
	if oauthSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_ = oauthSrv.Shutdown(ctx)
		cancel()
		oauthSrv = nil
	}
}

func antigravityOAuthCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	oauthPendMu.Lock()
	pend, ok := oauthPend[state]
	delete(oauthPend, state)
	oauthPendMu.Unlock()
	if !ok || code == "" {
		http.Error(w, "state/code invalid", http.StatusBadRequest)
		return
	}
	if err := exchangeAntigravityCode(code, pend.verifier); err != nil {
		http.Error(w, "token exchange gagal: "+err.Error(), http.StatusBadGateway)
		return
	}
	_, _ = w.Write([]byte("<html><body style='font-family:sans-serif;background:#111;color:#0f0;padding:40px'>" +
		"<h2>✅ Antigravity connected ke Flowork</h2><p>Token ke-simpen. Balik ke Flowork, tutup tab ini.</p></body></html>"))
	go func() { time.Sleep(2 * time.Second); stopAntigravityCallbackServer() }()
}

// ── token exchange + refresh ────────────────────────────────────────────────

type oauthTokenResp struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func postOAuthToken(form url.Values) (*oauthTokenResp, error) {
	req, _ := http.NewRequest(http.MethodPost, antigravityTokenEndpoint, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := oauthHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var tr oauthTokenResp
	if e := json.Unmarshal(body, &tr); e != nil {
		return nil, fmt.Errorf("decode token resp: %w", e)
	}
	if tr.Error != "" {
		return nil, fmt.Errorf("%s: %s", tr.Error, tr.ErrorDesc)
	}
	return &tr, nil
}

func exchangeAntigravityCode(code, verifier string) error {
	cid, secrets, err := extractOAuthConfig()
	if err != nil {
		return err
	}
	var lastErr error
	for _, sec := range secrets {
		form := url.Values{}
		form.Set("client_id", cid)
		form.Set("client_secret", sec)
		form.Set("code", code)
		form.Set("grant_type", "authorization_code")
		form.Set("redirect_uri", antigravityRedirectURI)
		form.Set("code_verifier", verifier)
		tr, e := postOAuthToken(form)
		if e != nil {
			lastErr = e
			continue
		}
		return storeAntigravityToken(tr, sec)
	}
	return fmt.Errorf("semua client_secret gagal: %w", lastErr)
}

func kvSet(d *sql.DB, k, v string) {
	_, _ = d.Exec(`INSERT INTO kv (k, v, updatedAt) VALUES (?, ?, datetime('now'))
		ON CONFLICT(k) DO UPDATE SET v=excluded.v, updatedAt=excluded.updatedAt`, k, v)
}

func kvGet(d *sql.DB, k string) string {
	var v string
	_ = d.QueryRow(`SELECT v FROM kv WHERE k=?`, k).Scan(&v)
	return v
}

func storeAntigravityToken(tr *oauthTokenResp, workingSecret string) error {
	d, err := store.Open()
	if err != nil {
		return err
	}
	_ = store.UpsertOAuthToken(d, &store.OAuthTokenRecord{
		Provider: "antigravity", AccessToken: tr.AccessToken, RefreshToken: tr.RefreshToken,
		TokenType: "Bearer", Scope: "oauth-login",
	})
	kvSet(d, antigravityExpiryKV, strconv.FormatInt(time.Now().Add(time.Duration(tr.ExpiresIn)*time.Second).Unix(), 10))
	if tr.RefreshToken != "" {
		kvSet(d, antigravityRefreshKV, tr.RefreshToken)
	}
	kvSet(d, antigravitySecretKV, workingSecret)
	if proj := loadAntigravityProject(tr.AccessToken); proj != "" {
		kvSet(d, antigravityProjectKV, proj)
	}
	ensureAntigravityProvider(d, tr.AccessToken)
	return nil
}

// antigravityValidToken — access token valid (auto-refresh saat expired).
func antigravityValidToken() (string, error) {
	d, err := store.Open()
	if err != nil {
		return "", err
	}
	rec, _ := store.GetOAuthToken(d, "antigravity")
	if rec == nil || rec.AccessToken == "" {
		return "", fmt.Errorf("antigravity belum login (OAuth)")
	}
	exp, _ := strconv.ParseInt(kvGet(d, antigravityExpiryKV), 10, 64)
	if exp > 0 && time.Now().Unix() < exp-60 {
		return rec.AccessToken, nil
	}
	if newTok, e := refreshAntigravityToken(d); e == nil && newTok != "" {
		return newTok, nil
	}
	return rec.AccessToken, nil
}

func refreshAntigravityToken(d *sql.DB) (string, error) {
	refresh := kvGet(d, antigravityRefreshKV)
	if refresh == "" {
		return "", fmt.Errorf("no refresh token")
	}
	cid, secrets, err := extractOAuthConfig()
	if err != nil {
		return "", err
	}
	// Pakai secret yg kerja dulu; fallback coba semua.
	trySecrets := secrets
	if s := kvGet(d, antigravitySecretKV); s != "" {
		trySecrets = append([]string{s}, secrets...)
	}
	var lastErr error
	for _, sec := range trySecrets {
		form := url.Values{}
		form.Set("client_id", cid)
		form.Set("client_secret", sec)
		form.Set("refresh_token", refresh)
		form.Set("grant_type", "refresh_token")
		tr, e := postOAuthToken(form)
		if e != nil {
			lastErr = e
			continue
		}
		_ = store.UpsertOAuthToken(d, &store.OAuthTokenRecord{
			Provider: "antigravity", AccessToken: tr.AccessToken, RefreshToken: refresh,
			TokenType: "Bearer", Scope: "oauth-login",
		})
		kvSet(d, antigravityExpiryKV, strconv.FormatInt(time.Now().Add(time.Duration(tr.ExpiresIn)*time.Second).Unix(), 10))
		kvSet(d, antigravitySecretKV, sec)
		return tr.AccessToken, nil
	}
	return "", fmt.Errorf("refresh gagal: %w", lastErr)
}

// loadAntigravityProject — onboarding project id (loadCodeAssist). Best-effort.
func loadAntigravityProject(accessTok string) string {
	body := []byte(`{"metadata":{"ideType":"IDE_UNSPECIFIED","platform":"PLATFORM_UNSPECIFIED","pluginType":"GEMINI"}}`)
	req, _ := http.NewRequest(http.MethodPost, antigravityLoadCodeAssist, strings.NewReader(string(body)))
	req.Header.Set("Authorization", "Bearer "+accessTok)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "antigravity")
	req.Header.Set("X-Goog-Api-Client", "google-cloud-sdk vscode_cloudshelleditor/0.1")
	resp, err := oauthHTTP.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var parsed struct {
		CloudaicompanionProject string `json:"cloudaicompanionProject"`
	}
	_ = json.Unmarshal(raw, &parsed)
	return parsed.CloudaicompanionProject
}
