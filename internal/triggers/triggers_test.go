package triggers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRenderTemplate(t *testing.T) {
	got := renderTemplate("ada {{name}} di {{ path }} ({{missing}})", map[string]string{"name": "a.pdf", "path": "/inbox"})
	if got != "ada a.pdf di /inbox ()" {
		t.Fatalf("render salah: %q", got)
	}
}

func TestTimeTypeFiresOncePerMinute(t *testing.T) {
	tt := &timeType{}
	cfg := map[string]string{"cron": "* * * * *"} // tiap menit
	ev, st, err := tt.Check(cfg, "")
	if err != nil || len(ev) != 1 {
		t.Fatalf("harus fire 1x: ev=%d err=%v", len(ev), err)
	}
	// state sekarang = menit ini → Check lagi TIDAK fire (anti dobel)
	ev2, _, _ := tt.Check(cfg, st)
	if len(ev2) != 0 {
		t.Fatalf("tidak boleh fire dua kali di menit yang sama: %d", len(ev2))
	}
	// cron invalid → error, bukan panic
	if _, _, e := tt.Check(map[string]string{"cron": "bukan cron"}, ""); e == nil {
		t.Fatal("cron invalid harus error")
	}
}

func TestFileWatchSeedsThenFiresNew(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "lama.pdf"), []byte("x"), 0o644)
	fw := &fileWatchType{}
	cfg := map[string]string{"folder": dir, "pattern": "*.pdf"}
	// run pertama = SEED: file lama TIDAK fire
	ev, st, err := fw.Check(cfg, "")
	if err != nil || len(ev) != 0 {
		t.Fatalf("seed run tak boleh fire: ev=%d err=%v", len(ev), err)
	}
	// tambah file BARU → fire 1x dgn payload
	os.WriteFile(filepath.Join(dir, "baru.pdf"), []byte("y"), 0o644)
	ev2, _, _ := fw.Check(cfg, st)
	if len(ev2) != 1 || ev2[0].Payload["name"] != "baru.pdf" || ev2[0].Payload["ext"] != "pdf" {
		t.Fatalf("file baru harus fire dgn payload: %+v", ev2)
	}
	// non-match (txt) diabaikan
	os.WriteFile(filepath.Join(dir, "skip.txt"), []byte("z"), 0o644)
	ev3, _, _ := fw.Check(cfg, st)
	for _, e := range ev3 {
		if e.Payload["name"] == "skip.txt" {
			t.Fatal("pattern *.pdf harus skip .txt")
		}
	}
}

func TestWebhookParsesPayloadAndKey(t *testing.T) {
	wh := &webhookType{}
	ev, err := wh.OnWebhook(nil, []byte(`{"key":"abc","title":"hi","n":3}`))
	if err != nil || len(ev) != 1 {
		t.Fatalf("webhook gagal: %v", err)
	}
	if ev[0].Key != "abc" || ev[0].Payload["title"] != "hi" || ev[0].Payload["n"] != "3" {
		t.Fatalf("payload/key salah: %+v", ev[0])
	}
	// tanpa key → tetap fire (key unik)
	ev2, _ := wh.OnWebhook(nil, []byte(`{"x":1}`))
	if len(ev2) != 1 || ev2[0].Key == "" {
		t.Fatal("tanpa key harus tetap fire dgn key unik")
	}
}
