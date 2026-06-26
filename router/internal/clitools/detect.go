// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package clitools

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Status struct {
	ID             string         `json:"id"`
	DisplayName    string         `json:"displayName"`
	Installed      bool           `json:"installed"`
	BinaryPath     string         `json:"binaryPath,omitempty"`
	SettingsExists bool           `json:"settingsExists"`
	SettingsPath   string         `json:"settingsPath"`
	HasFlowRouter  bool           `json:"hasFlowRouter"`
	Format         ConfigFormat   `json:"format"`
	BaseURL        string         `json:"baseUrl,omitempty"`
	TokenSet       bool           `json:"tokenSet"`
	Settings       map[string]any `json:"settings,omitempty"`
	ErrorMessage   string         `json:"errorMessage,omitempty"`
}

func Detect(toolID string) (*Status, error) {
	t := Get(toolID)
	if t == nil {
		return nil, fmt.Errorf("unknown tool: %s", toolID)
	}
	st := &Status{
		ID:           t.ID,
		DisplayName:  t.DisplayName,
		SettingsPath: t.SettingsPath,
		Format:       t.Format,
	}

	if t.BinaryName != "" {
		bp := findBinary(t.BinaryName, t.BinaryAliases...)
		if bp != "" {
			st.Installed = true
			st.BinaryPath = bp
		}
	}

	if _, err := os.Stat(t.SettingsPath); err == nil {
		st.SettingsExists = true
		st.Installed = true
	}

	settings, err := readSettings(t)
	if err == nil && settings != nil {
		st.Settings = settings
		st.BaseURL = lookupNested(settings, t.BaseURLKey)
		st.TokenSet = lookupNested(settings, t.TokenKey) != ""
		if st.BaseURL != "" && isFlowRouterURL(st.BaseURL) {
			st.HasFlowRouter = true
		}
	} else if err != nil && !os.IsNotExist(err) {
		st.ErrorMessage = err.Error()
	}
	return st, nil
}

func DetectAll() []Status {
	tools := All()
	out := make([]Status, len(tools))
	for i, t := range tools {
		s, err := Detect(t.ID)
		if err != nil {
			out[i] = Status{ID: t.ID, DisplayName: t.DisplayName, ErrorMessage: err.Error()}
			continue
		}
		out[i] = *s
	}
	return out
}

func WriteEnv(toolID string, env map[string]any) (map[string]any, error) {
	t := Get(toolID)
	if t == nil {
		return nil, fmt.Errorf("unknown tool: %s", toolID)
	}

	if hasCustomWriter(toolID) {
		home, _ := os.UserHomeDir()
		return customWriters[toolID](home, env)
	}
	if t.Format == FormatProxy {

		return writeJSONSettings(t.SettingsPath, env, "")
	}
	switch t.Format {
	case FormatJSON:
		return writeJSONSettings(t.SettingsPath, env, t.ID)
	case FormatTOML:
		return writeTOMLSettings(t.SettingsPath, env)
	case FormatEnv:
		return writeEnvSettings(t.SettingsPath, env)
	}
	return nil, fmt.Errorf("unsupported format: %s", t.Format)
}

func ResetEnv(toolID string) error {
	t := Get(toolID)
	if t == nil {
		return fmt.Errorf("unknown tool: %s", toolID)
	}
	if _, err := os.Stat(t.SettingsPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	switch t.Format {
	case FormatJSON:
		return resetJSONKeys(t.SettingsPath, t.EnvKeys, t.ID)
	case FormatTOML:
		return resetTOMLKeys(t.SettingsPath, t.EnvKeys)
	case FormatEnv:
		return resetEnvKeys(t.SettingsPath, t.EnvKeys)
	case FormatProxy:
		return os.Remove(t.SettingsPath)
	}
	return nil
}

func findBinary(name string, aliases ...string) string {
	candidates := append([]string{name}, aliases...)
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if p, err := exec.LookPath(c); err == nil {
			return p
		}
	}
	return ""
}

func readSettings(t *Tool) (map[string]any, error) {
	data, err := os.ReadFile(t.SettingsPath)
	if err != nil {
		return nil, err
	}
	switch t.Format {
	case FormatJSON, FormatProxy:
		var out map[string]any
		if err := json.Unmarshal(data, &out); err != nil {
			return nil, fmt.Errorf("parse json: %w", err)
		}

		if t.ID == "claude" {
			if envMap, ok := out["env"].(map[string]any); ok {
				flat := map[string]any{}
				for k, v := range out {
					flat[k] = v
				}
				for k, v := range envMap {
					flat[k] = v
				}
				return flat, nil
			}
		}
		return out, nil
	case FormatTOML:
		return parseSimpleTOML(string(data)), nil
	case FormatEnv:
		return parseDotEnv(string(data)), nil
	}
	return nil, nil
}

func lookupNested(m map[string]any, key string) string {
	if key == "" || m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	parts := strings.Split(key, ".")
	cur := any(m)
	for _, p := range parts {
		mm, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = mm[p]
	}
	if s, ok := cur.(string); ok {
		return s
	}
	return ""
}

func isFlowRouterURL(s string) bool {
	low := strings.ToLower(s)
	return strings.Contains(low, "127.0.0.1:2402") ||
		strings.Contains(low, "localhost:2402") ||
		strings.Contains(low, "flow_router") ||
		strings.Contains(low, "flow-router")
}

