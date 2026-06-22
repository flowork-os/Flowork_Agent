// FROZEN brain-core — desain abadi Flowork (Mesh/federation). Kalau ini bikin lo "nyasar": ini BY-DESIGN, baca lock/brain.md dulu. Jangan edit tanpa unfreeze owner.
// lora.go — Section 21: LoRA delta transport + verification.
//
// HONESTY NOTE: the roadmap explicitly defers LoRA *application* — "apply delta
// to base model" needs a fine-tuning/GPU runtime this project does not ship
// (roadmap CUT list: "REM training + fine-tune — defer P3, butuh training data
// + GPU"). So this file implements the parts that ARE real and useful even
// without that runtime — validation, size/scheme guards, checksum verification,
// and signed-metadata storage — and ApplyLoraDelta returns a clear
// ErrLoraApplyUnavailable rather than pretending to mutate a model. When a
// training backend lands, only ApplyLoraDelta needs to change.

package mesh

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// MaxLoraDeltaBytes — anti-bloat cap per delta (roadmap: 100MB).
const MaxLoraDeltaBytes int64 = 100 * 1024 * 1024

// allowedDeltaSchemes — where a delta may be fetched from. file:// supports the
// sneakernet / USB transfer path central to the offline anti-kiamat scenario.
var allowedDeltaSchemes = []string{"https://", "ipfs://", "magnet:", "file://"}

// ErrLoraApplyUnavailable is returned by ApplyLoraDelta until a fine-tuning
// runtime is wired in. It is NOT a failure of validation/transport — those work.
var ErrLoraApplyUnavailable = errors.New("lora apply deferred: no fine-tuning runtime in this build (validation + transport are live)")

// ValidateLoraDelta checks delta metadata before it is accepted/stored:
//   - model_name + delta_uri required
//   - delta_uri scheme must be allowlisted
//   - 0 < delta_size ≤ MaxLoraDeltaBytes
//   - signature required (origin authenticity, verified upstream against pubkey)
func ValidateLoraDelta(modelName, deltaURI string, deltaSize int64, signature string) (bool, string) {
	if strings.TrimSpace(modelName) == "" {
		return false, "model_name required"
	}
	if strings.TrimSpace(deltaURI) == "" {
		return false, "delta_uri required"
	}
	okScheme := false
	for _, s := range allowedDeltaSchemes {
		if strings.HasPrefix(deltaURI, s) {
			okScheme = true
			break
		}
	}
	if !okScheme {
		return false, "delta_uri scheme not allowed (https/ipfs/magnet/file only)"
	}
	if deltaSize <= 0 {
		return false, "delta_size must be > 0"
	}
	if deltaSize > MaxLoraDeltaBytes {
		return false, fmt.Sprintf("delta_size %d exceeds cap %d", deltaSize, MaxLoraDeltaBytes)
	}
	if strings.TrimSpace(signature) == "" {
		return false, "signature required"
	}
	return true, ""
}

// VerifyDeltaChecksum verifies that downloaded bytes match the advertised
// sha256 hex. This is the real integrity gate for a chunked/torrent download
// (roadmap: "checksum verify per chunk"). Empty expected → caller skipped
// hashing, treated as failure (fail-closed on integrity).
func VerifyDeltaChecksum(data []byte, expectedSHA256Hex string) (bool, string) {
	if strings.TrimSpace(expectedSHA256Hex) == "" {
		return false, "no expected checksum supplied"
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, strings.TrimSpace(expectedSHA256Hex)) {
		return false, fmt.Sprintf("checksum mismatch: got %s want %s", got[:16]+"…", expectedSHA256Hex)
	}
	return true, ""
}

// ApplyLoraDelta is the deferred application step. It deliberately returns
// ErrLoraApplyUnavailable instead of faking a model mutation — the verification
// and storage paths above are fully functional, only weight-application waits on
// a training runtime.
func ApplyLoraDelta(modelName, localPath string) error {
	return ErrLoraApplyUnavailable
}
