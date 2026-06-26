// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

func main() {
	var brainPath, outPath, ollamaURL, model string
	var batch, conc, limit int
	flag.StringVar(&brainPath, "brain", "", "source brain sqlite (tabel drawers: rowid,id,content)")
	flag.StringVar(&outPath, "out", "", "output v2 vectors sqlite (dibuat kalau belum ada)")
	flag.StringVar(&ollamaURL, "ollama", "http://127.0.0.1:11434", "Ollama base URL")
	flag.StringVar(&model, "model", "bge-m3", "embedding model")
	flag.IntVar(&batch, "batch", 64, "teks per request /api/embed (batch GPU)")
	flag.IntVar(&conc, "conc", 3, "request embed paralel")
	flag.IntVar(&limit, "limit", 0, "maks drawer diproses (0=semua; buat test)")
	flag.Parse()
	if brainPath == "" || outPath == "" {
		log.Fatal("wajib -brain dan -out")
	}

	src, err := sql.Open("sqlite", "file:"+brainPath+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		log.Fatalf("open brain: %v", err)
	}
	defer src.Close()
	out, err := sql.Open("sqlite", "file:"+outPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		log.Fatalf("open out: %v", err)
	}
	defer out.Close()
	if _, err := out.Exec(`
		CREATE TABLE IF NOT EXISTS drawer_vec_v2 (
			drawer_id TEXT PRIMARY KEY, vector BLOB NOT NULL, dim INTEGER NOT NULL, model TEXT NOT NULL);
		CREATE TABLE IF NOT EXISTS reembed_state (k TEXT PRIMARY KEY, v TEXT NOT NULL);`); err != nil {
		log.Fatalf("schema: %v", err)
	}

	lastRowid := int64(0)
	_ = out.QueryRow(`SELECT v FROM reembed_state WHERE k='last_rowid'`).Scan(&lastRowid)
	var already int
	_ = out.QueryRow(`SELECT COUNT(*) FROM drawer_vec_v2`).Scan(&already)
	var total int

	_ = src.QueryRow(`SELECT COUNT(*) FROM drawers WHERE length(content)>0 AND deleted_at IS NULL`).Scan(&total)
	log.Printf("brain=%s out=%s | total drawer(non-empty)=%d | udah=%d | resume dari rowid>%d",
		brainPath, outPath, total, already, lastRowid)

	client := &http.Client{Timeout: 5 * time.Minute}
	page := batch * conc
	done := already
	start := time.Now()

	for {
		if limit > 0 && (done-already) >= limit {
			break
		}
		rows, err := src.Query(
			`SELECT rowid, id, content FROM drawers WHERE rowid > ? AND length(content)>0 AND deleted_at IS NULL ORDER BY rowid LIMIT ?`,
			lastRowid, page)
		if err != nil {
			log.Fatalf("query drawers: %v", err)
		}
		type item struct {
			rowid   int64
			id, txt string
		}
		var items []item
		for rows.Next() {
			var it item
			if err := rows.Scan(&it.rowid, &it.id, &it.txt); err != nil {
				rows.Close()
				log.Fatalf("scan: %v", err)
			}

			if r := []rune(it.txt); len(r) > 1500 {
				it.txt = string(r[:1500])
			}
			items = append(items, it)
		}
		rows.Close()
		if len(items) == 0 {
			break
		}

		vecs := make([][]float32, len(items))
		errc := make(chan error, conc+1)
		sem := make(chan struct{}, conc)
		var wg sync.WaitGroup
		for s := 0; s < len(items); s += batch {
			e := s + batch
			if e > len(items) {
				e = len(items)
			}
			wg.Add(1)
			sem <- struct{}{}
			go func(s, e int) {
				defer wg.Done()
				defer func() { <-sem }()
				texts := make([]string, e-s)
				for i := s; i < e; i++ {
					texts[i-s] = clip(items[i].txt, 4000)
				}
				out, err := ollamaEmbed(client, ollamaURL, model, texts)
				if err != nil {
					errc <- err
					return
				}
				if len(out) != e-s {
					errc <- fmt.Errorf("embed count %d != %d", len(out), e-s)
					return
				}
				for i := s; i < e; i++ {
					vecs[i] = normalize(out[i-s])
				}
			}(s, e)
		}
		wg.Wait()
		select {
		case err := <-errc:
			log.Fatalf("embed gagal (resumable — jalanin lagi): %v", err)
		default:
		}

		tx, err := out.Begin()
		if err != nil {
			log.Fatalf("begin: %v", err)
		}
		stmt, _ := tx.Prepare(`INSERT OR REPLACE INTO drawer_vec_v2(drawer_id,vector,dim,model) VALUES(?,?,?,?)`)
		maxRowid := lastRowid
		for i, it := range items {
			if _, err := stmt.Exec(it.id, f32blob(vecs[i]), len(vecs[i]), model); err != nil {
				tx.Rollback()
				log.Fatalf("insert: %v", err)
			}
			if it.rowid > maxRowid {
				maxRowid = it.rowid
			}
		}
		stmt.Close()
		if _, err := tx.Exec(`INSERT OR REPLACE INTO reembed_state(k,v) VALUES('last_rowid',?)`, fmt.Sprint(maxRowid)); err != nil {
			tx.Rollback()
			log.Fatalf("state: %v", err)
		}
		if err := tx.Commit(); err != nil {
			log.Fatalf("commit: %v", err)
		}
		lastRowid = maxRowid
		done += len(items)

		el := time.Since(start).Seconds()
		rate := float64(done-already) / el
		eta := "-"
		if rate > 0 && total > done {
			eta = (time.Duration(float64(total-done)/rate) * time.Second).Truncate(time.Second).String()
		}
		log.Printf("  %d/%d (%.1f%%) | %.0f/s | ETA %s | rowid=%d", done, total, 100*float64(done)/float64(max(total, 1)), rate, eta, lastRowid)
	}
	log.Printf("SELESAI: %d drawer ter-embed di %s (%.0fs)", done, outPath, time.Since(start).Seconds())
}

func ollamaEmbed(c *http.Client, base, model string, texts []string) ([][]float32, error) {
	body, _ := json.Marshal(map[string]any{"model": model, "input": texts})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, base+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024*1024))
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("ollama %d: %.200s", resp.StatusCode, raw)
	}
	var out struct {
		Embeddings [][]float32 `json:"embeddings"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out.Embeddings, nil
}

func normalize(v []float32) []float32 {
	var ss float64
	for _, x := range v {
		ss += float64(x) * float64(x)
	}
	n := float32(math.Sqrt(ss))
	if n == 0 {
		return v
	}
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = x / n
	}
	return out
}

func f32blob(v []float32) []byte {
	b := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
