package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/teetah2402/flowork/internal/provider"
)

// PeerReviewTool — meminta review dari agent lain lewat shared_chat.jsonl.
// Flow:
//  1. Agent ini memanggil peer_review dengan (target, artifact, criteria).
//  2. Tool menulis entry ke shared_chat dengan format:
//     [PEER-REVIEW REQUEST] from=<self> to=@<target>
//     artifact: <path|snippet|PR#>
//     criteria: <one-line|bullets>
//     verdict_format: APPROVE | REQUEST_CHANGES | NEEDS_DISCUSSION
//  3. Watcher target (flowork-watcher --as <target>) lihat mention di inbox,
//     feed ke agent target, lalu post balik dengan verdict di baris pertama.
//  4. Agent ini boleh polling inbox lewat shared_inbox tool untuk ambil verdict.
//
// Tool ini TIDAK block — asynchronous by design. Untuk tunggu verdict, agent
// panggil tool lagi atau pakai sleep + shared_inbox read.
type PeerReviewTool struct {
	selfIdentity string
}

func NewPeerReviewTool(selfIdentity string) *PeerReviewTool {
	if selfIdentity == "" {
		selfIdentity = "flowork"
	}
	return &PeerReviewTool{selfIdentity: selfIdentity}
}

type peerReviewArgs struct {
	Target   string `json:"target"`
	Artifact string `json:"artifact" validate:"required"`
	Criteria string `json:"criteria,omitempty"`
	Channel  string `json:"channel,omitempty"`
}

func (t *PeerReviewTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "peer_review",
		Description: `Minta review dari agent lain (identitas yang punya watcher: claude, gemini, dll) via shared_chat.

Post request berformat khusus; watcher target auto-feed ke agent-nya dan post balik dengan verdict APPROVE | REQUEST_CHANGES | NEEDS_DISCUSSION di baris pertama.

Tool ini return segera setelah request dipost — untuk ambil verdict, tunggu beberapa detik lalu panggil shared_inbox tool dengan channel yang sama.

Gunakan untuk: dua-opini pada PR/komponen kritis, cross-check keputusan arsitektur, audit keamanan sebelum merge.`,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target":   map[string]any{"type": "string", "description": "Identitas agent target tanpa '@' (mis. claude, gemini, flowork)."},
				"artifact": map[string]any{"type": "string", "description": "Path file, snippet, atau PR# yang di-review. Keep scannable."},
				"criteria": map[string]any{"type": "string", "description": "Apa yang harus dicek (security, correctness, naming, test coverage, dll)."},
				"channel":  map[string]any{"type": "string", "description": "Shared chat channel, default 'main'."},
			},
			"required": []string{"target", "artifact"},
		},
	}
}

func (t *PeerReviewTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args peerReviewArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("decode: %w", err)
	}
	if err := ValidateRequired(&args); err != nil { return Result{}, fmt.Errorf("validation failed: %w", err) }

	target := strings.TrimPrefix(strings.TrimSpace(args.Target), "@")
	if target == "" {
		return Result{ToolName: "peer_review", OK: false, Output: "target required"}, nil
	}
	if strings.TrimSpace(args.Artifact) == "" {
		return Result{ToolName: "peer_review", OK: false, Output: "artifact required"}, nil
	}
	channel := strings.TrimSpace(args.Channel)
	if channel == "" {
		channel = "main"
	}

	criteria := strings.TrimSpace(args.Criteria)
	if criteria == "" {
		criteria = "bug potensial, code smell, missing test"
	}

	message := fmt.Sprintf(`[PEER-REVIEW REQUEST]
from: %s
to: @%s

artifact:
%s

criteria:
%s

verdict_format: Di BARIS PERTAMA reply-mu tuliskan salah satu — APPROVE | REQUEST_CHANGES | NEEDS_DISCUSSION — diikuti alasan ringkas di baris berikutnya. Tanpa prefix itu di baris pertama, request dianggap tidak dijawab.`,
		t.selfIdentity, target, args.Artifact, criteria)

	if err := writeSharedChat(t.selfIdentity, channel, message); err != nil {
		return Result{ToolName: "peer_review", OK: false, Output: "write shared_chat: " + err.Error()}, nil
	}

	return Result{
		ToolName: "peer_review",
		OK:       true,
		Output: fmt.Sprintf("Peer review request dikirim ke @%s di channel %s.\nTunggu ~10-30 detik lalu pakai shared_inbox tool untuk ambil verdict.",
			target, channel),
		Metadata: map[string]any{
			"target":  target,
			"channel": channel,
		},
	}, nil
}

// writeSharedChat — wrapper serupa appendMsg di flowork-chat, tapi di package
// tools agar tool ini self-contained.
func writeSharedChat(from, channel, message string) error {
	home, _ := os.UserHomeDir()
	p := filepath.Join(home, ".flowork", "shared_chat.jsonl")
	_ = os.MkdirAll(filepath.Dir(p), 0755)
	entry := map[string]string{
		"ts":      time.Now().UTC().Format(time.RFC3339),
		"from":    from,
		"channel": channel,
		"message": message,
	}
	b, _ := json.Marshal(entry)
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("tools: writeSharedChat: open: %w", err)
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s\n", b)
	return fmt.Errorf("tools: writeSharedChat: close: %w", err)
}
