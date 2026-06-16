// Command brain-search — CLI semantic search atas index vecindex (debug/ops + bukti e2e).
//
// TUJUAN (buat AI lain): query teks → embed (Ollama bge-m3, ENGINE SAMA dgn index) → vecindex
// top-k → tampilin drawer (id+content dari brain). Buktiin pipeline RAG sovereign end-to-end:
// pertanyaan manusia → memori relevan, lokal, anti-halu. Pure-Go.
//
// Pakai: go run ./cmd/brain-search -index <brain.vindex> -brain <src.sqlite> -query "..." [-k 5]
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/flowork-os/flowork_Router/internal/brain/vecindex"
	_ "modernc.org/sqlite"
)

func main() {
	var indexPath, brainPath, query, ollamaURL, model string
	var k int
	flag.StringVar(&indexPath, "index", "", "file index vecindex (.vindex)")
	flag.StringVar(&brainPath, "brain", "", "brain sqlite (buat ambil content drawer)")
	flag.StringVar(&query, "query", "", "teks query")
	flag.StringVar(&ollamaURL, "ollama", "http://127.0.0.1:11434", "Ollama base URL")
	flag.StringVar(&model, "model", "bge-m3", "embedding model")
	flag.IntVar(&k, "k", 5, "top-k")
	flag.Parse()
	if indexPath == "" || query == "" {
		log.Fatal("wajib -index dan -query")
	}
	t := time.Now()
	idx, err := vecindex.Load(indexPath)
	if err != nil {
		log.Fatalf("load index: %v", err)
	}
	qv, err := embed(ollamaURL, model, query)
	if err != nil {
		log.Fatalf("embed query: %v", err)
	}
	hits := idx.Search(qv, k)
	fmt.Printf("query=%q | index=%d vektor | %d hit (%.0fms)\n", query, idx.Len(), len(hits), float64(time.Since(t).Microseconds())/1000)

	var brain *sql.DB
	if brainPath != "" {
		brain, _ = sql.Open("sqlite", "file:"+brainPath+"?mode=ro")
		defer brain.Close()
	}
	for i, h := range hits {
		content := ""
		if brain != nil {
			_ = brain.QueryRow(`SELECT content FROM drawers WHERE id=?`, h.ID).Scan(&content)
		}
		fmt.Printf("  %d. [%.0f] %s\n", i+1, h.Score, clip(oneLine(content), 120))
	}
}

func embed(base, model, text string) ([]float32, error) {
	body, _ := json.Marshal(map[string]any{"model": model, "input": text})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, base+"/api/embed", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: time.Minute}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("ollama %d: %.200s", resp.StatusCode, raw)
	}
	var out struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if len(out.Embeddings) == 0 {
		return nil, fmt.Errorf("no embedding")
	}
	v := out.Embeddings[0]
	var ss float64
	for _, x := range v {
		ss += float64(x) * float64(x)
	}
	n := float32(math.Sqrt(ss))
	if n > 0 {
		for i := range v {
			v[i] /= n
		}
	}
	return v, nil
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
func clip(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
