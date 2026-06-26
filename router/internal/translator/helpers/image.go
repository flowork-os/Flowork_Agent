// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package helpers

import "strings"

func OpenAIImageToAnthropic(part map[string]any) map[string]any {
	urlObj, _ := part["image_url"].(map[string]any)
	url, _ := urlObj["url"].(string)
	if strings.HasPrefix(url, "data:") {

		mime, payload := splitDataURL(url)
		return map[string]any{
			"type": "image",
			"source": map[string]any{
				"type":       "base64",
				"media_type": mime,
				"data":       payload,
			},
		}
	}
	return map[string]any{
		"type": "image",
		"source": map[string]any{
			"type": "url",
			"url":  url,
		},
	}
}

func AnthropicImageToOpenAI(block map[string]any) map[string]any {
	src, _ := block["source"].(map[string]any)
	srcType, _ := src["type"].(string)
	switch srcType {
	case "base64":
		mt, _ := src["media_type"].(string)
		data, _ := src["data"].(string)
		return map[string]any{
			"type": "image_url",
			"image_url": map[string]any{
				"url": "data:" + mt + ";base64," + data,
			},
		}
	case "url":
		url, _ := src["url"].(string)
		return map[string]any{
			"type": "image_url",
			"image_url": map[string]any{
				"url": url,
			},
		}
	}
	return nil
}

func OpenAIImageToGemini(part map[string]any) map[string]any {
	urlObj, _ := part["image_url"].(map[string]any)
	url, _ := urlObj["url"].(string)
	if strings.HasPrefix(url, "data:") {
		mime, payload := splitDataURL(url)
		return map[string]any{
			"inline_data": map[string]any{
				"mime_type": mime,
				"data":      payload,
			},
		}
	}
	return map[string]any{
		"file_data": map[string]any{
			"file_uri": url,
		},
	}
}

func splitDataURL(s string) (string, string) {
	if !strings.HasPrefix(s, "data:") {
		return "application/octet-stream", ""
	}
	body := strings.TrimPrefix(s, "data:")
	sep := strings.Index(body, ",")
	if sep < 0 {
		return "application/octet-stream", ""
	}
	head, payload := body[:sep], body[sep+1:]
	mime := head
	if i := strings.Index(head, ";"); i >= 0 {
		mime = head[:i]
	}
	if mime == "" {
		mime = "application/octet-stream"
	}
	return mime, payload
}
