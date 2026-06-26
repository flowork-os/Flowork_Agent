// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package scanner

import (
	"regexp"
	"strings"
)

func init() {
	Auditors["error_string_format_auditor"] = AuditErrorStringFormat
	Auditors["todo_comment_auditor"] = AuditTodoComment
	Auditors["debug_fmt_print_auditor"] = AuditDebugFmtPrint
	Auditors["switch_no_default_auditor"] = AuditSwitchNoDefault
	Auditors["shadowed_err_auditor"] = AuditShadowedErr
	Auditors["ineffective_assign_auditor"] = AuditIneffectiveAssign
	Auditors["conditional_inversion_auditor"] = AuditConditionalInversion
	Auditors["redundant_nil_check_auditor"] = AuditRedundantNilCheck
	Auditors["unused_var_auditor"] = AuditUnusedVar
	Auditors["missing_doc_comment_auditor"] = AuditMissingDocComment
}

var errCapitalRE = regexp.MustCompile(`errors\.New\s*\(\s*"[A-Z]`)
var errPeriodRE = regexp.MustCompile(`errors\.New\s*\(\s*"[^"]*\.\s*"\s*\)`)
var fmtErrorfCapRE = regexp.MustCompile(`fmt\.Errorf\s*\(\s*"[A-Z]`)

func AuditErrorStringFormat(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if errCapitalRE.MatchString(line) || fmtErrorfCapRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "error_string_format_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "error string mulai dengan capital — Go convention: lowercase, no period",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "lowercase + no trailing period: `errors.New(\"foo bar\")`",
			})
		}
		if errPeriodRE.MatchString(line) {
			out = append(out, Finding{
				Auditor:     "error_string_format_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "error string end dengan '.' — Go convention violated",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "hapus period akhir: `errors.New(\"foo bar\")`",
			})
		}
	}
	return out
}

var todoRE = regexp.MustCompile(`(?i)//\s*(TODO|FIXME|XXX|HACK|BUG)\b`)

func AuditTodoComment(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if m := todoRE.FindStringSubmatch(line); m != nil {
			sev := SevLow
			if strings.ToUpper(m[1]) == "BUG" || strings.ToUpper(m[1]) == "FIXME" {
				sev = SevMedium
			}
			out = append(out, Finding{
				Auditor:     "todo_comment_auditor",
				Severity:    sev,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     m[1] + " comment — track or resolve",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "resolve, atau move ke roadmap/issue tracker dengan due date",
			})
		}
	}
	return out
}

var debugPrintRE = regexp.MustCompile(`^\s*fmt\.(Println|Printf|Print)\s*\(`)

func AuditDebugFmtPrint(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") || strings.HasSuffix(filePath, "main.go") {
		return nil
	}
	out := []Finding{}
	for i, line := range strings.Split(content, "\n") {
		if debugPrintRE.MatchString(line) {

			if strings.Contains(line, "Fprint") {
				continue
			}
			out = append(out, Finding{
				Auditor:     "debug_fmt_print_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "fmt.Println/Printf di non-main — likely leftover debug, gunakan log.* untuk structured",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "ganti ke log.Printf untuk audit trail, atau hapus kalau debug temporary",
			})
		}
	}
	return out
}

func AuditSwitchNoDefault(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}
	out := []Finding{}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "switch ") || !strings.HasSuffix(trimmed, "{") {
			continue
		}

		depth := 1
		hasDefault := false
		for j := i + 1; j < len(lines) && depth > 0; j++ {
			depth += strings.Count(lines[j], "{") - strings.Count(lines[j], "}")
			if strings.HasPrefix(strings.TrimSpace(lines[j]), "default:") {
				hasDefault = true
			}
		}
		if !hasDefault {
			out = append(out, Finding{
				Auditor:     "switch_no_default_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "switch tanpa default — unexpected value silent ignored",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "tambah `default:` dengan error/log untuk catch typo+new values",
			})
		}
	}
	return out
}

var shadowedErrRE = regexp.MustCompile(`^\s*if\s+(\w+,\s*)?err\s*:=`)

