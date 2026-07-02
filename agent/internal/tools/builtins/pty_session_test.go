//go:build linux

package builtins

import (
	"context"
	"strings"
	"testing"

	"flowork-gui/internal/tools"
)

func TestPTYStartSendReadClose(t *testing.T) {
	ctx := tools.WithSharedDir(context.Background(), t.TempDir())

	// `cat` = echo stdin ke stdout → gampang diverifikasi.
	res, err := (ptyStartTool{}).Run(ctx, map[string]any{"command": "cat", "wait_ms": float64(300)})
	if err != nil {
		t.Fatalf("pty_start: %v", err)
	}
	sid, _ := res.Output.(map[string]any)["session_id"].(string)
	if sid == "" {
		t.Fatal("session_id kosong")
	}
	defer (ptyCloseTool{}).Run(ctx, map[string]any{"session_id": sid})

	sres, err := (ptySendTool{}).Run(ctx, map[string]any{
		"session_id": sid, "input": "halo-pty-123", "wait_ms": float64(500),
	})
	if err != nil {
		t.Fatalf("pty_send: %v", err)
	}
	out, _ := sres.Output.(map[string]any)["output"].(string)
	if !strings.Contains(out, "halo-pty-123") {
		t.Errorf("output ga memuat input yang dikirim, dapet: %q", out)
	}

	cres, err := (ptyCloseTool{}).Run(ctx, map[string]any{"session_id": sid})
	if err != nil {
		t.Fatalf("pty_close: %v", err)
	}
	if closed, _ := cres.Output.(map[string]any)["closed"].(bool); !closed {
		t.Error("closed harus true")
	}
	// Sesi udah ilang.
	if _, ok := ptyGet(sid); ok {
		t.Error("sesi harusnya udah dihapus dari store")
	}
}

func TestPTYInteractiveShellEcho(t *testing.T) {
	ctx := tools.WithSharedDir(context.Background(), t.TempDir())
	res, err := (ptyStartTool{}).Run(ctx, map[string]any{"wait_ms": float64(300)}) // shell interaktif
	if err != nil {
		t.Fatalf("pty_start shell: %v", err)
	}
	sid := res.Output.(map[string]any)["session_id"].(string)
	defer (ptyCloseTool{}).Run(ctx, map[string]any{"session_id": sid})

	sres, err := (ptySendTool{}).Run(ctx, map[string]any{
		"session_id": sid, "input": "echo NBTEST_MARKER", "wait_ms": float64(600),
	})
	if err != nil {
		t.Fatalf("pty_send: %v", err)
	}
	out := sres.Output.(map[string]any)["output"].(string)
	if !strings.Contains(out, "NBTEST_MARKER") {
		t.Errorf("shell ga ngeksekusi echo, output: %q", out)
	}
}

func TestPTYGuardsDangerousStartAndSend(t *testing.T) {
	ctx := tools.WithSharedDir(context.Background(), t.TempDir())

	// Start command bahaya → diblok shell_guard.
	if _, err := (ptyStartTool{}).Run(ctx, map[string]any{"command": "rm -rf /"}); err == nil {
		t.Error("pty_start harus blok 'rm -rf /'")
	}

	// Sesi aman, tapi input bahaya diblok.
	res, err := (ptyStartTool{}).Run(ctx, map[string]any{"command": "cat", "wait_ms": float64(200)})
	if err != nil {
		t.Fatalf("pty_start cat: %v", err)
	}
	sid := res.Output.(map[string]any)["session_id"].(string)
	defer (ptyCloseTool{}).Run(ctx, map[string]any{"session_id": sid})
	if _, err := (ptySendTool{}).Run(ctx, map[string]any{"session_id": sid, "input": "rm -rf /"}); err == nil {
		t.Error("pty_send harus blok input 'rm -rf /'")
	}
}

func TestPTYSendUnknownSession(t *testing.T) {
	ctx := tools.WithSharedDir(context.Background(), t.TempDir())
	_ = ctx
	if _, err := (ptySendTool{}).Run(context.Background(), map[string]any{"session_id": "pty-nonexist", "input": "x"}); err == nil {
		t.Error("pty_send ke sesi ga ada harus error")
	}
}
