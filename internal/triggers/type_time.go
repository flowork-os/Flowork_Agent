package triggers

import (
	"fmt"
	"time"

	"flowork-gui/internal/scheduler"
)

func init() { Register(&timeType{}) }

// timeType — tipe "time": cron → fire (INI Schedule / ROADMAP 2 yang diserap). Reuse parser
// cron yang sudah ada (internal/scheduler/cron.go, LOCKED — hanya dipanggil).
type timeType struct{}

func (t *timeType) ID() string            { return "time" }
func (t *timeType) Name() string          { return "Schedule (time)" }
func (t *timeType) Mode() string          { return "poll" }
func (t *timeType) PayloadKeys() []string { return []string{"time", "date"} }
func (t *timeType) ConfigSchema() []Field {
	return []Field{{
		Key: "cron", Label: "Cron (min hour dom mon dow)", Type: "text", Default: "0 9 * * *", Required: true,
		Help: "contoh: 0 9 * * 1-5 = hari kerja jam 09:00 · 0 * * * * = tiap jam",
	}}
}
func (t *timeType) OnWebhook(_ map[string]string, _ []byte) ([]Event, error) { return nil, nil }

func (t *timeType) Check(cfg map[string]string, state string) ([]Event, string, error) {
	spec, err := scheduler.Parse(cfg["cron"])
	if err != nil {
		return nil, state, fmt.Errorf("cron invalid: %w", err)
	}
	now := time.Now()
	nowMin := now.Format("2006-01-02T15:04")
	if state == nowMin {
		return nil, state, nil // sudah fire di menit ini (anti dobel)
	}
	if !spec.Matches(now) {
		return nil, state, nil
	}
	ev := Event{Key: nowMin, Payload: map[string]string{
		"time": now.Format("15:04"), "date": now.Format("2006-01-02"),
	}}
	return []Event{ev}, nowMin, nil
}