func AuditShadowedErr(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}
	out := []Finding{}
	lines := strings.Split(content, "\n")

	for i, line := range lines {
		if !shadowedErrRE.MatchString(line) {
			continue
		}

		for j := i - 1; j >= maxInt(0, i-15); j-- {
			prev := strings.TrimSpace(lines[j])
			if strings.HasPrefix(prev, "err := ") || strings.HasPrefix(prev, "err = ") {
				out = append(out, Finding{
					Auditor:     "shadowed_err_auditor",
					Severity:    SevLow,
					FilePath:    filePath,
					LineNumber:  i + 1,
					Message:     "`if err :=` may shadow outer err (line " + intToStr(j+1) + ")",
					Snippet:     truncateSnippet(line, 120),
					Remediation: "kalau intend reuse, pakai `if err = ...` tanpa `:=`",
				})
				break
			}
		}
	}
	return out
}

var ineffectiveRE = regexp.MustCompile(`^\s*(\w+)\s*:?=\s*`)

func AuditIneffectiveAssign(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}
	out := []Finding{}
	lines := strings.Split(content, "\n")
	for i := 0; i < len(lines)-1; i++ {
		m := ineffectiveRE.FindStringSubmatch(lines[i])
		if m == nil {
			continue
		}
		v := m[1]

		if v == "err" || v == "_" || v == "ok" || v == "" {
			continue
		}
		nextLine := strings.TrimSpace(lines[i+1])
		if strings.HasPrefix(nextLine, v+" =") || strings.HasPrefix(nextLine, v+" :=") {
			out = append(out, Finding{
				Auditor:     "ineffective_assign_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "var " + v + " ditulis lalu langsung di-overwrite next line",
				Snippet:     truncateSnippet(lines[i], 120),
				Remediation: "hapus assignment yang ditimpa, atau merge ke single statement",
			})
		}
	}
	return out
}

var negCondRE = regexp.MustCompile(`^\s*if\s+!\w+\s*\{$`)

func AuditConditionalInversion(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") {
		return nil
	}
	out := []Finding{}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if !negCondRE.MatchString(line) {
			continue
		}

		_ = i
		_ = lines
	}
	return out
}

func AuditRedundantNilCheck(filePath, content string) []Finding {

	return nil
}

var declarationRE = regexp.MustCompile(`^\s*var\s+(\w+)\s+`)

func AuditUnusedVar(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}
	out := []Finding{}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		m := declarationRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := m[1]

		if name == "" || (name[0] >= 'A' && name[0] <= 'Z') {
			continue
		}

		usageCount := strings.Count(content, name) - 1
		if usageCount == 0 {
			out = append(out, Finding{
				Auditor:     "unused_var_auditor",
				Severity:    SevLow,
				FilePath:    filePath,
				LineNumber:  i + 1,
				Message:     "var " + name + " declared but tidak terpakai (heuristic)",
				Snippet:     truncateSnippet(line, 120),
				Remediation: "hapus var atau prefix `_` kalau intentional",
			})
		}
	}
	return out
}

var exportedFuncRE = regexp.MustCompile(`^func\s+(\(\w+\s+\*?\w+\)\s+)?([A-Z]\w*)`)
var exportedTypeRE = regexp.MustCompile(`^type\s+([A-Z]\w*)\s+(struct|interface)`)

func AuditMissingDocComment(filePath, content string) []Finding {
	if !strings.HasSuffix(filePath, ".go") || strings.HasSuffix(filePath, "_test.go") {
		return nil
	}
	out := []Finding{}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		var name string
		if m := exportedFuncRE.FindStringSubmatch(line); m != nil {
			name = m[2]
		} else if m := exportedTypeRE.FindStringSubmatch(line); m != nil {
			name = m[1]
		}
		if name == "" {
			continue
		}

		if i == 0 {
			continue
		}
		prev := strings.TrimSpace(lines[i-1])
		if strings.HasPrefix(prev, "//") {
			continue
		}
		out = append(out, Finding{
			Auditor:     "missing_doc_comment_auditor",
			Severity:    SevLow,
			FilePath:    filePath,
			LineNumber:  i + 1,
			Message:     "exported `" + name + "` tanpa doc comment — godoc kosong",
			Snippet:     truncateSnippet(line, 120),
			Remediation: "tambah baris `// " + name + " — keterangan singkat...` sebelum declaration",
		})
	}
	return out
}
