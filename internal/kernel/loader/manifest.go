// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-05-30 (re-locked 2026-06-11)
// 2026-06-11 OWNER-APPROVED (frozen, KERNEL_FREEZE hash regenerated): primitive
//   whitelist += "mcp" so an agent can hold `mcp:<connector>` and call MCP tools
//   via tool.run (completes MCP-for-agents; the comment already invited this).
//   Pure addition to the allowed set — does not loosen any existing rule.
// Reason: Manifest parser + validator. Audit pass — DisallowUnknownFields
//   (anti typo), strict regex (ID, semver, capability syntax), reject `*`
//   wildcard (anti privesc), known primitive whitelist (fs/net/kv/exec/bus/
//   secret/time/rpc/state/mcp), boundary checks (memory 1-512MB, timeout 1-300s),
//   batch error reporting (errors.Join), lifecycle nil-safe, duplicate RPC
//   detection. Note: primitive whitelist butuh update saat spec evolve.
//
// Package loader — manifest parser + plugin folder scanner.
//
//// Manifest parser.
// Validasi rules dari section 2 "Validation rules (kernel-side parser)".

package loader

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Manifest is the in-memory representation of a plugin's manifest.json.
//
type Manifest struct {
	// Identity
	ID          string `json:"id"`
	Version     string `json:"version"`
	Kind        Kind   `json:"kind"`
	DisplayName string `json:"display_name"`
	Description string `json:"description,omitempty"`

	// Compatibility
	MinKernelVersion string `json:"min_kernel_version"`
	MaxKernelVersion string `json:"max_kernel_version,omitempty"`
	ABIVersion       int    `json:"abi_version"`

	// Author / signing
	Author    string `json:"author,omitempty"`
	AuthorURL string `json:"author_url,omitempty"`
	License   string `json:"license,omitempty"`
	Signature string `json:"signature,omitempty"`

	// Runtime
	Entry            string `json:"entry"`
	MemoryMaxMB      int    `json:"memory_max_mb,omitempty"`
	TimeoutInitMS    int    `json:"timeout_init_ms,omitempty"`
	TimeoutCallMS    int    `json:"timeout_call_ms,omitempty"`

	// Capabilities (granular per SPEC_CAPABILITY)
	CapabilitiesRequired []string `json:"capabilities_required,omitempty"`
	CapabilitiesOptional []string `json:"capabilities_optional,omitempty"`

	// Inter-plugin
	DependsOn   []Dependency `json:"depends_on,omitempty"`
	ExposesRPC  []RPCMethod  `json:"exposes_rpc,omitempty"`

	// Bus topics
	PublishesTopics  []BusTopic `json:"publishes_topics,omitempty"`
	SubscribesTopics []string   `json:"subscribes_topics,omitempty"`

	// UI
	UIContributes *UIContrib `json:"ui_contributes,omitempty"`

	// UISchema — schema declarative untuk popup setting. Agent declare
	// extra section + field; popup renderer otomatis bikin form. Section
	// standar (Prompt/Schedule/Tools/Skills/Workspace/Settings/Database)
	// SELALU muncul; UISchema cuma nambah section di bawahnya.
	UISchema *UISchema `json:"ui_schema,omitempty"`

	// i18n
	I18nKeys map[string]map[string]string `json:"i18n_keys,omitempty"`

	// Config
	KVDefaults map[string]any `json:"kv_defaults,omitempty"`

	// Assets
	Assets []Asset `json:"assets,omitempty"`

	// Lifecycle hooks (defaults applied at load time)
	Lifecycle *Lifecycle `json:"lifecycle,omitempty"`
}

// Kind — plugin kind enum.
type Kind string

const (
	KindAgent    Kind = "agent"
	KindToolPack Kind = "tool-pack"
	KindChannel  Kind = "channel"
	KindGUI      Kind = "gui"
	KindInfra    Kind = "infra"
	KindService  Kind = "service"
)

func (k Kind) Valid() bool {
	switch k {
	case KindAgent, KindToolPack, KindChannel, KindGUI, KindInfra, KindService:
		return true
	}
	return false
}

