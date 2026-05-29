//go:build ignore

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	fmt.Println("==========================================================")
	fmt.Println(" 🛡️  FLOWORK AUDITOR ARSENAL - FULL SCAN 🛡️  ")
	fmt.Println("==========================================================")
	fmt.Println("Menjalankan semua flowork_*_auditor.go (Mode: REPLACE-LOG)...")
	fmt.Println()

	// Pastikan direktori docs/bug ada
	reportPath := filepath.Join("state", "scanner-reports", "omni_arsenal_report.md")
	os.MkdirAll(filepath.Dir(reportPath), 0755)

	// Buat file laporan (os.Create otomatis REPLACE/Overwite file jika sudah ada)
	// Kita tidak pakai os.OpenFile(os.O_APPEND), jadi tidak akan "numpuk"
	file, err := os.Create(reportPath)
	if err != nil {
		fmt.Printf("Gagal membuat file laporan: %v\n", err)
		return
	}
	defer file.Close()

	// Menulis header laporan
	file.WriteString("# 🛡️ Laporan Keamanan OMNI-ARSENAL (Auto-Generated)\n")
	file.WriteString("> Laporan ini di-generate ulang (replace) otomatis setiap scanner dieksekusi.\n\n")
	file.WriteString("```log\n")

	// MultiWriter = cetak di Terminal IYA, simpan ke File JUGA.
	mw := io.MultiWriter(os.Stdout, file)

	// Baca otomatis seluruh file flowork_*_auditor.go di dalam folder scanner
	scripts, err := filepath.Glob(filepath.Join("scanner", "flowork_*_auditor.go"))
	if err != nil || len(scripts) == 0 {
		fmt.Println("❌ Gagal menemukan skrip auditor atau folder kosong!")
		return
	}

	for _, script := range scripts {
		if strings.Contains(script, "flowork_runner") {
			continue
		}
		header := fmt.Sprintf("\n>>> 🚀 LAUNCHING: %s\n", script)
		// Cetak header ke terminal & ke file
		mw.Write([]byte(header))

		cmd := exec.Command("go", "run", script)
		cmd.Stdout = mw
		cmd.Stderr = mw

		runErr := cmd.Run()
		if runErr != nil {
			fmt.Printf("❌ Gagal menjalankan %s: %v\n", script, runErr)
		}
	}

	file.WriteString("```\n")

	fmt.Println("\n==========================================================")
	fmt.Printf(" 🎉 SEMUA DIVISI SELESAI! Laporan timpa tersimpan di: %s\n", reportPath)
	fmt.Println("==========================================================")
}
