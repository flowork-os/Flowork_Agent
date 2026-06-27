// Owner: Mr.Dev · github.com/flowork-os/Flowork-OS · floworkos.com
// ⚠️ FROZEN (siklus-hidup AI Studio — stabil). Dok: lock/ai-studio.md.
//
// deathletter.go — DEATH-LETTER / surat kematian kemampuan (ROADMAP_AI_STUDIO F3). Pas
// kemampuan DIBUANG (reap / uninstall) → catat APA yang mati + KENAPA, biar kemampuan/AI
// lain ga halu manggil yang udah ga ada. Store = 1 file JSON di samping AgentsDir.
//
// EXTENSIBLE (Rule #7) lewat SWITCH RegisterDeathObserver (POLA A): reaksi-pada-kematian
// BARU (mis. broadcast mesh, notif TG) didaftar via sibling init() TANPA nyentuh file ini.
// Best-effort, no-fatal: gagal nulis surat / observer panik ga boleh nggagalin pembuangan.
package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"flowork-gui/internal/kernel/loader"
)

// DeathLetter — 1 catatan kematian kemampuan.
type DeathLetter struct {
	ID     string   `json:"id"`               // id kategori/kemampuan
	Kind   string   `json:"kind"`             // category | app | tool | slash | scanner | channel
	Name   string   `json:"name"`             // nama human-readable
	Reason string   `json:"reason"`           // kenapa mati (reap broken / failing / manual)
	Agents []string `json:"agents,omitempty"` // agent yang ikut dibuang
	At     string   `json:"at"`               // RFC3339 waktu mati
}

const deathLetterMax = 200 // simpan N terakhir (anti-bengkak)

var deathLetterMu sync.Mutex

// deathObservers — SWITCH (POLA A). Dipanggil tiap kematian. Default KOSONG = no-op aman.
var deathObservers []func(DeathLetter)

// RegisterDeathObserver — daftarin reaksi-pada-kematian BARU via sibling init() (nil di-skip).
func RegisterDeathObserver(fn func(DeathLetter)) {
	if fn != nil {
		deathObservers = append(deathObservers, fn)
	}
}

// deathLettersPath — ~/.flowork/death-letters.json (di samping AgentsDir, mirror coder-pending).
func deathLettersPath() string {
	return filepath.Join(filepath.Dir(loader.AgentsDir()), "death-letters.json")
}

// listDeathLetters — baca semua surat (terbaru dulu). Best-effort (file ga ada = kosong).
func listDeathLetters() []DeathLetter {
	deathLetterMu.Lock()
	defer deathLetterMu.Unlock()
	return readDeathLettersLocked()
}

func readDeathLettersLocked() []DeathLetter {
	raw, err := os.ReadFile(deathLettersPath())
	if err != nil {
		return []DeathLetter{}
	}
	var out []DeathLetter
	if json.Unmarshal(raw, &out) != nil {
		return []DeathLetter{}
	}
	return out
}

// recordDeathLetter — catat 1 kematian (prepend = terbaru dulu, cap deathLetterMax) + notify
// observer (SWITCH). Best-effort: error nulis di-swallow (pembuangan ga boleh gagal).
func recordDeathLetter(id, kind, name, reason string, agents []string) {
	dl := DeathLetter{
		ID: id, Kind: kind, Name: name, Reason: reason, Agents: agents,
		At: time.Now().UTC().Format(time.RFC3339),
	}
	deathLetterMu.Lock()
	letters := append([]DeathLetter{dl}, readDeathLettersLocked()...)
	if len(letters) > deathLetterMax {
		letters = letters[:deathLetterMax]
	}
	if raw, err := json.MarshalIndent(letters, "", "  "); err == nil {
		_ = os.WriteFile(deathLettersPath(), raw, 0o644)
	}
	deathLetterMu.Unlock()
	for _, ob := range deathObservers {
		func() {
			defer func() { _ = recover() }() // observer panik ga boleh ngerusak pembuangan
			ob(dl)
		}()
	}
}

// studioDeathLettersHandler — GET /api/studio/deathletters → daftar surat kematian.
func studioDeathLettersHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		letters := listDeathLetters()
		tfWriteJSON(w, 0, map[string]any{"death_letters": letters, "total": len(letters)})
	}
}

// daftar route lewat seam RegisterFeature (registry frozen, registrasi = seam).
func init() {
	RegisterFeature(Feature{Name: "studio-deathletter", Phase: PhaseRoute, Apply: func(d *Deps) {
		d.Mux.HandleFunc("/api/studio/deathletters", studioDeathLettersHandler())
	}})
}
