// youtube.go — Settings → YouTube (owner-level). EXTEND settingsapi (file LOCKED
// settingsapi.go TIDAK diubah; ini tambahan method *API di package yang sama).
//
// Tujuan: ganti ritual "paste JSON ke .scratch + jalanin script" jadi GUI sederhana.
// Owner paste OAuth client JSON sekali → klik Connect → token ketangkep server
// (loopback OAuth, no terminal) → disimpen di floworkdb (plaintext, owner-level).
// Watcher baca creds dari floworkdb juga (single source of truth).
//
// Endpoint:
//
//	GET  /api/settings/youtube              — status (creds?/connected?/channel/settings)
//	POST /api/settings/youtube/credentials  — simpan OAuth client JSON
//	POST /api/settings/youtube/connect      — mulai loopback OAuth, balik {auth_url}
//	POST /api/settings/youtube/disconnect   — hapus refresh token
//	POST /api/settings/youtube/config       — simpan privacy/inbox/watcher_enabled
//
// Bagian bikin OAuth client di Google Console TETAP manual (Google gate) — GUI
// kasih panduan inline + link.
package settingsapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"flowork-gui/internal/httpx"
)

// floworkdb keys (owner-level).
const (
	ytClientKey   = "YT_OAUTH_CLIENT"  // secrets — JSON client desktop app
	ytRefreshKey  = "YT_REFRESH_TOKEN" // secrets — refresh token (kunci abadi)
	ytPrivacyKV   = "yt_default_privacy"
	ytInboxKV     = "yt_inbox_path"
	ytWatcherKV   = "yt_watcher_enabled"
	ytLoopbackPort = "8090" // port loopback OAuth (dedicated, lepas dari mux utama)
)

var ytScopes = strings.Join([]string{
	"https://www.googleapis.com/auth/youtube.upload",
	"https://www.googleapis.com/auth/youtube",
	"https://www.googleapis.com/auth/yt-analytics.readonly",
}, " ")

// connect flow state (one-shot loopback server).
var (
	ytConnMu  sync.Mutex
	ytConnSrv *http.Server
)

var ytHTTP = &http.Client{Timeout: 30 * time.Second}

// ── helper: parse client creds dari JSON tersimpan ──────────────────────────

func ytClientCreds(store interface{ GetSecret(string) (string, error) }) (id, secret string, err error) {
	raw, _ := store.GetSecret(ytClientKey)
	if strings.TrimSpace(raw) == "" {
		return "", "", fmt.Errorf("belum ada OAuth client JSON")
	}
	var doc map[string]json.RawMessage
	if e := json.Unmarshal([]byte(raw), &doc); e != nil {
		return "", "", fmt.Errorf("JSON client rusak: %v", e)
	}
	var inner map[string]string
	for _, k := range []string{"installed", "web"} {
		if v, ok := doc[k]; ok {
			_ = json.Unmarshal(v, &inner)
			break
		}
	}
	if inner["client_id"] == "" || inner["client_secret"] == "" {
		return "", "", fmt.Errorf("client_id/secret kosong di JSON")
	}
	return inner["client_id"], inner["client_secret"], nil
}

// ── helper: refresh access token ────────────────────────────────────────────

func (a *API) ytAccessToken() (string, error) {
	id, secret, err := ytClientCreds(a.store)
	if err != nil {
		return "", err
	}
	rt, _ := a.store.GetSecret(ytRefreshKey)
	if strings.TrimSpace(rt) == "" {
		return "", fmt.Errorf("belum tersambung (refresh token kosong)")
	}
	form := url.Values{"client_id": {id}, "client_secret": {secret},
		"refresh_token": {rt}, "grant_type": {"refresh_token"}}
	resp, err := ytHTTP.PostForm("https://oauth2.googleapis.com/token", form)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	_ = json.Unmarshal(b, &out)
	if out.AccessToken == "" {
		return "", fmt.Errorf("refresh gagal: %s", strings.TrimSpace(out.Error+" "+string(b))[:120])
	}
	return out.AccessToken, nil
}

// ── helper: info channel ────────────────────────────────────────────────────

