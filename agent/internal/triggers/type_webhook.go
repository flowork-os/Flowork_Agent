// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/trigger-schedule.md

package triggers

import (
	"encoding/json"
	"strconv"
	"time"
)

func init() { Register(&webhookType{}) }

type webhookType struct{}

func (t *webhookType) ID() string            { return "webhook" }
func (t *webhookType) Name() string          { return "Webhook (push)" }
func (t *webhookType) Mode() string          { return "webhook" }
func (t *webhookType) PayloadKeys() []string { return []string{"body", "key", "event"} }
func (t *webhookType) ConfigSchema() []Field { return []Field{} }
func (t *webhookType) Check(_ map[string]string, state string) ([]Event, string, error) {
	return nil, state, nil
}

func (t *webhookType) OnWebhook(_ map[string]string, body []byte) ([]Event, error) {
	payload := map[string]string{"body": string(body)}
	key := ""
	var raw map[string]any
	if json.Unmarshal(body, &raw) == nil {
		for k, v := range raw {
			switch s := v.(type) {
			case string:
				payload[k] = s
			case float64:
				payload[k] = strconv.FormatFloat(s, 'f', -1, 64)
			case bool:
				payload[k] = strconv.FormatBool(s)
			default:
				b, _ := json.Marshal(v)
				payload[k] = string(b)
			}
		}
		if k, ok := raw["key"].(string); ok && k != "" {
			key = k
		}
	}
	if key == "" {
		key = "wh-" + time.Now().UTC().Format("20060102T150405.000000000")
	}
	return []Event{{Key: key, Payload: payload}}, nil
}
