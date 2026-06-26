// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package executors

import (
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"runtime"
	"time"
)

const cursorBase64Alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

func CursorHashed64Hex(input, salt string) string {
	sum := sha256.Sum256([]byte(input + salt))
	return hex.EncodeToString(sum[:])
}

func GenerateCursorChecksum(machineID string) string {

	ts := time.Now().UnixMilli() / 1_000_000

	bytes := [6]byte{
		byte((ts >> 40) & 0xFF),
		byte((ts >> 32) & 0xFF),
		byte((ts >> 24) & 0xFF),
		byte((ts >> 16) & 0xFF),
		byte((ts >> 8) & 0xFF),
		byte(ts & 0xFF),
	}

	t := byte(165)
	for i := 0; i < len(bytes); i++ {
		bytes[i] = byte((int(bytes[i]^t) + (i % 256)) & 0xFF)
		t = bytes[i]
	}

	var encoded []byte
	for i := 0; i < len(bytes); i += 3 {
		a := bytes[i]
		var b, c byte
		if i+1 < len(bytes) {
			b = bytes[i+1]
		}
		if i+2 < len(bytes) {
			c = bytes[i+2]
		}
		encoded = append(encoded, cursorBase64Alphabet[a>>2])
		encoded = append(encoded, cursorBase64Alphabet[((a&3)<<4)|(b>>4)])
		if i+1 < len(bytes) {
			encoded = append(encoded, cursorBase64Alphabet[((b&15)<<2)|(c>>6)])
		}
		if i+2 < len(bytes) {
			encoded = append(encoded, cursorBase64Alphabet[c&63])
		}
	}
	return string(encoded) + machineID
}

func BuildCursorHeaders(accessToken, machineID string, ghostMode bool) map[string]string {

	cleanToken := accessToken
	for i := 0; i < len(cleanToken)-1; i++ {
		if cleanToken[i] == ':' && cleanToken[i+1] == ':' {
			cleanToken = cleanToken[i+2:]
			break
		}
	}

	if machineID == "" {
		machineID = CursorHashed64Hex(cleanToken, "machineId")
	}
	sessionID := cursorUUIDv5DNS(cleanToken)
	clientKey := CursorHashed64Hex(cleanToken, "")
	checksum := GenerateCursorChecksum(machineID)

	osName := "linux"
	switch runtime.GOOS {
	case "windows":
		osName = "windows"
	case "darwin":
		osName = "macos"
	}
	arch := "x64"
	if runtime.GOARCH == "arm64" {
		arch = "aarch64"
	}

	ghost := "false"
	if ghostMode {
		ghost = "true"
	}

	return map[string]string{
		"authorization":               "Bearer " + cleanToken,
		"connect-accept-encoding":     "gzip",
		"connect-protocol-version":    "1",
		"content-type":                "application/connect+proto",
		"user-agent":                  "connect-es/1.6.1",
		"x-amzn-trace-id":             "Root=" + randomUUIDStr(),
		"x-client-key":                clientKey,
		"x-cursor-checksum":           checksum,
		"x-cursor-client-version":     "3.1.0",
		"x-cursor-client-type":        "ide",
		"x-cursor-client-os":          osName,
		"x-cursor-client-arch":        arch,
		"x-cursor-client-device-type": "desktop",
		"x-cursor-config-version":     randomUUIDStr(),
		"x-cursor-timezone":           "UTC",
		"x-ghost-mode":                ghost,
		"x-request-id":                randomUUIDStr(),
		"x-session-id":                sessionID,
	}
}

func cursorUUIDv5DNS(name string) string {
	dnsNs := []byte{
		0x6b, 0xa7, 0xb8, 0x10,
		0x9d, 0xad,
		0x11, 0xd1,
		0x80, 0xb4,
		0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8,
	}
	h := sha1.New()
	h.Write(dnsNs)
	h.Write([]byte(name))
	hash := h.Sum(nil)

	hash[6] = (hash[6] & 0x0F) | 0x50
	hash[8] = (hash[8] & 0x3F) | 0x80
	return formatUUID(hash[:16])
}

func formatUUID(b []byte) string {
	const hexd = "0123456789abcdef"
	out := make([]byte, 36)
	dashes := map[int]bool{8: true, 13: true, 18: true, 23: true}
	bi := 0
	for i := 0; i < 36; i++ {
		if dashes[i] {
			out[i] = '-'
			continue
		}
		out[i] = hexd[b[bi]>>4]
		i++
		out[i] = hexd[b[bi]&0xF]
		bi++
	}
	return string(out)
}

func randomUUIDStr() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0F) | 0x40
	b[8] = (b[8] & 0x3F) | 0x80
	return formatUUID(b[:])
}
