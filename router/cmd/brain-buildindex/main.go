// === LOCKED FILE (soft) === Status: STABLE — owner-approved 2026-06-16 (LOCKED ≠ FREEZE).
// AI lain: JANGAN otak-atik tanpa izin owner.
//
// Command brain-buildindex — bangun INDEX VEKTOR 8-bit (vecindex) dari db v2 hasil brain-reembed.
//
// TUJUAN (buat AI lain): ubah vektor fp32 (db v2) jadi index ter-kuantisasi yang dipakai router
// buat semantic search (RAG anti-halu). 2-PASS biar muat RAM di 5jt: pass-1 scan max|komponen|
// (skala global, recall optimal) → pass-2 STREAM Build (vecindex.Builder, kode int8 numbuh) → Save.
// Output ~1 byte/dim (5jt ≈ 5GB). Pure-Go, no-cgo. Owner-run ops; router load file ini on boot (#6).
//
// Pakai: go run ./cmd/brain-buildindex -vec <vec_v2.sqlite> -out <brain.vindex> [-scale 0]
package main

import (
	"database/sql"
	"encoding/binary"
	"flag"
	"log"
	"math"
	"time"

	"github.com/flowork-os/flowork_Router/internal/brain/vecindex"
	_ "modernc.org/sqlite"
)

func blobToF32(b []byte) []float32 {
	n := len(b) / 4
	v := make([]float32, n)
	for i := 0; i < n; i++ {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

func main() {
	var vecPath, outPath string
	var fixedScale float64
	flag.StringVar(&vecPath, "vec", "", "db v2 hasil brain-reembed (drawer_vec_v2)")
	flag.StringVar(&outPath, "out", "", "file index output (.vindex)")
	flag.Float64Var(&fixedScale, "scale", 0, "skala kuantisasi (0 = auto-scan max|komponen|)")
	flag.Parse()
	if vecPath == "" || outPath == "" {
		log.Fatal("wajib -vec dan -out")
	}
	db, err := sql.Open("sqlite", "file:"+vecPath+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		log.Fatalf("open vec: %v", err)
	}
	defer db.Close()

	var total, dim int
	_ = db.QueryRow(`SELECT COUNT(*), COALESCE(MAX(dim),0) FROM drawer_vec_v2`).Scan(&total, &dim)
	if total == 0 || dim == 0 {
		log.Fatalf("v2 kosong / dim 0 (total=%d dim=%d)", total, dim)
	}
	log.Printf("vec=%s | total=%d dim=%d", vecPath, total, dim)
	start := time.Now()

	scale := float32(fixedScale)
	if scale <= 0 {
		// PASS-1: scan max|komponen| (constant memory — gak nyimpen vektor).
		rows, err := db.Query(`SELECT vector FROM drawer_vec_v2`)
		if err != nil {
			log.Fatalf("pass1 query: %v", err)
		}
		var maxAbs float32
		var seen int
		for rows.Next() {
			var blob []byte
			if err := rows.Scan(&blob); err != nil {
				rows.Close()
				log.Fatalf("pass1 scan: %v", err)
			}
			for _, x := range blobToF32(blob) {
				if a := float32(math.Abs(float64(x))); a > maxAbs {
					maxAbs = a
				}
			}
			seen++
		}
		rows.Close()
		scale = maxAbs
		log.Printf("pass-1: max|komponen|=%.5f (scale) atas %d vektor (%.0fs)", scale, seen, time.Since(start).Seconds())
	} else {
		log.Printf("scale tetap = %.5f (skip pass-1)", scale)
	}

	// PASS-2: STREAM build (kuantisasi tiap vektor → kode int8 numbuh).
	b := vecindex.NewBuilder(dim, scale)
	rows, err := db.Query(`SELECT drawer_id, vector FROM drawer_vec_v2`)
	if err != nil {
		log.Fatalf("pass2 query: %v", err)
	}
	for rows.Next() {
		var id string
		var blob []byte
		if err := rows.Scan(&id, &blob); err != nil {
			rows.Close()
			log.Fatalf("pass2 scan: %v", err)
		}
		if err := b.Add(id, blobToF32(blob)); err != nil {
			rows.Close()
			log.Fatalf("add %s: %v", id, err)
		}
		if b.Len()%200000 == 0 {
			log.Printf("  build %d/%d (%.1f%%)", b.Len(), total, 100*float64(b.Len())/float64(total))
		}
	}
	rows.Close()

	idx := b.Finish()
	if err := idx.Save(outPath); err != nil {
		log.Fatalf("save: %v", err)
	}
	log.Printf("SELESAI: index %d vektor dim=%d → %s (%.0fs)", idx.Len(), dim, outPath, time.Since(start).Seconds())
}
