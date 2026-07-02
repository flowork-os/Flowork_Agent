// antigravity_ext.go — WIRING plug-and-play Antigravity (NON-frozen, deletable).
// 📄 Dok: FLowork_os/lock/antigravity.md
//
// Prinsip owner (Flowork ABADI): fitur yg gantung pihak-ke-3 = COLOKAN. Hapus
// file ini → capture mati, executor balik header default, provider auto bisa
// dihapus dari GUI → core router UTUH. Google matiin Antigravity? cabut colokan.
//
// Nyambungin 2 seam:
//   1. handlers.AntigravityCaptureHook — pas app Antigravity lewat MITM, TANGKEP
//      Bearer + header client asli → persist (OAuth token + provider auto + header KV).
//   2. executors.AntigravityHeaderHook — pas mr-flow manggil gemini via executor,
//      SUNTIK header asli hasil capture + Bearer terfresh (biar lolos validasi Google).
// Switch: FLOWORK_ANTIGRAVITY_CAPTURE (default ON).
package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"

	"github.com/flowork-os/flowork_Router/internal/executors"
	"github.com/flowork-os/flowork_Router/internal/mitm/handlers"
	"github.com/flowork-os/flowork_Router/internal/store"
)

const (
	antigravityProviderID = "antigravity-auto"
	antigravityHeadersKV  = "antigravity:headers"
	antigravityModelsKV   = "antigravity:models" // set model hasil CONTEK dari traffic app (anti-hardcode)
)

var (
	antigravityLastTokMu sync.Mutex
	antigravityLastTok   string
)

var errAntigravityNoToken = errors.New("antigravity: belum ada token ke-capture (jalanin app Antigravity sekali lewat MITM)")

func antigravityCaptureEnabled() bool {
	v := strings.TrimSpace(os.Getenv("FLOWORK_ANTIGRAVITY_CAPTURE"))
	return v == "" || v == "1" || strings.EqualFold(v, "true") // default ON
}

func init() {
	handlers.AntigravityCaptureHook = persistAntigravityCreds
	executors.AntigravityHeaderHook = antigravityInjectHeaders
	// Colok loader token buat OAuth-import GUI (auto-import Antigravity dari
	// token yg udah ke-capture MITM). Papan di handlers_oauth.go.
	RegisterTokenLoader("antigravity", loadAntigravityToken)
}

// loadAntigravityToken — token terfresh hasil auto-capture MITM (buat GUI import).
func loadAntigravityToken() (string, error) {
	d, err := store.Open()
	if err != nil {
		return "", err
	}
	rec, err := store.GetOAuthToken(d, "antigravity")
	if err != nil || rec == nil || rec.AccessToken == "" {
		return "", errAntigravityNoToken
	}
	return rec.AccessToken, nil
}

// persistAntigravityCreds — dipanggil handler MITM tiap app manggil. Idempotent.
// ANTI-HARDCODE: token, header, DAN model semuanya di-CONTEK dari traffic app.
func persistAntigravityCreds(auth string, clientHeaders map[string]string, model string) {
	if !antigravityCaptureEnabled() {
		return
	}
	tok := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	if tok == "" {
		return
	}
	d, err := store.Open()
	if err != nil {
		return
	}
	// Model baru ke-contek → akumulasi ke set (walau token sama). Provider
	// dapet daftar model REAL yg app-nya pakai, bukan tebakan kode.
	modelsChanged := accumulateAntigravityModel(d, model)

	antigravityLastTokMu.Lock()
	tokChanged := tok != antigravityLastTok
	antigravityLastTok = tok
	antigravityLastTokMu.Unlock()
	if !tokChanged && !modelsChanged {
		return // ga ada yg baru → skip tulis DB (hemat I/O)
	}

	// 1) Simpan token (OAuth record) — sumber kebenaran token terfresh.
	_ = store.UpsertOAuthToken(d, &store.OAuthTokenRecord{
		Provider: "antigravity", AccessToken: tok, TokenType: "Bearer", Scope: "mitm-capture",
	})
	// 2) Simpan header client asli (JSON) → di-reuse executor.
	if len(clientHeaders) > 0 {
		if b, e := json.Marshal(clientHeaders); e == nil {
			_, _ = d.Exec(`INSERT INTO kv (k, v, updatedAt) VALUES (?, ?, datetime('now'))
				ON CONFLICT(k) DO UPDATE SET v=excluded.v, updatedAt=excluded.updatedAt`,
				antigravityHeadersKV, string(b))
		}
	}
	// 3) Auto-provision provider (models = hasil contek, bukan hardcode).
	ensureAntigravityProvider(d, tok)
}