type Dependency struct {
	ID         string `json:"id"`
	MinVersion string `json:"min_version,omitempty"`
}

type RPCMethod struct {
	Name         string         `json:"name"`
	Description  string         `json:"description,omitempty"`
	InputSchema  map[string]any `json:"input_schema,omitempty"`
	OutputSchema map[string]any `json:"output_schema,omitempty"`
}

type BusTopic struct {
	Name   string         `json:"name"`
	Schema map[string]any `json:"schema,omitempty"`
}

type UIContrib struct {
	Tab            *UITab            `json:"tab,omitempty"`
	SettingsPanel  *UISettingsPanel  `json:"settings_panel,omitempty"`
}

type UITab struct {
	Path      string `json:"path"`
	LabelKey  string `json:"label_key"`
	Icon      string `json:"icon,omitempty"`
	ParentHub string `json:"parent_hub,omitempty"`
}

type UISettingsPanel struct {
	Path     string `json:"path"`
	LabelKey string `json:"label_key"`
}

// UISchema — declarative schema buat extra section di popup setting.
// Agent declare di manifest.json; popup renderer baca + bikin field
// otomatis tanpa nulis HTML/JS.
type UISchema struct {
	Sections []UISection `json:"sections,omitempty"`
}

// UISection — satu section custom di popup (di bawah 7 standar).
type UISection struct {
	ID          string    `json:"id"`                     // unique slug
	Title       string    `json:"title"`                  // header label
	Icon        string    `json:"icon,omitempty"`         // emoji prefix
	Description string    `json:"description,omitempty"`  // sub-text under header
	Fields      []UIField `json:"fields,omitempty"`
}

// UIField — input dalam UISection. Renderer pick widget berdasar Type.
type UIField struct {
	Key         string         `json:"key"`                 // nama (= env var kalau storage=secrets)
	Label       string         `json:"label"`               // user-facing label
	Type        string         `json:"type"`                // text|password|textarea|number|select|checkbox|json
	Placeholder string         `json:"placeholder,omitempty"`
	Default     any            `json:"default,omitempty"`
	Required    bool           `json:"required,omitempty"`
	Storage     string         `json:"storage,omitempty"`   // secrets|kv|meta (default: secrets)
	Options     []UIFieldOpt   `json:"options,omitempty"`   // buat type=select
	Validation  string         `json:"validation,omitempty"`// regex:^pattern$ atau min:N, max:N
	Help        string         `json:"help,omitempty"`      // tooltip text
}

// UIFieldOpt — option buat select.
type UIFieldOpt struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type Asset struct {
	Path    string `json:"path"`
	Purpose string `json:"purpose"`
}

type Lifecycle struct {
	OnInstall            string `json:"on_install,omitempty"`
	OnLoad               string `json:"on_load,omitempty"`
	OnUnload             string `json:"on_unload,omitempty"`
	OnCapabilityRevoked  string `json:"on_capability_revoked,omitempty"`
}

// Default field values applied during Parse when fields are zero/empty.
const (
	DefaultEntry         = "plugin.wasm"
	DefaultMemoryMaxMB   = 16
	DefaultTimeoutInitMS = 5000
	DefaultTimeoutCallMS = 30000
)

// Defaults for lifecycle hook names (referenced by name as wasm exports).
const (
	DefaultOnInstall   = "init"
	DefaultOnLoad      = "boot"
	DefaultOnUnload    = "shutdown"
	DefaultOnCapChange = "permission_changed"
)

// reID — matches `^[a-z][a-z0-9-]{2,31}$`
var reID = regexp.MustCompile(`^[a-z][a-z0-9-]{2,31}$`)

// reSemver — light SemVer matcher; ignores build metadata that isn't
// meaningful at parse time. Strict semver canonicalisation deferred until
// version comparison code in a later phase.
var reSemver = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.\-]+)?$`)

// reCapability — per SPEC_CAPABILITY section 2 sintaks granular.
//
//	<primitive>                              primitive only (ditolak by default)
//	<primitive>:<sub>                        sub scope
//	<primitive>:<sub>:<arg>                  arg boleh berisi `:` (mis. URL `http://host:port/path`)
//
// Regex:
//   - primitive = `[a-z]+`
//   - optional `:` + sub `[a-z][a-z0-9_-]*`
//   - optional `:` + arg `[^\s]+` (semua non-whitespace, termasuk `:` dan `/`)
var reCapability = regexp.MustCompile(`^[a-z]+(:[a-z][a-z0-9_-]*(:[^\s]+)?)?$`)

