// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package clitools

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const flowRouterProviderKey = "flow_router"

type customWriter func(home string, env map[string]any) (map[string]any, error)

var customWriters = map[string]customWriter{
	"hermes":   writeHermes,
	"openclaw": writeOpenclaw,
	"codex":    writeCodex,
	"kilo":     writeKilo,
}

func hasCustomWriter(toolID string) bool {
	_, ok := customWriters[toolID]
	return ok
}

func IsCustom(toolID string) bool { return hasCustomWriter(toolID) }

func BuildConnectEnv(toolID, baseURL, apiKey, model string) map[string]any {
	t := Get(toolID)
	if t == nil {
		return map[string]any{"baseUrl": baseURL, "apiKey": apiKey, "model": model}
	}
	if hasCustomWriter(toolID) {
		return map[string]any{"baseUrl": baseURL, "apiKey": apiKey, "model": model}
	}
	env := map[string]any{}
	if t.BaseURLKey != "" {

		url := baseURL
		if toolID == "claude" {
			url = ensureV1(baseURL)
		}
		env[t.BaseURLKey] = url
	}
	if t.TokenKey != "" && apiKey != "" {
		env[t.TokenKey] = apiKey
	}
	return env
}

func strField(env map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := env[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

func ensureV1(u string) string {
	u = strings.TrimRight(u, "/")
	if u == "" {
		return u
	}
	if strings.HasSuffix(u, "/v1") {
		return u
	}
	return u + "/v1"
}

func writeHermes(home string, env map[string]any) (map[string]any, error) {
	dir := filepath.Join(home, ".hermes")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	baseURL := strField(env, "baseUrl", "base_url", "OPENAI_BASE_URL", "OPENAI_API_BASE")
	apiKey := strField(env, "apiKey", "api_key", "OPENAI_API_KEY")
	if baseURL == "" {
		return nil, fmt.Errorf("hermes: baseUrl required")
	}

	cfgPath := filepath.Join(dir, "config.yaml")
	modelBlock := fmt.Sprintf("model:\n  provider: \"custom\"\n  base_url: %q\n", baseURL)
	existing := ""
	if b, err := os.ReadFile(cfgPath); err == nil {
		existing = string(b)
	}
	var out string
	reModel := regexp.MustCompile(`(?m)^model:[ \t]*\r?\n(?:[ \t]+.*\r?\n?|[ \t]*\r?\n)*`)
	if reModel.MatchString(existing) {
		out = reModel.ReplaceAllString(existing, modelBlock)
	} else {
		out = modelBlock + existing
	}
	if err := os.WriteFile(cfgPath, []byte(out), 0o600); err != nil {
		return nil, err
	}

	envPath := filepath.Join(dir, ".env")
	if apiKey != "" {
		envContent := map[string]string{}
		if b, err := os.ReadFile(envPath); err == nil {
			for k, v := range parseDotEnv(string(b)) {
				if s, ok := v.(string); ok {
					envContent[k] = s
				}
			}
		}
		envContent["OPENAI_API_KEY"] = apiKey
		var sb strings.Builder
		for k, v := range envContent {
			fmt.Fprintf(&sb, "%s=%s\n", k, v)
		}
		if err := os.WriteFile(envPath, []byte(sb.String()), 0o600); err != nil {
			return nil, err
		}
	}
	return map[string]any{
		"configYaml": cfgPath,
		"envFile":    envPath,
		"model":      map[string]any{"provider": "custom", "base_url": baseURL},
		"tokenSet":   apiKey != "",
	}, nil
}

func writeOpenclaw(home string, env map[string]any) (map[string]any, error) {
	dir := filepath.Join(home, ".openclaw")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	baseURL := ensureV1(strField(env, "baseUrl", "base_url"))
	apiKey := strField(env, "apiKey", "api_key")
	model := strField(env, "model")
	if baseURL == "" || model == "" {
		return nil, fmt.Errorf("openclaw: baseUrl and model required")
	}
	if apiKey == "" {
		apiKey = "your_api_key"
	}

	modelsPath := filepath.Join(dir, "models.json")
	modelsDoc := map[string]any{}
	if b, err := os.ReadFile(modelsPath); err == nil {
		_ = json.Unmarshal(b, &modelsDoc)
	}
	providers, _ := modelsDoc["providers"].(map[string]any)
	if providers == nil {
		providers = map[string]any{}
	}
	providers[flowRouterProviderKey] = map[string]any{
		"baseUrl": baseURL,
		"apiKey":  apiKey,
		"models":  []map[string]any{{"id": model}},
	}
	modelsDoc["providers"] = providers
	mb, _ := json.MarshalIndent(modelsDoc, "", "  ")
	if err := os.WriteFile(modelsPath, mb, 0o600); err != nil {
		return nil, err
	}

	cfgPath := filepath.Join(dir, "openclaw.json")
	cfg := map[string]any{}
	if b, err := os.ReadFile(cfgPath); err == nil {
		_ = json.Unmarshal(b, &cfg)
	}
	agents, _ := cfg["agents"].(map[string]any)
	if agents == nil {
		agents = map[string]any{}
	}
	defaults, _ := agents["defaults"].(map[string]any)
	if defaults == nil {
		defaults = map[string]any{}
	}
	modelObj, _ := defaults["model"].(map[string]any)
	if modelObj == nil {
		modelObj = map[string]any{}
	}
	primary := flowRouterProviderKey + "/" + model
	modelObj["primary"] = primary
	defaults["model"] = modelObj

	if dm, ok := defaults["models"].(map[string]any); ok {
		for k := range dm {
			if strings.HasPrefix(k, flowRouterProviderKey+"/") {
				delete(dm, k)
			}
		}
		defaults["models"] = dm
	}
	agents["defaults"] = defaults
	cfg["agents"] = agents
	if _, ok := cfg["models"].(map[string]any); !ok {
		cfg["models"] = map[string]any{"providers": map[string]any{}}
	}
	cb, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.WriteFile(cfgPath, cb, 0o600); err != nil {
		return nil, err
	}
	return map[string]any{
		"openclawJson": cfgPath,
		"modelsJson":   modelsPath,
		"primaryModel": primary,
		"baseUrl":      baseURL,
	}, nil
}

func writeCodex(home string, env map[string]any) (map[string]any, error) {
	dir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	baseURL := ensureV1(strField(env, "baseUrl", "base_url", "model_providers.openai.base_url"))
	apiKey := strField(env, "apiKey", "api_key", "OPENAI_API_KEY")
	model := strField(env, "model")
	if baseURL == "" {
		return nil, fmt.Errorf("codex: baseUrl required")
	}

	cfgPath := filepath.Join(dir, "config.toml")
	existing := map[string]string{}
	if b, err := os.ReadFile(cfgPath); err == nil {
		for k, v := range parseSimpleTOML(string(b)) {
			if s, ok := v.(string); ok {
				existing[k] = s
			}
		}
	}
	if model != "" {
		existing["model"] = model
	}
	existing["model_provider"] = flowRouterProviderKey
	sec := "model_providers." + flowRouterProviderKey
	existing[sec+".name"] = flowRouterProviderKey
	existing[sec+".base_url"] = baseURL
	existing[sec+".wire_api"] = "responses"
	if _, err := writeTOMLSettings(cfgPath, mapStrToAny(existing)); err != nil {
		return nil, err
	}

	authPath := filepath.Join(dir, "auth.json")
	auth := map[string]any{}
	if b, err := os.ReadFile(authPath); err == nil {
		_ = json.Unmarshal(b, &auth)
	}
	if apiKey != "" {
		auth["OPENAI_API_KEY"] = apiKey
	}
	ab, _ := json.MarshalIndent(auth, "", "  ")
	if err := os.WriteFile(authPath, ab, 0o600); err != nil {
		return nil, err
	}
	return map[string]any{
		"configToml":    cfgPath,
		"authJson":      authPath,
		"modelProvider": flowRouterProviderKey,
		"baseUrl":       baseURL,
		"tokenSet":      apiKey != "",
	}, nil
}

func writeKilo(home string, env map[string]any) (map[string]any, error) {
	cfgDir := filepath.Join(home, ".config", "kilo")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		return nil, err
	}
	baseURL := ensureV1(strField(env, "baseUrl", "base_url"))
	apiKey := strField(env, "apiKey", "api_key")
	model := strField(env, "model")
	if baseURL == "" || apiKey == "" || model == "" {
		return nil, fmt.Errorf("kilo: baseUrl, apiKey and model required")
	}

	settingsPath := filepath.Join(cfgDir, "settings.json")
	settings := map[string]any{}
	if b, err := os.ReadFile(settingsPath); err == nil {
		_ = json.Unmarshal(b, &settings)
	}
	settings["kilocode.customProvider"] = map[string]any{
		"name":    "Flow Router",
		"baseURL": baseURL,
		"apiKey":  apiKey,
		"model":   model,
	}
	sb, _ := json.MarshalIndent(settings, "", "  ")
	if err := os.WriteFile(settingsPath, sb, 0o600); err != nil {
		return nil, err
	}

	authPath := filepath.Join(cfgDir, "auth.json")
	auth := map[string]any{flowRouterProviderKey: map[string]any{"apiKey": apiKey, "baseUrl": baseURL}}
	ab, _ := json.MarshalIndent(auth, "", "  ")
	if err := os.WriteFile(authPath, ab, 0o600); err != nil {
		return nil, err
	}
	return map[string]any{
		"settingsJson": settingsPath,
		"authJson":     authPath,
		"baseUrl":      baseURL,
		"model":        model,
	}, nil
}

func mapStrToAny(m map[string]string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