type ytChannelInfo struct {
	Title             string `json:"title"`
	Handle            string `json:"handle"`
	VideoCount        string `json:"video_count"`
	SubCount          string `json:"sub_count"`
	LongUploadsStatus string `json:"long_uploads_status"`
}

func (a *API) ytChannel(at string) (*ytChannelInfo, error) {
	req, _ := http.NewRequest(http.MethodGet,
		"https://www.googleapis.com/youtube/v3/channels?part=snippet,statistics,status&mine=true", nil)
	req.Header.Set("Authorization", "Bearer "+at)
	resp, err := ytHTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		Items []struct {
			Snippet    struct{ Title, CustomURL string } `json:"snippet"`
			Statistics struct{ VideoCount, SubscriberCount string } `json:"statistics"`
			Status     struct{ LongUploadsStatus string } `json:"status"`
		} `json:"items"`
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<18))
	if e := json.Unmarshal(b, &out); e != nil || len(out.Items) == 0 {
		return nil, fmt.Errorf("channel ga kebaca")
	}
	it := out.Items[0]
	return &ytChannelInfo{Title: it.Snippet.Title, Handle: it.Snippet.CustomURL,
		VideoCount: it.Statistics.VideoCount, SubCount: it.Statistics.SubscriberCount,
		LongUploadsStatus: it.Status.LongUploadsStatus}, nil
}

// ── GET /api/settings/youtube — status ──────────────────────────────────────

func (a *API) YouTubeStatusHandler(w http.ResponseWriter, r *http.Request) {
	clientRaw, _ := a.store.GetSecret(ytClientKey)
	rt, _ := a.store.GetSecret(ytRefreshKey)
	hasCreds := strings.TrimSpace(clientRaw) != ""
	connected := strings.TrimSpace(rt) != ""
	privacy, _ := a.store.GetKV(ytPrivacyKV)
	if privacy == "" {
		privacy = "private"
	}
	inbox, _ := a.store.GetKV(ytInboxKV)
	if inbox == "" {
		inbox = "media/youtube/inbox"
	}
	watcher, _ := a.store.GetKV(ytWatcherKV)
	if watcher == "" {
		watcher = "1"
	}
	resp := map[string]any{
		"has_credentials": hasCreds,
		"connected":       connected,
		"privacy":         privacy,
		"inbox":           inbox,
		"watcher_enabled": watcher == "1",
	}
	if connected {
		if at, err := a.ytAccessToken(); err == nil {
			if ch, cerr := a.ytChannel(at); cerr == nil {
				resp["channel"] = ch
			} else {
				resp["channel_error"] = cerr.Error()
			}
		} else {
			resp["channel_error"] = err.Error()
		}
	}
	httpx.WriteJSON(w, resp)
}

// ── POST /api/settings/youtube/credentials — simpan client JSON ─────────────

