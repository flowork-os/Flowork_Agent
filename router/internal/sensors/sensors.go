// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package sensors

import (
	"crypto/subtle"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

const AlgoVersion = "v1"

var reSourceID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]{1,31}$`)

var ErrUnknownSource = errors.New("unknown sensor source")

var ErrInvalidToken = errors.New("invalid sensor token")

var ErrInvalidSourceID = errors.New("invalid source id format")

func ValidateSource(sourceID string) (expectedToken string, err error) {
	if !reSourceID.MatchString(sourceID) {
		return "", ErrInvalidSourceID
	}
	envKey := "FLOW_ROUTER_SENSOR_" + strings.ToUpper(strings.ReplaceAll(sourceID, "-", "_")) + "_TOKEN"
	expectedToken = os.Getenv(envKey)
	if expectedToken == "" {
		return "", fmt.Errorf("%w: %s (env %s not set)", ErrUnknownSource, sourceID, envKey)
	}
	return expectedToken, nil
}

func CompareToken(expected, provided string) bool {
	if expected == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(provided)) == 1
}

func AuthSource(sourceID, providedToken string) error {
	expected, err := ValidateSource(sourceID)
	if err != nil {

		return err
	}
	if !CompareToken(expected, providedToken) {
		return ErrInvalidToken
	}
	return nil
}
