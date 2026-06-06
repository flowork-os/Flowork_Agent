package triggers

import (
	"encoding/json"
	"strconv"
	"time"
)

func init() { Register(&webhookType{}) }

// webhookType — tipe "webhook": sistem EKSTERNAL POST → fire. Mode push paling AGNOSTIC —
// sumber apa pun (CCTV/ML/IoT/script/web service) yang bisa HTTP POST bisa memicu aksi,
// tanpa kode khusus di inti. Payload = field JSON body (untuk {{...}}).
type webhookType struct{}

func (t *webhookType) ID() string            { return "webhook" }
func (t *webhookType) Name() string          { return "Webhook (push)" }
func (t *webhookType) Mode() string          { return "webhook" }
func (t *webhookType) PayloadKeys() []string { return []string{"body", "key", "event"} }
func (t *webhookType) ConfigSchema() []Field { return []Field{} } // secret auto-generate, tak ada field config
func (t *webhookType) Check(_ map[string]string, state string) ([]Event, string, error) {
	return nil, state, nil // bukan poll
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
			key = k // klien boleh kasih "key" untuk dedup; kalau tidak, tiap POST unik
		}
	}
	if key == "" {
		key = "wh-" + time.Now().UTC().Format("20060102T150405.000000000")
	}
	return []Event{{Key: key, Payload: payload}}, nil
}
