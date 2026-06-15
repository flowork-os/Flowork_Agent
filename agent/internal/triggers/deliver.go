// === LOCKED FILE (soft) === Status: STABLE (owner-approved 2026-06-15, R4). E2E verified:
// trigger fire → deliver "chat" via registry → pesan masuk chat_session (format identik pra-R4),
// run status=ok. LOCKED≠FREEZE. Extend = RegisterDeliverer, jangan edit core.
//
// deliver.go — R4 EXTENSION POINT: channel pengiriman hasil trigger yang PLUG-ABLE.
// Owner-approved 2026-06-15 (refactor konsolidasi R4). Dulu: switch inline di engine.go
// runAction (jalur-panas LOCKED). Sekarang: REGISTRY — nambah channel = RegisterDeliverer,
// JANGAN edit engine (sejalan konvensi "tambah TIPE = file baru" di engine.go).
// Builtin "telegram"+"chat" di-register di init() — behavior IDENTIK pra-R4 (backward-compatible).
package triggers

import (
	"context"
	"encoding/json"
	"strings"

	"flowork-gui/internal/floworkdb"
)

// Deliverer kirim reply sebuah trigger ke SATU destinasi (telegram/chat/…). Dikasih akses
// Engine (Notify + Store). Return error → dicatat di errText trigger run (status tetap "ok").
type Deliverer func(ctx context.Context, e *Engine, r floworkdb.Trigger, reply string) error

// deliverers = registry channel by-kind. Diisi init() (builtin) + RegisterDeliverer (eksternal).
var deliverers = map[string]Deliverer{}

// RegisterDeliverer daftarin/override channel pengiriman. Last-writer-wins. Titik-extend
// resmi R4: nambah channel TANPA nyentuh jalur-panas engine.go.
func RegisterDeliverer(kind string, fn Deliverer) {
	if kind = strings.TrimSpace(kind); kind != "" && fn != nil {
		deliverers[kind] = fn
	}
}

// dispatchDeliver kirim reply ke tiap channel (comma-separated) di r.Deliver via registry.
// Return errText channel TERAKHIR yang gagal ("" = semua OK). Channel tak dikenal = skip
// (sama kayak default switch pra-R4).
func dispatchDeliver(ctx context.Context, e *Engine, r floworkdb.Trigger, reply string) string {
	errText := ""
	for _, d := range strings.Split(r.Deliver, ",") {
		kind := strings.TrimSpace(d)
		if kind == "" {
			continue
		}
		fn := deliverers[kind]
		if fn == nil {
			continue // channel tak dikenal → skip (preserve behavior)
		}
		if derr := fn(ctx, e, r, reply); derr != nil {
			errText = "deliver " + kind + ": " + derr.Error()
		}
	}
	return errText
}

func init() {
	// Builtin "telegram": kirim ke owner via Notify (= notifyOwnerTelegram). Format identik pra-R4.
	RegisterDeliverer("telegram", func(ctx context.Context, e *Engine, r floworkdb.Trigger, reply string) error {
		if e.Notify == nil {
			return nil
		}
		return e.Notify(ctx, "🔔 "+r.Name+"\n\n"+reply)
	})
	// Builtin "chat": append hasil ke chat_session (Config) → muncul di tab Chat. Format identik pra-R4.
	RegisterDeliverer("chat", func(ctx context.Context, e *Engine, r floworkdb.Trigger, reply string) error {
		var cfg struct {
			ChatSession string `json:"chat_session"`
		}
		_ = json.Unmarshal([]byte(r.Config), &cfg)
		if cfg.ChatSession == "" {
			return nil
		}
		_, cerr := e.Store.AddChatMessage(cfg.ChatSession, "assistant", "⏰ "+r.Name+"\n\n"+reply)
		return cerr
	})
}
