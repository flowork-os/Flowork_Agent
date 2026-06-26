// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package utils

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var stdin = bufio.NewScanner(os.Stdin)

func init() {
	stdin.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
}

func Prompt(label, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", label, def)
	} else {
		fmt.Printf("%s: ", label)
	}
	if !stdin.Scan() {
		return def
	}
	s := strings.TrimSpace(stdin.Text())
	if s == "" {
		return def
	}
	return s
}

func Confirm(label string, def bool) bool {
	defStr := "y/N"
	if def {
		defStr = "Y/n"
	}
	fmt.Printf("%s [%s]: ", label, defStr)
	if !stdin.Scan() {
		return def
	}
	s := strings.ToLower(strings.TrimSpace(stdin.Text()))
	if s == "" {
		return def
	}
	return s == "y" || s == "yes"
}

func Select(prompt string, labels []string) int {
	fmt.Println()
	for i, l := range labels {
		fmt.Printf("  %d) %s\n", i+1, l)
	}
	fmt.Printf("\n%s (1-%d): ", prompt, len(labels))
	if !stdin.Scan() {
		return -1
	}
	s := strings.TrimSpace(stdin.Text())
	if s == "" {
		return -1
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > len(labels) {
		return -1
	}
	return n - 1
}

func PromptInt(label string, def int) int {
	s := Prompt(label, strconv.Itoa(def))
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func PromptSecret(label string) string { return Prompt(label, "") }
