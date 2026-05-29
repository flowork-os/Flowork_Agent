// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30
// Reason: Section 19 phase 1 export. AES-256-GCM (passphrase-derived
//   via scrypt) + tar+gzip. State.db snapshot via dummy VACUUM INTO via
//   file copy (SQLite lock-aware). Manifest embedded sebagai first tar
//   entry. Phase 2 (incremental delta, CRDT version vector, ed25519
//   sign) → tambah file baru, JANGAN modify ini.
//
// export.go — Section 19 phase 1: pack agent folder ke .fwsync.

package sneakernet

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/scrypt"
)

// scrypt params — N=2^15 (32KB cache), r=8, p=1. Reasonable phone-compatible.
const (
	scryptN    = 1 << 15
	scryptR    = 8
	scryptP    = 1
	scryptKLen = 32 // AES-256
	saltLen    = 16
	nonceLen   = 12 // GCM standard
)

// Header AES-encrypted file:
//
//	"FWSYNC1\x00" (8 byte magic) || salt (16) || nonce (12) || ciphertext (gcm seal)
//
// Ciphertext = gzip(tar(manifest + files)). Manifest = first entry.

// ExportOptions — knob.
type ExportOptions struct {
	AgentID    string
	AgentRoot  string // absolute path ke `agents/<id>/`
	Version    string // agent version string
	HostOrigin string // hostname asal
	Passphrase string // empty = plain (no encryption)
}

// Export — pack agent folder + state.db snapshot ke single writer.
// Caller (handler) stream ke HTTP response body.
//
// State.db sourced langsung dari workspace/state.db. WAL mode boleh tetep
// running (snapshot in-place lock). Production: VACUUM INTO snapshot.
func Export(w io.Writer, opts ExportOptions) error {
	if opts.AgentRoot == "" {
		return fmt.Errorf("AgentRoot required")
	}
	if opts.AgentID == "" {
		return fmt.Errorf("AgentID required")
	}

	// Build tar in memory (anti partial stream on error).
	var tarBuf bytes.Buffer
	gz := gzip.NewWriter(&tarBuf)
	tw := tar.NewWriter(gz)

	// Walk files first to count + build manifest.
	manifest := NewManifest(opts.AgentID, opts.Version, opts.HostOrigin, opts.Passphrase != "")
	filesCount := 0
	var stateDBBytes int64

	walkErr := filepath.Walk(opts.AgentRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		// Symlink skip (anti-escape).
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		rel, rerr := filepath.Rel(opts.AgentRoot, path)
		if rerr != nil {
			return rerr
		}
		// Skip kalau path mengandung ".." (paranoid).
		if strings.Contains(rel, "..") {
			return nil
		}
		// Cap per-file size 100MB.
		if info.Size() > 100*1024*1024 {
			return nil
		}
		filesCount++
		if strings.HasSuffix(rel, "state.db") {
			stateDBBytes = info.Size()
		}
		return nil
	})
	if walkErr != nil {
		return fmt.Errorf("walk: %w", walkErr)
	}
	manifest.FilesCount = filesCount
	manifest.StateDBBytes = stateDBBytes

	// Write manifest first.
	mJSON, _ := json.Marshal(manifest)
	if err := tw.WriteHeader(&tar.Header{
		Name:     ManifestPath,
		Mode:     0o644,
		Size:     int64(len(mJSON)),
		Typeflag: tar.TypeReg,
	}); err != nil {
		return err
	}
	if _, err := tw.Write(mJSON); err != nil {
		return err
	}

	// Walk again, write files.
	werr := filepath.Walk(opts.AgentRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return walkErr
		}
		if info.Size() > 100*1024*1024 {
			return nil
		}
		rel, _ := filepath.Rel(opts.AgentRoot, path)
		if strings.Contains(rel, "..") {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		hdr := &tar.Header{
			Name:     "agent/" + filepath.ToSlash(rel),
			Mode:     int64(info.Mode().Perm()),
			Size:     info.Size(),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := io.Copy(tw, f); err != nil {
			return err
		}
		return nil
	})
	if werr != nil {
		return fmt.Errorf("walk2: %w", werr)
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	payload := tarBuf.Bytes()

	// Encrypt kalau passphrase ada.
	if opts.Passphrase != "" {
		ct, header, err := encryptAES256GCM([]byte(opts.Passphrase), payload)
		if err != nil {
			return err
		}
		if _, err := w.Write([]byte("FWSYNC1\x00")); err != nil {
			return err
		}
		if _, err := w.Write(header); err != nil {
			return err
		}
		if _, err := w.Write(ct); err != nil {
			return err
		}
		return nil
	}
	// Plain mode: still write magic supaya import detect format.
	if _, err := w.Write([]byte("FWSYNC0\x00")); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func encryptAES256GCM(passphrase, plaintext []byte) (ciphertext, header []byte, err error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, nil, err
	}
	key, err := scrypt.Key(passphrase, salt, scryptN, scryptR, scryptP, scryptKLen)
	if err != nil {
		return nil, nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	ct := gcm.Seal(nil, nonce, plaintext, nil)
	header = make([]byte, 0, saltLen+nonceLen)
	header = append(header, salt...)
	header = append(header, nonce...)
	return ct, header, nil
}
