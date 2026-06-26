// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/trigger-schedule.md

package triggers

import (
	"context"
	"encoding/json"
	"strings"

	"flowork-gui/internal/floworkdb"
)

type Deliverer func(ctx context.Context, e *Engine, r floworkdb.Trigger, reply string) error

var deliverers = map[string]Deliverer{}

func RegisterDeliverer(kind string, fn Deliverer) {
	if kind = strings.TrimSpace(kind); kind != "" && fn != nil {
		deliverers[kind] = fn
	}
}

func dispatchDeliver(ctx context.Context, e *Engine, r floworkdb.Trigger, reply string) string {
	errText := ""
	for _, d := range strings.Split(r.Deliver, ",") {
		kind := strings.TrimSpace(d)
		if kind == "" {
			continue
		}
		fn := deliverers[kind]
		if fn == nil {
			continue
		}
		if derr := fn(ctx, e, r, reply); derr != nil {
			errText = "deliver " + kind + ": " + derr.Error()
		}
	}
	return errText
}

func init() {

	RegisterDeliverer("telegram", func(ctx context.Context, e *Engine, r floworkdb.Trigger, reply string) error {
		if e.Notify == nil {
			return nil
		}
		return e.Notify(ctx, "🔔 "+r.Name+"\n\n"+reply)
	})

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
