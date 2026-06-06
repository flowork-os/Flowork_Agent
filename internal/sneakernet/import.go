// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 19 phase 1 import. Verify magic + decrypt + extract +
//   validate manifest + write ke target directory. Idempotent — overwrite
//   existing folder (caller harus konfirmasi). Anti-zip-slip via
//   path sanity check. Phase 2 (CRDT merge state.db, atomic-rename) →
//   tambah file baru, JANGAN modify ini.
//
// import.go — Section 19 phase 1: unpack .fwsync ke target folder.

package sneakernet

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/scrypt"
)

// ImportOptions — knob.
type ImportOptions struct {
	TargetRoot string // dir tujuan (akan dibuat kalau belum ada)
	Passphrase string // empty kalau plain mode (magic = FWSYNC0)
}

// ImportResult — diagnostic untuk handler.
type ImportResult struct {
	Manifest    Manifest `json:"manifest"`
	FilesCount  int      `json:"files_count"`
	BytesWriten int64    `json:"bytes_written"`
}

// Import — reverse of Export. Read magic, decrypt, untar, write files.
// TargetRoot must NOT contain pre-existing data caller anggap valuable —
// import overwrites by filename.
func Import(r io.Reader, opts ImportOptions) (ImportResult, error) {
	var res ImportResult

	// Read magic 8 byte.
	magic := make([]byte, 8)
	if _, err := io.ReadFull(r, magic); err != nil {
		return res, fmt.Errorf("read magic: %w", err)
	}

	var payload []byte
	switch string(magic) {
	case "FWSYNC0\x00":
		// Plain — read remainder.
		buf, err := io.ReadAll(io.LimitReader(r, 200*1024*1024)) // cap 200MB
		if err != nil {
			return res, err
		}
		payload = buf
	case "FWSYNC1\x00":
		if opts.Passphrase == "" {
			return res, fmt.Errorf("passphrase required for encrypted .fwsync")
		}
		header := make([]byte, saltLen+nonceLen)
		if _, err := io.ReadFull(r, header); err != nil {
			return res, fmt.Errorf("read header: %w", err)
		}
		salt := header[:saltLen]
		nonce := header[saltLen:]
		ct, err := io.ReadAll(io.LimitReader(r, 200*1024*1024))
		if err != nil {
			return res, err
		}
		key, err := scrypt.Key([]byte(opts.Passphrase), salt, scryptN, scryptR, scryptP, scryptKLen)
		if err != nil {
			return res, fmt.Errorf("scrypt: %w", err)
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return res, err
		}
		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return res, err
		}
		pt, err := gcm.Open(nil, nonce, ct, nil)
		if err != nil {
			return res, fmt.Errorf("decrypt failed (wrong passphrase?): %w", err)
		}
		payload = pt
	default:
		return res, fmt.Errorf("bad magic %q (expected FWSYNC0/1)", string(magic))
	}

	// Decompress tar.
	gz, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return res, fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	if err := os.MkdirAll(opts.TargetRoot, 0o755); err != nil {
		return res, err
	}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return res, err
		}
		// Anti zip-slip / .. traversal.
		clean := filepath.Clean(hdr.Name)
		if strings.Contains(clean, "..") || filepath.IsAbs(clean) {
			continue
		}
		switch hdr.Name {
		case ManifestPath:
			body, _ := io.ReadAll(io.LimitReader(tr, 1*1024*1024))
			if err := json.Unmarshal(body, &res.Manifest); err != nil {
				return res, fmt.Errorf("manifest decode: %w", err)
			}
			continue
		}
		if !strings.HasPrefix(hdr.Name, "agent/") {
			continue
		}
		rel := strings.TrimPrefix(hdr.Name, "agent/")
		dest := filepath.Join(opts.TargetRoot, rel)
		// Ensure dir.
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return res, err
		}
		out, err := os.Create(dest)
		if err != nil {
			return res, fmt.Errorf("create %s: %w", dest, err)
		}
		n, err := io.Copy(out, tr)
		out.Close()
		if err != nil {
			return res, fmt.Errorf("write %s: %w", dest, err)
		}
		// Chmod from header (best-effort).
		_ = os.Chmod(dest, os.FileMode(hdr.Mode))
		res.FilesCount++
		res.BytesWriten += n
	}
	return res, nil
}