// Parse decodes the manifest bytes and applies default values. Validation
//validation rules below are enforced; the first failure wins.
func Parse(raw []byte) (*Manifest, error) {
	var m Manifest
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields() // catch typos early; SPEC freezes schema
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}
	if err := m.applyDefaults(); err != nil {
		return nil, fmt.Errorf("apply defaults: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

func (m *Manifest) applyDefaults() error {
	if m.Entry == "" {
		m.Entry = DefaultEntry
	}
	if m.MemoryMaxMB == 0 {
		m.MemoryMaxMB = DefaultMemoryMaxMB
	}
	if m.TimeoutInitMS == 0 {
		m.TimeoutInitMS = DefaultTimeoutInitMS
	}
	if m.TimeoutCallMS == 0 {
		m.TimeoutCallMS = DefaultTimeoutCallMS
	}
	if m.Lifecycle == nil {
		m.Lifecycle = &Lifecycle{}
	}
	if m.Lifecycle.OnInstall == "" {
		m.Lifecycle.OnInstall = DefaultOnInstall
	}
	if m.Lifecycle.OnLoad == "" {
		m.Lifecycle.OnLoad = DefaultOnLoad
	}
	if m.Lifecycle.OnUnload == "" {
		m.Lifecycle.OnUnload = DefaultOnUnload
	}
	if m.Lifecycle.OnCapabilityRevoked == "" {
		m.Lifecycle.OnCapabilityRevoked = DefaultOnCapChange
	}
	return nil
}

// Validate runs structural validation. Errors are
// joined so a developer sees every problem in one pass instead of the
// fix-one-find-next loop.
func (m *Manifest) Validate() error {
	var errs []error

	if !reID.MatchString(m.ID) {
		errs = append(errs, fmt.Errorf("invalid id %q: must match ^[a-z][a-z0-9-]{2,31}$", m.ID))
	}
	if !reSemver.MatchString(m.Version) {
		errs = append(errs, fmt.Errorf("invalid version %q: must be SemVer", m.Version))
	}
	if !m.Kind.Valid() {
		errs = append(errs, fmt.Errorf("invalid kind %q: enum agent|tool-pack|channel|gui|infra|service", m.Kind))
	}
	if strings.TrimSpace(m.DisplayName) == "" {
		errs = append(errs, errors.New("display_name required"))
	}
	if !reSemver.MatchString(m.MinKernelVersion) {
		errs = append(errs, fmt.Errorf("invalid min_kernel_version %q: must be SemVer", m.MinKernelVersion))
	}
	if m.MaxKernelVersion != "" && !reSemver.MatchString(m.MaxKernelVersion) {
		errs = append(errs, fmt.Errorf("invalid max_kernel_version %q: must be SemVer", m.MaxKernelVersion))
	}
	if m.ABIVersion <= 0 {
		errs = append(errs, fmt.Errorf("abi_version required and > 0 (got %d)", m.ABIVersion))
	}
	if strings.TrimSpace(m.Entry) == "" {
		errs = append(errs, errors.New("entry required"))
	}
	if m.MemoryMaxMB <= 0 || m.MemoryMaxMB > 512 {
		errs = append(errs, fmt.Errorf("memory_max_mb out of range (1-512): %d", m.MemoryMaxMB))
	}
	if m.TimeoutInitMS <= 0 || m.TimeoutInitMS > 60_000 {
		errs = append(errs, fmt.Errorf("timeout_init_ms out of range (1-60000): %d", m.TimeoutInitMS))
	}
	if m.TimeoutCallMS <= 0 || m.TimeoutCallMS > 300_000 {
		errs = append(errs, fmt.Errorf("timeout_call_ms out of range (1-300000): %d", m.TimeoutCallMS))
	}

	// Capability syntax — per SPEC_CAPABILITY section 2 ("wildcard di
	// posisi <primitive> ditolak"). Single "*" rejected here too.
	for i, c := range m.CapabilitiesRequired {
		if err := validateCapability(c); err != nil {
			errs = append(errs, fmt.Errorf("capabilities_required[%d] %q: %w", i, c, err))
		}
	}
	for i, c := range m.CapabilitiesOptional {
		if err := validateCapability(c); err != nil {
			errs = append(errs, fmt.Errorf("capabilities_optional[%d] %q: %w", i, c, err))
		}
	}

	// Dependency ids must be valid plugin id (reuse reID).
	for i, d := range m.DependsOn {
		if !reID.MatchString(d.ID) {
			errs = append(errs, fmt.Errorf("depends_on[%d].id %q: invalid id", i, d.ID))
		}
		if d.MinVersion != "" && !reSemver.MatchString(d.MinVersion) {
			errs = append(errs, fmt.Errorf("depends_on[%d].min_version %q: invalid SemVer", i, d.MinVersion))
		}
	}

	// RPC method names — snake_case; unique within plugin.
	rpcSeen := map[string]bool{}
	reRPC := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	for i, r := range m.ExposesRPC {
		if !reRPC.MatchString(r.Name) {
			errs = append(errs, fmt.Errorf("exposes_rpc[%d].name %q: must be snake_case", i, r.Name))
		}
		if rpcSeen[r.Name] {
			errs = append(errs, fmt.Errorf("exposes_rpc[%d].name %q: duplicate within plugin", i, r.Name))
		}
		rpcSeen[r.Name] = true
	}

	// Bus topic names — dot.notation.
	reTopic := regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z0-9]+)*$`)
	for i, t := range m.PublishesTopics {
		if !reTopic.MatchString(t.Name) {
			errs = append(errs, fmt.Errorf("publishes_topics[%d].name %q: must be dot.notation", i, t.Name))
		}
	}
	for i, t := range m.SubscribesTopics {
		if !reTopic.MatchString(t) {
			errs = append(errs, fmt.Errorf("subscribes_topics[%d] %q: must be dot.notation", i, t))
		}
	}

	// i18n: every value object must include "en" per SPEC.
	for k, locales := range m.I18nKeys {
		if _, ok := locales["en"]; !ok {
			errs = append(errs, fmt.Errorf("i18n_keys[%q]: missing 'en' fallback", k))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// validateCapability — checks one capability string against SPEC_CAPABILITY.
//
//   reject `*` standalone
//   reject `<primitive>:*` (wildcard at primitive position is implicit in `:*` after primitive)
//   accept primitive[:sub[:arg]]
//   accept glob `*` inside sub/arg
func validateCapability(c string) error {
	c = strings.TrimSpace(c)
	if c == "" {
		return errors.New("empty capability")
	}
	if c == "*" {
		return errors.New("standalone wildcard not allowed")
	}
	if !reCapability.MatchString(c) {
		return errors.New("syntax: <primitive>[:<sub>[:<arg>]]")
	}
	parts := strings.SplitN(c, ":", 2)
	primitive := parts[0]
	if primitive == "*" || primitive == "" {
		return errors.New("primitive cannot be wildcard or empty")
	}
	// Known primitive enforcement — drop in extras here as spec evolves.
	switch primitive {
	case "fs", "net", "kv", "exec", "bus", "secret", "time", "rpc", "state", "mcp":
		// known. `state` ditambah 2026-05-29 untuk host_log_interaction
		// (`state:write`) — log row ke tabel `interactions` di state.db
		// agent. Lihat internal/kernel/runtime/host.go::logInteraction.
		// `mcp` ditambah 2026-06-11 (owner-approved): cap `mcp:<connector>` —
		// izin agent manggil tool MCP (mcp_<id>_<tool>) lewat tool.run; broker
		// prefix-match `mcp:web` ke approved `mcp`. Tanpa ini MCP-buat-agent
		// setengah jadi (tool kedaftar tapi agent gak bisa di-grant cap-nya).
	default:
		return fmt.Errorf("unknown primitive %q", primitive)
	}
	return nil
}