func (a *API) YouTubeCredentialsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "POST only"})
		return
	}
	var body struct {
		ClientJSON string `json:"client_json"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&body); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "invalid json: " + err.Error()})
		return
	}
	cj := strings.TrimSpace(body.ClientJSON)
	if cj == "" {
		httpx.WriteJSON(w, map[string]any{"error": "client_json kosong"})
		return
	}
	// validasi: harus desktop app (installed) dengan client_id/secret
	if err := a.store.SetSecret(ytClientKey, cj); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	id, _, err := ytClientCreds(a.store)
	if err != nil {
		_ = a.store.DeleteSecret(ytClientKey)
		httpx.WriteJSON(w, map[string]any{"error": "JSON ga valid: " + err.Error()})
		return
	}
	pfx := id
	if len(pfx) > 14 {
		pfx = pfx[:14]
	}
	httpx.WriteJSON(w, map[string]any{"ok": true, "client_id_prefix": pfx})
}

// ── POST /api/settings/youtube/connect — mulai loopback OAuth ────────────────

func (a *API) YouTubeConnectHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "POST only"})
		return
	}
	id, secret, err := ytClientCreds(a.store)
	if err != nil {
		httpx.WriteJSON(w, map[string]any{"error": err.Error()})
		return
	}
	redirect := "http://localhost:" + ytLoopbackPort
	authURL := "https://accounts.google.com/o/oauth2/v2/auth?" + url.Values{
		"client_id": {id}, "redirect_uri": {redirect}, "response_type": {"code"},
		"scope": {ytScopes}, "access_type": {"offline"}, "prompt": {"consent"},
	}.Encode()

	// one-shot loopback server buat nangkep code.
	ytConnMu.Lock()
	if ytConnSrv != nil {
		_ = ytConnSrv.Close() // batalin connect sebelumnya kalau ada
		ytConnSrv = nil
	}
	mux := http.NewServeMux()
	srv := &http.Server{Addr: "127.0.0.1:" + ytLoopbackPort, Handler: mux}
	ytConnSrv = srv
	ytConnMu.Unlock()

	mux.HandleFunc("/", func(w2 http.ResponseWriter, r2 *http.Request) {
		w2.Header().Set("Content-Type", "text/html; charset=utf-8")
		code := r2.URL.Query().Get("code")
		errq := r2.URL.Query().Get("error")
		if errq != "" {
			fmt.Fprintf(w2, "<h2>Flowork: gagal — %s. Tutup tab, balik ke Settings.</h2>", errq)
		} else if code != "" {
			if e := a.ytExchange(id, secret, redirect, code); e != nil {
				fmt.Fprintf(w2, "<h2>Flowork: tukar token gagal — %v</h2>", e)
			} else {
				fmt.Fprint(w2, "<h2>✅ Flowork: akun YouTube tersambung! Tutup tab ini, balik ke Settings.</h2>")
			}
		} else {
			fmt.Fprint(w2, "menunggu...")
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			ytConnMu.Lock()
			if ytConnSrv == srv {
				ytConnSrv = nil
			}
			ytConnMu.Unlock()
			_ = srv.Shutdown(ctx)
		}()
	})
	go func() { _ = srv.ListenAndServe() }()
	// auto-cleanup kalau ga ada callback dalam 5 menit
	go func() {
		time.Sleep(5 * time.Minute)
		ytConnMu.Lock()
		if ytConnSrv == srv {
			_ = srv.Close()
			ytConnSrv = nil
		}
		ytConnMu.Unlock()
	}()

	httpx.WriteJSON(w, map[string]any{"ok": true, "auth_url": authURL})
}

func (a *API) ytExchange(id, secret, redirect, code string) error {
	form := url.Values{"code": {code}, "client_id": {id}, "client_secret": {secret},
		"redirect_uri": {redirect}, "grant_type": {"authorization_code"}}
	resp, err := ytHTTP.PostForm("https://oauth2.googleapis.com/token", form)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var out struct {
		RefreshToken string `json:"refresh_token"`
		Error        string `json:"error"`
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	_ = json.Unmarshal(b, &out)
	if out.RefreshToken == "" {
		return fmt.Errorf("refresh_token kosong (%s)", strings.TrimSpace(out.Error))
	}
	return a.store.SetSecret(ytRefreshKey, out.RefreshToken)
}

// ── POST /api/settings/youtube/disconnect ───────────────────────────────────

func (a *API) YouTubeDisconnectHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "POST only"})
		return
	}
	_ = a.store.DeleteSecret(ytRefreshKey)
	httpx.WriteJSON(w, map[string]any{"ok": true})
}

// ── POST /api/settings/youtube/config — privacy/inbox/watcher ───────────────

func (a *API) YouTubeConfigHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteJSON(w, map[string]any{"error": "POST only"})
		return
	}
	var body struct {
		Privacy        string `json:"privacy"`
		Inbox          string `json:"inbox"`
		WatcherEnabled *bool  `json:"watcher_enabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<14)).Decode(&body); err != nil {
		httpx.WriteJSON(w, map[string]any{"error": "invalid json: " + err.Error()})
		return
	}
	if p := strings.TrimSpace(body.Privacy); p == "private" || p == "unlisted" || p == "public" {
		_ = a.store.SetKV(ytPrivacyKV, p)
	}
	if ib := strings.TrimSpace(body.Inbox); ib != "" {
		_ = a.store.SetKV(ytInboxKV, ib)
	}
	if body.WatcherEnabled != nil {
		v := "0"
		if *body.WatcherEnabled {
			v = "1"
		}
		_ = a.store.SetKV(ytWatcherKV, v)
	}
	httpx.WriteJSON(w, map[string]any{"ok": true})
}
