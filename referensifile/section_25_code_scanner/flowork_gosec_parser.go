//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type GosecResult struct {
	Issues []struct {
		Severity   string `json:"severity"`
		Confidence string `json:"confidence"`
		RuleID     string `json:"rule_id"`
		Details    string `json:"details"`
		File       string `json:"file"`
		Line       string `json:"line"`
		Code       string `json:"code"`
	} `json:"Issues"`
	Stats struct {
		Files int `json:"files"`
		Lines int `json:"lines"`
		Found int `json:"found"`
	} `json:"Stats"`
}

func main() {
	b, err := os.ReadFile("gosec-results")
	if err != nil {
		panic(err)
	}

	var res GosecResult
	if err := json.Unmarshal(b, &res); err != nil {
		panic(err)
	}

	out := "# Laporan Vulnerability: Audit File-Demi-File\n\n"
	out += fmt.Sprintf("Total Issues Found: %d\n\n", res.Stats.Found)

	// Group by File
	groups := make(map[string][]string)
	for _, issue := range res.Issues {
		relPath := issue.File
		if idx := strings.Index(relPath, "floworkos-go"); idx != -1 {
			relPath = relPath[idx+len("floworkos-go")+1:]
		}

		key := fmt.Sprintf("## 🗎 File: `%s`", relPath)
		item := fmt.Sprintf("- **Line(s)**: %s\n- **Severity**: %s (%s)\n- **Rule**: %s\n- **Details**: %s\n- **Code**:\n```go\n%s```\n", issue.Line, issue.Severity, issue.Confidence, issue.RuleID, issue.Details, issue.Code)
		groups[key] = append(groups[key], item)
	}

	for key, items := range groups {
		out += key + "\n"
		for _, item := range items {
			out += item + "\n"
		}
	}

	err = os.MkdirAll(filepath.Join("state", "scanner-reports"), 0755)
	if err != nil {
		panic(err)
	}
	err = os.WriteFile(filepath.Join("state", "scanner-reports", "file_by_file_gosec.md"), []byte(out), 0644)
	if err != nil {
		panic(err)
	}
	fmt.Println("Wrote to state/scanner-reports/file_by_file_gosec.md")
}
