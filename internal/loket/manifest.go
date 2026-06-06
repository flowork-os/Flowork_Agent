package loket

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Kind is the type of a module. The kernel wires every kind through the same
// manifest + the same call() contract; the kind only changes which extra fields
// are required and how the kernel routes inbound messages.
type Kind string

const (
	KindAgent   Kind = "agent"   // an AI worker (one "ant")
	KindGroup   Kind = "group"   // a team/colony of agents with tasks
	KindChannel Kind = "channel" // an I/O adapter (telegram/discord/cli/web)
	KindScanner Kind = "scanner" // a security scanner
	KindService Kind = "service" // a shared capability provider (llm, brain.shared)
	KindTool    Kind = "tool"    // a single domain capability
)

var validKinds = map[Kind]bool{
	KindAgent: true, KindGroup: true, KindChannel: true,
	KindScanner: true, KindService: true, KindTool: true,
}

// Tier of an agent module. An extension keeps its brain in its own folder and
// cannot touch the 5M shared corpus; a primary is engine-fused and may.
type Tier string

const (
	TierExtension Tier = "extension" // own-folder brain only (default)
	TierPrimary   Tier = "primary"   // engine-fused, may consume tier-gated caps
)

// Manifest is a module's identity card. The kernel reads it, validates it
// against this frozen shape, has the owner approve high-risk caps, and wires the
// module — with zero kernel code per module. Dropping a folder with a manifest
// is how a feature plugs in.
type Manifest struct {
	ID         string `json:"id"`
	Kind       Kind   `json:"kind"`
	Name       string `json:"name"` // user-facing, ENGLISH (open-source / global market)
	Version    string `json:"version"`
	ABIVersion string `json:"abi_version"`
	Entry      string `json:"entry"` // wasm export the kernel calls (the handle func)

	Tier     Tier     `json:"tier,omitempty"`     // agent only
	Consumes []string `json:"consumes,omitempty"` // capabilities it will call()
	Provides []string `json:"provides,omitempty"` // capabilities it offers (service/tool)

	GUI     json.RawMessage `json:"gui,omitempty"`     // declarative panel schema
	Members []string        `json:"members,omitempty"` // group: member module ids
	Tasks   []TaskSpec      `json:"tasks,omitempty"`   // group: the jobs it runs
	Config  []ConfigField   `json:"config,omitempty"`  // owner-settable fields (token, target, …)

	Meta ManifestMeta `json:"meta,omitempty"`
}

// ConfigField declares one owner-settable setting a module needs (e.g. a connector's
// bot token). The kernel/GUI render the field from this schema and store the value in
// the MODULE'S OWN store — the single source of truth — so the kernel never hardcodes
// any module's keys. A connector thus owns its credential and nothing is duplicated.
type ConfigField struct {
	Key     string `json:"key"`               // the storage/env key, e.g. "TELEGRAM_BOT_TOKEN"
	Label   string `json:"label"`             // human label for the GUI
	Type    string `json:"type,omitempty"`    // "text" | "secret" (default "text")
	Default string `json:"default,omitempty"` // prefilled value
	Help    string `json:"help,omitempty"`    // optional hint
}

// TaskSpec is one job a group can run across its members.
type TaskSpec struct {
	Name        string `json:"name"`
	Synthesizer string `json:"synthesizer,omitempty"` // module id that combines member outputs
}

// ManifestMeta holds non-functional metadata.
type ManifestMeta struct {
	Author      string `json:"author,omitempty"`
	Description string `json:"description,omitempty"`
}

var idRe = regexp.MustCompile(`^[a-z][a-z0-9-]{1,63}$`)

// ParseManifest decodes and validates a manifest.json payload.
func ParseManifest(raw []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("manifest parse: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

// Validate checks a manifest against the frozen contract. It rejects unknown
// kinds, malformed ids, unknown consumed capabilities, and — as a hard
// architectural rule — an extension that asks for a tier-gated capability.
//
// Note the deliberate asymmetry: a GrantOwner capability is allowed in the
// manifest (it is approved by the owner at install time), but a GrantTier
// capability on a non-primary module is rejected outright, because tier is an
// architectural boundary, not an approval the owner can override.
func (m *Manifest) Validate() error {
	if !idRe.MatchString(m.ID) {
		return fmt.Errorf("manifest id %q invalid (want ^[a-z][a-z0-9-]{1,63}$)", m.ID)
	}
	if !validKinds[m.Kind] {
		return fmt.Errorf("manifest kind %q invalid (agent|group|channel|scanner|service|tool)", m.Kind)
	}
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("manifest name required")
	}
	if strings.TrimSpace(m.Entry) == "" {
		return fmt.Errorf("manifest entry required")
	}
	if m.ABIVersion != "" && m.ABIVersion != ABIVersion {
		return fmt.Errorf("manifest abi_version %q unsupported (kernel speaks %q)", m.ABIVersion, ABIVersion)
	}
	isPrimary := m.Tier == TierPrimary
	for _, c := range m.Consumes {
		spec, ok := LookupCap(c)
		if !ok {
			return fmt.Errorf("manifest consumes unknown capability %q", c)
		}
		if spec.Grant == GrantTier && !isPrimary {
			return fmt.Errorf("capability %q is primary-only; module %q is tier %q", c, m.ID, m.tierOrDefault())
		}
	}
	for _, c := range m.Provides {
		// A service may offer a NEW capability name (extending the namespace), so
		// Provides is not forced into the frozen Catalog. Only sanity-check shape.
		if strings.TrimSpace(c) == "" {
			return fmt.Errorf("manifest provides contains an empty capability")
		}
	}
	if m.Kind == KindGroup && len(m.Members) == 0 {
		return fmt.Errorf("group %q must declare at least one member", m.ID)
	}
	return nil
}

func (m *Manifest) tierOrDefault() Tier {
	if m.Tier == "" {
		return TierExtension
	}
	return m.Tier
}
