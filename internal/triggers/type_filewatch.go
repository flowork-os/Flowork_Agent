package triggers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func init() { Register(&fileWatchType{}) }

// fileWatchType — tipe "file-watch": file baru di folder → fire. Poll (list+diff), portable
// (bukan inotify OS-spesifik). Built-in (Go host) → akses fs langsung; owner menetapkan folder.
// (Plugin wasm pihak-ketiga kelak pakai cap fs.list scoped — increment berikut, interface tetap.)
type fileWatchType struct{}

func (t *fileWatchType) ID() string            { return "file-watch" }
func (t *fileWatchType) Name() string          { return "File Watch" }
func (t *fileWatchType) Mode() string          { return "poll" }
func (t *fileWatchType) PayloadKeys() []string { return []string{"path", "name", "ext", "size"} }
func (t *fileWatchType) ConfigSchema() []Field {
	return []Field{
		{Key: "folder", Label: "Folder", Type: "path", Required: true, Help: "folder yang dipantau (mis. /home/you/inbox)"},
		{Key: "pattern", Label: "Pattern (glob)", Type: "text", Default: "*", Help: "contoh: *.pdf"},
	}
}
func (t *fileWatchType) OnWebhook(_ map[string]string, _ []byte) ([]Event, error) { return nil, nil }

// Check — state = JSON array "name|mtime" yang sudah terlihat. Run pertama (state=="") = SEED
// (tandai file lama sebagai terlihat, JANGAN fire) → hanya file BARU setelah rule dibuat yang fire.
func (t *fileWatchType) Check(cfg map[string]string, state string) ([]Event, string, error) {
	folder := strings.TrimSpace(cfg["folder"])
	if folder == "" {
		return nil, state, nil
	}
	pattern := cfg["pattern"]
	if pattern == "" {
		pattern = "*"
	}
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, state, nil // folder belum ada / tak terbaca → diam (jangan crash)
	}
	seen := map[string]bool{}
	if state != "" {
		var arr []string
		if json.Unmarshal([]byte(state), &arr) == nil {
			for _, k := range arr {
				seen[k] = true
			}
		}
	}
	firstRun := state == ""
	var events []Event
	cur := []string{}
	for _, en := range entries {
		if en.IsDir() {
			continue
		}
		name := en.Name()
		if ok, _ := filepath.Match(pattern, name); !ok {
			continue
		}
		mt, sz := "0", int64(0)
		if info, ierr := en.Info(); ierr == nil {
			mt = strconv.FormatInt(info.ModTime().Unix(), 10)
			sz = info.Size()
		}
		key := name + "|" + mt
		cur = append(cur, key)
		if seen[key] || firstRun {
			continue // sudah pernah, atau seed pertama
		}
		events = append(events, Event{Key: key, Payload: map[string]string{
			"path": filepath.Join(folder, name), "name": name,
			"ext":  strings.TrimPrefix(filepath.Ext(name), "."),
			"size": strconv.FormatInt(sz, 10),
		}})
	}
	newState, _ := json.Marshal(cur)
	return events, string(newState), nil
}
