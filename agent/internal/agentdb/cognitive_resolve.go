// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval (autonomy grant 2026-06-19).
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/Flowork-OS
// Locked at: 2026-06-19
// Reason: CGM entity resolution (8-bit quantize + cosine) — built + unit-tested (build/vet/test green). Extend = new file, jangan modify ini.
//
// cognitive_resolve.go — Entity Resolution buat CGM (roadmap §4.4).
//
// Masalah: entitas sama disebut beda ("mobil"/"kendaraan"/"car") → kalau tiap kali
// bikin node baru, graph pecah → multi-hop sia-sia. Solusi: sebelum bikin node baru,
// bandingin embedding label kandidat ke node existing (type sama) by cosine. Mirip
// (≥ threshold) → MERGE (pakai id existing). Beda → node baru.
//
// Embedding 8-bit quantized (1 byte/dim, ~99% recall vs float — pola vecindex router):
// unit-normalize → int8 = round(x*127). Cosine antar dua vektor quantized = dot/127^2
// (karena dua-duanya unit-normalized). Hemat storage, ikut portable di state.db.
//
// Layering: file ini DB-side murni (gak import routerclient). Caller (extractor/dream)
// yang manggil router buat dapet float vektor → Quantize → ResolveByEmbedding.

package agentdb

import "math"

// DefaultResolveThreshold — cosine minimum buat dianggap entitas SAMA (§4.4, tunable).
const DefaultResolveThreshold = 0.86

// Quantize ubah float vektor → 8-bit (1 byte/dim). Unit-normalize dulu biar cosine
// jadi dot/127^2. Vektor nol → nil (gak bisa diresolusi by meaning).
func Quantize(vec []float32) []byte {
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	norm = math.Sqrt(norm)
	if norm == 0 {
		return nil
	}
	out := make([]byte, len(vec))
	for i, v := range vec {
		q := math.Round(float64(v) / norm * 127)
		if q > 127 {
			q = 127
		} else if q < -127 {
			q = -127
		}
		out[i] = byte(int8(q))
	}
	return out
}

// CosineQ — cosine approx antar dua vektor 8-bit quantized (dot / 127^2). Beda
// panjang / kosong → 0.
func CosineQ(a, b []byte) float64 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}
	var dot int64
	for i := range a {
		dot += int64(int8(a[i])) * int64(int8(b[i]))
	}
	return float64(dot) / (127.0 * 127.0)
}

// ResolveByEmbedding cari node existing `typ` yang paling mirip `queryEmb` (quantized).
// Return (id, score, true) kalau score >= threshold. threshold<=0 → DefaultResolveThreshold.
// queryEmb kosong → (,,false) (caller fallback ke resolusi label-exact).
func (s *Store) ResolveByEmbedding(typ string, queryEmb []byte, threshold float64) (id string, score float64, found bool) {
	if len(queryEmb) == 0 {
		return "", 0, false
	}
	if threshold <= 0 {
		threshold = DefaultResolveThreshold
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureCognitiveGraphSchema()

	rows, err := s.db.Query(
		`SELECT id, embedding FROM cognitive_nodes
		 WHERE type=? AND status='active' AND embedding IS NOT NULL`, typ)
	if err != nil {
		return "", 0, false
	}
	defer rows.Close()

	bestScore := -1.0
	bestID := ""
	for rows.Next() {
		var nid string
		var emb []byte
		if err := rows.Scan(&nid, &emb); err != nil {
			continue
		}
		sc := CosineQ(queryEmb, emb)
		if sc > bestScore {
			bestScore, bestID = sc, nid
		}
	}
	if bestID != "" && bestScore >= threshold {
		return bestID, bestScore, true
	}
	return "", bestScore, false
}
