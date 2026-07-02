// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// 📄 Dok: FLowork_os/lock/chat-vision.md
//
// vision_route_ext.go — SIBLING non-frozen (deletable): deteksi request VISION (content
// user berupa content-block string dgn gambar) biar jalur anthropic NO-TOOLS ikut di-route
// ke jalur with-tools (buildAnthropicToolBody) yg SATU-SATUNYA bisa bikin image block.
//
// AKAR (fix 2026-07-02): AnthropicMessage.Content itu `string` → forwardAnthropic/
// streamAnthropic (no-tools) ga bisa bawa gambar → vision cuma jalan kalau kebetulan ada
// tool (GUI architect). Jalur Telegram (visionDescribe, no-tools) → base64 nyasar jadi
// TEKS → model halu (token bengkak, jawaban ngarang). Fix: gate frozen nambah
// `|| hasVisionContent(req)` → request vision selalu lewat jalur yg pasang image block.
// Switch FLOWORK_VISION=0 → matiin (samain seam vision lain).
package router

import (
	"os"
	"strings"

	"github.com/flowork-os/flowork_Router/internal/visionblocks"
)

// hasVisionContent — ada pesan user yg content-nya content-block string bergambar?
func hasVisionContent(req OpenAIRequest) bool {
	if visionRouteOff() {
		return false
	}
	for _, m := range req.Messages {
		if m.Role != "user" {
			continue
		}
		raw := strings.TrimSpace(m.Content)
		if !strings.HasPrefix(raw, "[") {
			continue
		}
		if blocks, ok := visionblocks.Parse(m.Content); ok {
			for _, b := range blocks {
				if b.IsImage() {
					return true
				}
			}
		}
	}
	return false
}

func visionRouteOff() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("FLOWORK_VISION")))
	return v == "0" || v == "false" || v == "off"
}
