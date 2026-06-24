package router

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/flowork-os/flowork_Router/internal/providers/embedding"
)

type mockEmbedder struct {
	embedFunc func(ctx context.Context, req embedding.Request) (*embedding.Result, error)
}

func (m *mockEmbedder) Name() string { return "local" }
func (m *mockEmbedder) Embed(ctx context.Context, req embedding.Request) (*embedding.Result, error) {
	if m.embedFunc != nil {
		return m.embedFunc(ctx, req)
	}
	data := make([]embedding.Embed, len(req.Input))
	for i := range req.Input {
		data[i] = embedding.Embed{
			Object:    "embedding",
			Embedding: []float64{0.577, 0.577, 0.577},
			Index:     i,
		}
	}
	return &embedding.Result{
		Object: "list",
		Data:   data,
	}, nil
}

func TestCosineSimilarity(t *testing.T) {
	cases := []struct {
		a    []float64
		b    []float64
		want float64
	}{
		{a: []float64{1, 0, 0}, b: []float64{1, 0, 0}, want: 1.0},
		{a: []float64{1, 0, 0}, b: []float64{0, 1, 0}, want: 0.0},
		{a: []float64{1, 1, 0}, b: []float64{1, 1, 0}, want: 1.0},
		{a: []float64{1, 0, 0}, b: []float64{-1, 0, 0}, want: -1.0},
		{a: []float64{}, b: []float64{1}, want: 0.0},
	}

	for _, c := range cases {
		got := cosineSimilarity(c.a, c.b)
		if mathAbs(got-c.want) > 1e-5 {
			t.Errorf("cosineSimilarity(%v, %v) = %f; want %f", c.a, c.b, got, c.want)
		}
	}
}

func mathAbs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func TestMaybeFilterTools_DisabledByDefault(t *testing.T) {
	os.Setenv("FLOW_ROUTER_DYNAMIC_TOOLS", "0")
	defer os.Unsetenv("FLOW_ROUTER_DYNAMIC_TOOLS")

	toolsJSON := `[
		{"type":"function","function":{"name":"grep_search","description":"search files"}},
		{"type":"function","function":{"name":"view_file","description":"read file"}}
	]`
	req := OpenAIRequest{
		Model: "some-model",
		Tools: json.RawMessage(toolsJSON),
		Messages: []OpenAIMessage{
			{Role: "user", Content: "hello"},
		},
	}

	filtered := maybeFilterTools(context.Background(), req, nil)
	if string(filtered.Tools) != toolsJSON {
		t.Errorf("expected no filtering when disabled, got %s", string(filtered.Tools))
	}
}

func TestMaybeFilterTools_Filtering(t *testing.T) {
	os.Setenv("FLOW_ROUTER_DYNAMIC_TOOLS", "1")
	defer os.Unsetenv("FLOW_ROUTER_DYNAMIC_TOOLS")

	mock := &mockEmbedder{
		embedFunc: func(ctx context.Context, req embedding.Request) (*embedding.Result, error) {
			data := make([]embedding.Embed, len(req.Input))
			for i, text := range req.Input {
				var vec []float64
				if text == "read file" || text == "cari kata di file" {
					vec = []float64{1.0, 0.0, 0.0}
				} else if text == "send telegram" {
					vec = []float64{0.0, 1.0, 0.0}
				} else {
					vec = []float64{0.0, 0.0, 1.0}
				}
				data[i] = embedding.Embed{
					Object:    "embedding",
					Embedding: vec,
					Index:     i,
				}
			}
			return &embedding.Result{
				Object: "list",
				Data:   data,
			}, nil
		},
	}
	embedding.Register(mock)

	toolsJSON := `[
		{"type":"function","function":{"name":"file_read","description":"read file"}},
		{"type":"function","function":{"name":"telegram_send","description":"send telegram"}},
		{"type":"function","function":{"name":"structured_output","description":"structured output"}}
	]`

	req := OpenAIRequest{
		Model: "some-model",
		Tools: json.RawMessage(toolsJSON),
		Messages: []OpenAIMessage{
			{Role: "user", Content: "cari kata di file"},
		},
	}

	filtered := maybeFilterTools(context.Background(), req, nil)

	var list []requestTool
	if err := json.Unmarshal(filtered.Tools, &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	hasRead := false
	hasTelegram := false
	hasStructured := false

	for _, t := range list {
		if t.Function.Name == "file_read" {
			hasRead = true
		}
		if t.Function.Name == "telegram_send" {
			hasTelegram = true
		}
		if t.Function.Name == "structured_output" {
			hasStructured = true
		}
	}

	if !hasRead {
		t.Error("expected file_read to be kept")
	}
	if hasTelegram {
		t.Error("expected telegram_send to be filtered out")
	}
	if !hasStructured {
		t.Error("expected structured_output to be kept")
	}
}
