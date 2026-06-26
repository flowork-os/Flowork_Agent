// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

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

type ImportOptions struct {
	TargetRoot string
	Passphrase string
}

type ImportResult struct {
	Manifest    Manifest `json:"manifest"`
	FilesCount  int      `json:"files_count"`
	BytesWriten int64    `json:"bytes_written"`
}

func Import(r io.Reader, opts ImportOptions) (ImportResult, error) {
	var res ImportResult

	magic := make([]byte, 8)
	if _, err := io.ReadFull(r, magic); err != nil {
		return res, fmt.Errorf("read magic: %w", err)
	}

	var payload []byte
	switch string(magic) {
	case "FWSYNC0\x00":

		buf, err := io.ReadAll(io.LimitReader(r, 200*1024*1024))
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

		_ = os.Chmod(dest, os.FileMode(hdr.Mode))
		res.FilesCount++
		res.BytesWriten += n
	}
	return res, nil
}
