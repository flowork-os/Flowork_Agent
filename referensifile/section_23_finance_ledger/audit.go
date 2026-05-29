package finance

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// LogAudit mencatat log pengeluaran dana Kementrian Keuangan dengan HMAC tanda tangan digital
// agar log kas tidak dapat diedit secara sepihak oleh AI (Tamper-Evident).
func LogAudit(agentName string, usdLimit float64) error {
	secret := os.Getenv("FLOWORK_OWNER_PASSWORD")
	if secret == "" {
		// I-A.2: fail-close — tanpa secret audit log tidak bisa diverifikasi.
		// Lebih aman tolak transaksi daripada pakai fallback yang diketahui penyerang.
		return fmt.Errorf("finance: FLOWORK_OWNER_PASSWORD tidak di-set; audit log tidak dapat di-sign — transaksi ditolak")
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	message := fmt.Sprintf("[%s] MINT_KEY: %s LIMIT: %.2f", timestamp, agentName, usdLimit)

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	signature := hex.EncodeToString(mac.Sum(nil))

	logEntry := fmt.Sprintf("%s | SIG: %s\n", message, signature)

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("finance: LogAudit: %w", err)
	}
	logPath := filepath.Join(home, ".flowork", "finance_audit.log")

	if err := os.MkdirAll(filepath.Dir(logPath), 0700); err != nil {
		return fmt.Errorf("finance: gagal membuat direktori log: %w", err)
	}
	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("finance: LogAudit: open: %w", err)
	}
	defer f.Close()

	// I-A.3: eksplisit chmod setelah create — diperlukan di Windows karena
	// mode bits di OpenFile tidak selalu dihormati saat file dibuat.
	if err := os.Chmod(logPath, 0600); err != nil {
		fmt.Fprintf(os.Stderr, "WARNING: Failed to set audit log permissions to 0600: %v\n", err)
	}

	if _, err := f.WriteString(logEntry); err != nil {
		return fmt.Errorf("finance: LogAudit: write: %w", err)
	}
	return nil
}
