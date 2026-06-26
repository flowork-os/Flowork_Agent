// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package builtins

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"flowork-gui/internal/tools"
)

const (
	telegramAPIBase = "https://api.telegram.org"
	telegramMaxText = 4096
	telegramTimeout = 15 * time.Second
)

type telegramSendTool struct{}

func (telegramSendTool) Name() string       { return "telegram_send" }
func (telegramSendTool) Capability() string { return "net:fetch:telegram" }
func (telegramSendTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Send Telegram message to allowed chat. Bot token from agent secrets. chat_id MUST be in TELEGRAM_ALLOWED_CHATS.",
		Params: []tools.Param{
			{Name: "chat_id", Type: tools.ParamInt, Description: "Telegram chat ID (must be in allowed list)", Required: true},
			{Name: "text", Type: tools.ParamString, Description: "message text (max 4096 char)", Required: true},
		},
		Returns: "{chat_id, message_id, ok: bool}",
	}
}

func (telegramSendTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok {
		return tools.Result{}, fmt.Errorf("agent store not in context")
	}

	var chatID int64
	switch v := args["chat_id"].(type) {
	case float64:
		chatID = int64(v)
	case int:
		chatID = int64(v)
	case int64:
		chatID = v
	case string:

		n, perr := strconv.ParseInt(v, 10, 64)
		if perr != nil {
			return tools.Result{}, fmt.Errorf("chat_id must be int (got string '%s')", v)
		}
		chatID = n
	}
	if chatID == 0 {
		return tools.Result{}, fmt.Errorf("chat_id required (non-zero)")
	}

	text, _ := args["text"].(string)
	if strings.TrimSpace(text) == "" {
		return tools.Result{}, fmt.Errorf("text required (non-empty)")
	}
	if len(text) > telegramMaxText {
		text = text[:telegramMaxText-3] + "…"
	}

	secrets, serr := store.Secrets()
	if serr != nil {
		return tools.Result{}, fmt.Errorf("read secrets: %w", serr)
	}
	token := strings.TrimSpace(secrets["TELEGRAM_BOT_TOKEN"])
	if token == "" {
		return tools.Result{}, fmt.Errorf("TELEGRAM_BOT_TOKEN not set in agent secrets")
	}
	allowedRaw := strings.TrimSpace(secrets["TELEGRAM_ALLOWED_CHATS"])
	if allowedRaw == "" {
		return tools.Result{}, fmt.Errorf("TELEGRAM_ALLOWED_CHATS not set")
	}

	allowed := false
	for _, s := range strings.Split(allowedRaw, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if n, perr := strconv.ParseInt(s, 10, 64); perr == nil && n == chatID {
			allowed = true
			break
		}
	}
	if !allowed {
		return tools.Result{}, fmt.Errorf("chat_id %d not in TELEGRAM_ALLOWED_CHATS (anti-spam guard)", chatID)
	}

	url := fmt.Sprintf("%s/bot%s/sendMessage", telegramAPIBase, token)
	body := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}
	bodyJSON, _ := json.Marshal(body)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(bodyJSON)))
	if err != nil {
		return tools.Result{}, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: telegramTimeout}
	resp, derr := client.Do(httpReq)
	if derr != nil {
		return tools.Result{}, fmt.Errorf("telegram api: %w", derr)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	var tgResp struct {
		OK     bool   `json:"ok"`
		Desc   string `json:"description,omitempty"`
		Result struct {
			MessageID int64 `json:"message_id"`
		} `json:"result"`
	}
	if uerr := json.Unmarshal(respBytes, &tgResp); uerr != nil {
		return tools.Result{}, fmt.Errorf("decode response: %w", uerr)
	}
	if !tgResp.OK {
		return tools.Result{}, fmt.Errorf("telegram api fail: %s", tgResp.Desc)
	}
	return tools.Result{Output: map[string]any{
		"chat_id":    chatID,
		"message_id": tgResp.Result.MessageID,
		"ok":         true,
	}}, nil
}
