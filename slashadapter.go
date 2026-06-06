// slashadapter.go — SLASH-PACK plug-and-play (roadmap multi-KIND `slash`).
// Pola IDENTIK tool-pack: slash command = wasm "slash-agent" (kind:agent),
// WasmSlash adapter implement slashcmd.SlashCommand; Run invoke wasm via
// host.InvokeAgentMessage (REUSE, NOL ubah kernel/locked). slashcmd udah punya
// Unregister/Has (registry_dynamic.go). Persist via marker slash.json + boot scan.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"flowork-gui/internal/kernel/loader"
	"flowork-gui/internal/kernelhost"
	"flowork-gui/internal/slashcmd"
)

var slashNameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,31}$`)

// slashSpec — definisi 1 slash command plugin (plugin.json + marker slash.json).
type slashSpec struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Aliases     []string `json:"aliases"`
	AgentID     string   `json:"agent_id"`
}

// WasmSlash — adapter slashcmd.SlashCommand yang eksekusi lewat wasm slash-agent.
type WasmSlash struct {
	spec slashSpec
	host *kernelhost.Host
}

func (s *WasmSlash) Name() string        { return s.spec.Name }
func (s *WasmSlash) Aliases() []string   { return s.spec.Aliases }
func (s *WasmSlash) Description() string { return s.spec.Description }

func (s *WasmSlash) Run(ctx context.Context, argsRaw string) (slashcmd.Result, error) {
	if s.host == nil {
		return slashcmd.Result{}, fmt.Errorf("slash /%s: host ga ke-wire", s.spec.Name)
	}
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	// argsRaw = teks plain (argumen slash). host ekstrak `reply` → balik teks respons.
	reply, err := s.host.InvokeAgentMessage(cctx, s.spec.AgentID, argsRaw, "slash:"+s.spec.Name)
	if err != nil {
		return slashcmd.Result{}, fmt.Errorf("invoke slash-agent %q: %w", s.spec.AgentID, err)
	}
	return slashcmd.Result{Text: reply, Format: "markdown"}, nil
}

func slashMarkerPath(agentID string) string {
	return filepath.Join(loader.AgentsDir(), agentID+".fwagent", "slash.json")
}

// registerWasmSlash — register WasmSlash (recover dari panic Register: dup
// name/alias). Tulis marker kalau persist.
func registerWasmSlash(host *kernelhost.Host, spec slashSpec, persist bool) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("register slash /%s: %v", spec.Name, r)
		}
	}()
	if spec.Name == "" || spec.AgentID == "" {
		return fmt.Errorf("slashSpec invalid (name/agent_id kosong)")
	}
	slashcmd.Register(&WasmSlash{spec: spec, host: host})
	if persist {
		raw, _ := json.MarshalIndent(spec, "", "  ")
		_ = os.WriteFile(slashMarkerPath(spec.AgentID), raw, 0o644)
	}
	return nil
}

// findSlashAgent — agent_id yang punya slash bernama `name` (scan marker).
func findSlashAgent(name string) string {
	name = strings.ToLower(name)
	root := loader.AgentsDir()
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		raw, rerr := os.ReadFile(filepath.Join(root, e.Name(), "slash.json"))
		if rerr != nil {
			continue
		}
		var spec slashSpec
		if json.Unmarshal(raw, &spec) == nil && strings.ToLower(spec.Name) == name {
			return spec.AgentID
		}
	}
	return ""
}

// installedSlashPacks — daftar slash plugin (dari marker) buat GUI.
func installedSlashPacks() []slashSpec {
	out := []slashSpec{}
	root := loader.AgentsDir()
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		raw, rerr := os.ReadFile(filepath.Join(root, e.Name(), "slash.json"))
		if rerr != nil {
			continue
		}
		var spec slashSpec
		if json.Unmarshal(raw, &spec) == nil && spec.Name != "" {
			out = append(out, spec)
		}
	}
	return out
}

// reregisterSlashPacksOnBoot — scan marker slash.json → re-register pas boot.
// Skip kalau nama udah ke-register (builtin) — proteksi inti.
func reregisterSlashPacksOnBoot(host *kernelhost.Host) int {
	n := 0
	for _, spec := range installedSlashPacks() {
		if slashcmd.Has(spec.Name) {
			fmt.Fprintf(os.Stderr, "[slash-pack] boot skip %q (bentrok builtin)\n", spec.Name)
			continue
		}
		if err := registerWasmSlash(host, spec, false); err == nil {
			n++
		} else {
			fmt.Fprintf(os.Stderr, "[slash-pack] boot re-register %q: %v\n", spec.Name, err)
		}
	}
	return n
}