// accumulateAntigravityModel — tambah model yg ke-contek ke set KV (dedup).
// Return true kalau ada model BARU. Anti-hardcode: daftar model tumbuh dari
// traffic app asli.
func accumulateAntigravityModel(d *sql.DB, model string) bool {
	model = strings.TrimSpace(model)
	if model == "" {
		return false
	}
	set := loadAntigravityModels(d)
	for _, m := range set {
		if m == model {
			return false
		}
	}
	set = append(set, model)
	if b, e := json.Marshal(set); e == nil {
		_, _ = d.Exec(`INSERT INTO kv (k, v, updatedAt) VALUES (?, ?, datetime('now'))
			ON CONFLICT(k) DO UPDATE SET v=excluded.v, updatedAt=excluded.updatedAt`,
			antigravityModelsKV, string(b))
	}
	return true
}

func loadAntigravityModels(d *sql.DB) []string {
	var v string
	if e := d.QueryRow(`SELECT v FROM kv WHERE k = ?`, antigravityModelsKV).Scan(&v); e != nil || v == "" {
		return nil
	}
	var set []string
	_ = json.Unmarshal([]byte(v), &set)
	return set
}

func ensureAntigravityProvider(d *sql.DB, tok string) {
	existing, _ := store.GetProvider(d, antigravityProviderID)
	// Model = hasil CONTEK dari traffic (anti-hardcode). Belum ada capture →
	// kosong (provider ga match apa2 sampai app jalan) — itu by-design.
	captured := loadAntigravityModels(d)
	models := make([]any, 0, len(captured))
	for _, m := range captured {
		models = append(models, m)
	}
	p := &store.ProviderConnection{
		ID:       antigravityProviderID,
		Provider: "antigravity",
		AuthType: store.AuthTypeAPIKey,
		Name:     "Antigravity (auto-capture)",
		Priority: 5,
		IsActive: true,
		Data: map[string]any{
			store.CfgAPIKey:  tok,
			store.CfgModels:  models,
			store.CfgBaseURL: "https://cloudcode-pa.googleapis.com",
		},
	}
	if existing != nil {
		// Pertahankan projectId / setting manual owner kalau ada.
		if pid, ok := existing.Data["projectId"]; ok {
			p.Data["projectId"] = pid
		}
		// CATATAN: model SENGAJA ga di-clobber dari existing — sumber kebenaran =
		// set hasil-contek (antigravity:models) yg tumbuh dari traffic (anti-hardcode).
		// Owner mau kunci model manual? edit KV antigravity:models langsung.
	}
	_ = store.UpsertProvider(d, p)
}

// antigravityInjectHeaders — SEAM executor: suntik header client ASLI + Bearer
// terfresh. base = header default frozen; captured menang (biar lolos Google).
func antigravityInjectHeaders(base map[string]string, p *store.ProviderConnection) map[string]string {
	if !antigravityCaptureEnabled() {
		return nil // pakai default frozen
	}
	d, err := store.Open()
	if err != nil {
		return nil
	}
	out := map[string]string{}
	for k, v := range base {
		out[k] = v
	}
	// Header client asli hasil capture.
	var v string
	if e := d.QueryRow(`SELECT v FROM kv WHERE k = ?`, antigravityHeadersKV).Scan(&v); e == nil && v != "" {
		var captured map[string]string
		if json.Unmarshal([]byte(v), &captured) == nil {
			for hk, hv := range captured {
				if hv != "" {
					out[hk] = hv
				}
			}
		}
	}
	// Bearer terfresh (OAuth record menang atas apiKey provider yg mungkin basi).
	if rec, e := store.GetOAuthToken(d, "antigravity"); e == nil && rec != nil && rec.AccessToken != "" {
		out["Authorization"] = "Bearer " + rec.AccessToken
	}
	return out
}
