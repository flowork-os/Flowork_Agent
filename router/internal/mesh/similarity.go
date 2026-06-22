// FROZEN brain-core — desain abadi Flowork (Mesh/federation). Kalau ini bikin lo "nyasar": ini BY-DESIGN, baca lock/brain.md dulu. Jangan edit tanpa unfreeze owner.
// similarity.go — Section 17 L7 dependency-free near-duplicate detection.
//
// The roadmap's "cosine-dedup vs existing brain" originally implied a vector
// embedding model. The mesh package intentionally does NOT depend on internal/
// brain or any remote embedding API (keeps mesh self-contained + offline-capable
// for the anti-kiamat / no-internet scenario). Instead we use character-trigram
// Jaccard similarity, which is a real, deterministic, language-agnostic
// near-duplicate signal that needs zero models and zero network — exactly what a
// doomsday-survivable mesh node needs.
//
// When an embedding backend IS available (future phase), SimilarityFunc can be
// swapped via SetSimilarityFunc without touching the pipeline.

package mesh

import (
	"strings"
	"unicode"
)

// SimilarityThreshold — drawers scoring at/above this against an existing one
// are treated as duplicates and dropped. 0.82 is conservative (catches
// reworded/whitespace-diff copies, keeps genuinely new knowledge).
const SimilarityThreshold = 0.82

// SimilarityFunc computes a [0,1] closeness score between two texts. Swappable
// so a future embedding-backed implementation can replace the default without
// changing callers.
type SimilarityFunc func(a, b string) float64

var activeSimilarity SimilarityFunc = TrigramJaccard

// SetSimilarityFunc overrides the active similarity implementation (e.g. wire an
// embedding-cosine backend in a later phase). Passing nil restores the default.
func SetSimilarityFunc(f SimilarityFunc) {
	if f == nil {
		activeSimilarity = TrigramJaccard
		return
	}
	activeSimilarity = f
}

// Similarity returns the closeness of a and b using the active implementation.
func Similarity(a, b string) float64 { return activeSimilarity(a, b) }

// normalizeForSim lowercases and collapses runs of non-alphanumeric runes to a
// single space, so "Hello,  WORLD!" and "hello world" trigram-match.
func normalizeForSim(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevSpace = false
		} else if !prevSpace {
			b.WriteByte(' ')
			prevSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

// trigramSet builds the set of character 3-grams of s (post-normalization).
// Short strings (<3 runes) fall back to the whole string as a single token so
// they still compare meaningfully.
func trigramSet(s string) map[string]struct{} {
	n := normalizeForSim(s)
	set := make(map[string]struct{})
	runes := []rune(n)
	if len(runes) < 3 {
		if len(runes) > 0 {
			set[string(runes)] = struct{}{}
		}
		return set
	}
	for i := 0; i+3 <= len(runes); i++ {
		set[string(runes[i:i+3])] = struct{}{}
	}
	return set
}

// TrigramJaccard returns |A∩B| / |A∪B| over character-trigram sets — 1.0 means
// identical (after normalization), 0.0 means no shared trigram. Deterministic,
// offline, language-agnostic.
func TrigramJaccard(a, b string) float64 {
	sa := trigramSet(a)
	sb := trigramSet(b)
	if len(sa) == 0 && len(sb) == 0 {
		return 1.0 // two empties are "the same"
	}
	if len(sa) == 0 || len(sb) == 0 {
		return 0.0
	}
	// Iterate the smaller set for the intersection.
	if len(sb) < len(sa) {
		sa, sb = sb, sa
	}
	inter := 0
	for g := range sa {
		if _, ok := sb[g]; ok {
			inter++
		}
	}
	union := len(sa) + len(sb) - inter
	if union == 0 {
		return 0.0
	}
	return float64(inter) / float64(union)
}
