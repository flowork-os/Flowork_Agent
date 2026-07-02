// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// ⚠️ FROZEN 2026-07-02 (owner) — jangan edit. STABIL + live-tested (stub 2ms). 📄 Dok: lock/prompt-diet.md
// Evolusi TANPA buka file ini: (a) switch GUI FLOWORK_FILE_DEDUP, ATAU
// (b) sibling _ext BARU yang override seam `fileReadDedup` lagi (POLA B).
// file_dedup_ext.go — DEDUP FILE-READ (gape1 §C, "Tinggi TPM").
//
// AKAR: agent sering baca-ulang file yang SAMA & GA berubah (verifikasi berulang) →
// isi utuh dikirim ulang ke LLM tiap kali = kuota TPM kebuang buat data duplikat.
// Claude Code: FILE_UNCHANGED_STUB berdasarkan mtime. Di sini via seam `fileReadDedup`
// (file.go frozen): baca-ulang file dgn (mtime,size) sama oleh agent yang sama dalam
// jendela TTL → balikin STUB (head 600 char + arahan) alih-alih isi penuh.
//
// ANTI-JEBAKAN (beda dari Claude): mr-flow MRUNE hasil tool lama jadi placeholder —
// stub buta ("liat hasil sebelumnya") bisa nunjuk ke konten yang udah ke-prune → model
// buta. Makanya: (a) stub bawa HEAD isi file (tetep ada grounding), (b) arahan eksplisit
// pakai {"force":true} buat baca penuh, (c) entri kadaluarsa 10 menit (re-read lama =
// full lagi), (d) cache di-invalidate otomatis kalau mtime/size beda.
//
// SWITCH (GUI = kebenaran): FLOWORK_FILE_DEDUP (bool, default ON).
// File ini DIHAPUS → seam balik no-op (perilaku lama). Delete-test aman.

package builtins

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"flowork-gui/internal/tools"
)

const (
	fileDedupTTL      = 10 * time.Minute
	fileDedupHeadRune = 600
	// File kecil: stub+note bisa LEBIH GEDE dari isi aslinya → dedup cuma nyala
	// buat isi yang beneran layak dihemat.
	fileDedupMinRune = 1200
)

type fileDedupEntry struct {
	mtimeMs int64
	size    int64
	readAt  time.Time
}

var (
	fileDedupMu    sync.Mutex
	fileDedupSeen  = map[string]fileDedupEntry{} // key: agentID + "\x00" + rel
	fileDedupSweep time.Time
)

func fileDedupEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("FLOWORK_FILE_DEDUP"))) {
	case "0", "off", "false", "no":
		return false
	}
	return true
}

func truthyArg(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		s := strings.ToLower(strings.TrimSpace(t))
		return s == "1" || s == "true" || s == "on" || s == "yes"
	case float64:
		return t != 0
	}
	return false
}

func headRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func init() {
	fileReadDedup = func(ctx context.Context, args map[string]any, rel string, mtimeMs, size int64, content string) (map[string]any, bool) {
		if !fileDedupEnabled() || truthyArg(args["force"]) {
			return nil, false
		}
		if len([]rune(content)) <= fileDedupMinRune {
			return nil, false // kecil: kirim penuh lebih murah daripada stub+note
		}
		agent := tools.FromAgent(ctx)
		if agent == "" {
			agent = tools.FromCaller(ctx)
		}
		key := agent + "\x00" + rel
		now := time.Now()
		fileDedupMu.Lock()
		defer fileDedupMu.Unlock()
		// sweep ringan tiap ~5 menit biar map ga numpuk
		if now.Sub(fileDedupSweep) > 5*time.Minute {
			for k, e := range fileDedupSeen {
				if now.Sub(e.readAt) > fileDedupTTL {
					delete(fileDedupSeen, k)
				}
			}
			fileDedupSweep = now
		}
		prev, ok := fileDedupSeen[key]
		fileDedupSeen[key] = fileDedupEntry{mtimeMs: mtimeMs, size: size, readAt: now}
		if !ok || prev.mtimeMs != mtimeMs || prev.size != size || now.Sub(prev.readAt) > fileDedupTTL {
			return nil, false // baru / berubah / kelamaan → isi penuh (perilaku lama)
		}
		return map[string]any{
			"path":       rel,
			"unchanged":  true,
			"size_bytes": size,
			"content_head": headRunes(content, fileDedupHeadRune),
			"note": "FILE TIDAK BERUBAH sejak pembacaan terakhir lo (mtime sama) — isi penuh TIDAK dikirim ulang biar hemat. " +
				"Pakai isi dari hasil baca sebelumnya di context. Kalau hasil lama udah ke-prune / lo butuh isi penuh lagi: " +
				"panggil file_read sekali lagi dengan tambahan argumen {\"force\": true}.",
		}, true
	}
}