func writeJSONSettings(path string, env map[string]any, toolID string) (map[string]any, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	var existing map[string]any
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &existing)
	}
	if existing == nil {
		existing = map[string]any{}
	}
	if toolID == "claude" {

		envMap, _ := existing["env"].(map[string]any)
		if envMap == nil {
			envMap = map[string]any{}
		}
		for k, v := range env {
			if k == "ANTHROPIC_BASE_URL" {
				if s, ok := v.(string); ok && !strings.HasSuffix(strings.TrimRight(s, "/"), "/v1") {
					v = strings.TrimRight(s, "/") + "/v1"
				}
			}
			envMap[k] = v
		}
		existing["env"] = envMap
		existing["hasCompletedOnboarding"] = true
	} else {

		for k, v := range env {
			setNested(existing, k, v)
		}
	}
	out, _ := json.MarshalIndent(existing, "", "  ")
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return nil, err
	}
	return existing, nil
}

func resetJSONKeys(path string, keys []string, toolID string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var existing map[string]any
	if err := json.Unmarshal(data, &existing); err != nil {
		return err
	}
	if toolID == "claude" {
		if envMap, ok := existing["env"].(map[string]any); ok {
			for _, k := range keys {
				delete(envMap, k)
			}
			if len(envMap) == 0 {
				delete(existing, "env")
			} else {
				existing["env"] = envMap
			}
		}
	} else {
		for _, k := range keys {
			deleteNested(existing, k)
		}
	}
	out, _ := json.MarshalIndent(existing, "", "  ")
	return os.WriteFile(path, out, 0o600)
}

func setNested(m map[string]any, key string, val any) {
	parts := strings.Split(key, ".")
	cur := m
	for i, p := range parts {
		if i == len(parts)-1 {
			cur[p] = val
			return
		}
		nxt, ok := cur[p].(map[string]any)
		if !ok {
			nxt = map[string]any{}
			cur[p] = nxt
		}
		cur = nxt
	}
}

func deleteNested(m map[string]any, key string) {
	parts := strings.Split(key, ".")
	if len(parts) == 1 {
		delete(m, key)
		return
	}
	cur := m
	for i, p := range parts {
		if i == len(parts)-1 {
			delete(cur, p)
			return
		}
		nxt, ok := cur[p].(map[string]any)
		if !ok {
			return
		}
		cur = nxt
	}
}

func parseSimpleTOML(s string) map[string]any {
	out := map[string]any{}
	section := ""
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.Trim(line[1:len(line)-1], " ")
			continue
		}
		eq := strings.Index(line, "=")
		if eq < 0 {
			continue
		}
		k := strings.TrimSpace(line[:eq])
		v := strings.TrimSpace(line[eq+1:])
		v = strings.TrimSuffix(strings.TrimPrefix(v, `"`), `"`)
		fullKey := k
		if section != "" {
			fullKey = section + "." + k
		}
		out[fullKey] = v
	}
	return out
}

func writeTOMLSettings(path string, env map[string]any) (map[string]any, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}

	existing := map[string]string{}
	if data, err := os.ReadFile(path); err == nil {
		for k, v := range parseSimpleTOML(string(data)) {
			if s, ok := v.(string); ok {
				existing[k] = s
			}
		}
	}

	for k, v := range env {
		if s, ok := v.(string); ok {
			existing[k] = s
		} else {
			b, _ := json.Marshal(v)
			existing[k] = string(b)
		}
	}

	bySection := map[string]map[string]string{}
	var rootKeys []string
	for k, v := range existing {
		dot := strings.Index(k, ".")
		if dot < 0 {
			rootKeys = append(rootKeys, k)
			if bySection[""] == nil {
				bySection[""] = map[string]string{}
			}
			bySection[""][k] = v
			continue
		}

		lastDot := strings.LastIndex(k, ".")
		sec := k[:lastDot]
		field := k[lastDot+1:]
		if bySection[sec] == nil {
			bySection[sec] = map[string]string{}
		}
		bySection[sec][field] = v
	}
	var sb strings.Builder
	if root, ok := bySection[""]; ok {
		for _, k := range rootKeys {
			fmt.Fprintf(&sb, "%s = %q\n", k, root[k])
		}
	}
	for sec, kv := range bySection {
		if sec == "" {
			continue
		}
		fmt.Fprintf(&sb, "\n[%s]\n", sec)
		for k, v := range kv {
			fmt.Fprintf(&sb, "%s = %q\n", k, v)
		}
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o600); err != nil {
		return nil, err
	}
	out := map[string]any{}
	for k, v := range existing {
		out[k] = v
	}
	return out, nil
}

func resetTOMLKeys(path string, keys []string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	current := parseSimpleTOML(string(data))
	for _, k := range keys {
		delete(current, k)
	}
	_, err = writeTOMLSettings(path, current)
	return err
}

func parseDotEnv(s string) map[string]any {
	out := map[string]any{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.Index(line, "=")
		if eq < 0 {
			continue
		}
		k := strings.TrimSpace(line[:eq])
		v := strings.TrimSpace(line[eq+1:])
		v = strings.TrimSuffix(strings.TrimPrefix(v, `"`), `"`)
		out[k] = v
	}
	return out
}

func writeEnvSettings(path string, env map[string]any) (map[string]any, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	existing := map[string]string{}
	if data, err := os.ReadFile(path); err == nil {
		for k, v := range parseDotEnv(string(data)) {
			if s, ok := v.(string); ok {
				existing[k] = s
			}
		}
	}
	for k, v := range env {
		if s, ok := v.(string); ok {
			existing[k] = s
		} else {
			b, _ := json.Marshal(v)
			existing[k] = string(b)
		}
	}
	var sb strings.Builder
	for k, v := range existing {
		fmt.Fprintf(&sb, "%s=%s\n", k, v)
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o600); err != nil {
		return nil, err
	}
	out := map[string]any{}
	for k, v := range existing {
		out[k] = v
	}
	return out, nil
}

func resetEnvKeys(path string, keys []string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	current := parseDotEnv(string(data))
	for _, k := range keys {
		delete(current, k)
	}
	_, err = writeEnvSettings(path, current)
	return err
}
