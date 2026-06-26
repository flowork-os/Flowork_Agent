// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package main

import (
	"context"
	"embed"
	"encoding/hex"
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/flowork-os/flowork_Router/internal/brain"
	"github.com/flowork-os/flowork_Router/internal/localai"
	"github.com/flowork-os/flowork_Router/internal/mesh"
	"github.com/flowork-os/flowork_Router/internal/policy"
	"github.com/flowork-os/flowork_Router/internal/store"

	_ "github.com/flowork-os/flowork_Router/internal/rtk/filters"

	_ "github.com/flowork-os/flowork_Router/internal/translator/request"
	_ "github.com/flowork-os/flowork_Router/internal/translator/response"

	_ "github.com/flowork-os/flowork_Router/internal/providers/embedding"
	_ "github.com/flowork-os/flowork_Router/internal/providers/image"
	_ "github.com/flowork-os/flowork_Router/internal/providers/tts"

	_ "github.com/flowork-os/flowork_Router/internal/search"
)

//go:embed web/static
var webFS embed.FS

//go:embed seed/router-config.seed.json
var seedConfigJSON []byte

const version = "1.0.0-phase1.5-all-features-functional"

func main() {
	addr := flag.String("addr", "127.0.0.1:2402", "listen address")
	exportSeed := flag.String("export-seed", "", "write the current config (secrets blanked) to this path as a first-run seed, then exit")
	flag.Parse()

	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()
	_ = ctx

	log.Printf("flow_router %s starting on %s", version, *addr)
	log.Printf("Data: %s", store.DBPath())

	d, err := store.Open()
	if err != nil {
		log.Fatalf("storage init: %v", err)
	}
	defer store.Close()

	if *exportSeed != "" {
		raw, merr := json.MarshalIndent(store.ExportSeed(d), "", "  ")
		if merr != nil {
			log.Fatalf("export-seed marshal: %v", merr)
		}
		if werr := os.WriteFile(*exportSeed, append(raw, '\n'), 0o644); werr != nil {
			log.Fatalf("export-seed write: %v", werr)
		}
		log.Printf("config seed written to %s (secrets blanked)", *exportSeed)
		return
	}

	if n := store.SeedFromBundleJSON(d, seedConfigJSON); n != nil {
		log.Printf("first-run config seed applied: %v", n)
	}
	if err := store.SeedDefaults(d); err != nil {
		log.Printf("WARN: seed defaults: %v", err)
	}
	if err := store.AugmentTierTags(d); err != nil {
		log.Printf("WARN: augment tier tags: %v", err)
	}
	if err := store.SeedDefaultPricing(d); err != nil {
		log.Printf("WARN: seed pricing: %v", err)
	}

	if s, _ := store.LoadSettings(d); s != nil && s.Brain.Enabled {
		if s.Brain.DBPath != "" {
			brain.SetDBPath(s.Brain.DBPath)
		}
		if nd, nc, err := brain.SeedDoctrine(); err != nil {
			log.Printf("WARN: seed doctrine: %v", err)
		} else if nd > 0 || nc > 0 {
			log.Printf("brain: Flowork doctrine seeded — %d drawers + %d constitution", nd, nc)
		}

		if ni, err := brain.SeedInstincts(); err != nil {
			log.Printf("WARN: seed instinct: %v", err)
		} else if ni > 0 {
			log.Printf("brain: Flowork instincts seeded — %d", ni)
		}
	}
	if err := store.PurgeExpiredSessions(d); err != nil {
		log.Printf("WARN: purge sessions: %v", err)
	}

	if id, err := mesh.EnsureIdentity(d, version); err != nil {
		log.Printf("WARN: mesh identity: %v", err)
	} else {
		shortKey := id.PubKeyHex
		if len(shortKey) > 16 {
			shortKey = shortKey[:16]
		}
		log.Printf("Mesh identity: %s... (host=%s)", shortKey, id.Hostname)

		pubkeyBytes, _ := hex.DecodeString(id.PubKeyHex)

		meshPort := 2402
		if _, ps, perr := net.SplitHostPort(*addr); perr == nil {
			if p, aerr := strconv.Atoi(ps); aerr == nil && p > 0 {
				meshPort = p
			}
		}
		discovery := mesh.NewDiscovery(pubkeyBytes, meshPort, version, d)

		discovery.WhitelistCheck = mesh.KarmaGate(d)
		if derr := discovery.Start(ctx); derr != nil {
			log.Printf("WARN: mesh discovery: %v", derr)
		}
	}

	gossipEngine := mesh.NewGossipEngine(d)
	gossipEngine.Start(ctx)
	defer gossipEngine.Stop()

	policyEngine := policy.New(d)
	policyEngine.Start(ctx)
	defer policyEngine.Stop()
	policyEngineRef = policyEngine

	loadMITMCaptureState()
	loadLearnCaptureState()
	loadLocalAIAutostartState()
	startTunnelWatchdog()
	providers, _ := store.ListProviders(d)
	log.Printf("Providers loaded: %d", len(providers))
	for _, p := range providers {
		status := "off"
		if p.IsActive {
			status = "on"
		}
		log.Printf("  - [%s] %s (%s, priority=%d)", status, p.Name, p.AuthType, p.Priority)
	}

	go maybeAutostartLocalAI(providers)

	llmIdleSleepInit()

	go func() {
		if n, err := brain.RebuildFreshIndex(ctx); err != nil {
			log.Printf("WARN: fresh-index boot rebuild: %v", err)
		} else if n > 0 {
			log.Printf("brain: fresh-recall index built (%d federation drawers)", n)
		}
		t := time.NewTicker(2 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if n, err := brain.RebuildFreshIndex(ctx); err != nil {
					log.Printf("WARN: fresh-index rebuild: %v", err)
				} else if n > 0 {
					log.Printf("brain: fresh-recall index refreshed (%d federation drawers)", n)
				}
			}
		}
	}()

	startDreamGraphAutoSync(ctx)

	mux := http.NewServeMux()

	registerRoutes(mux)

	srv := &http.Server{
		Addr: *addr,

		Handler: apiKeyMiddleware(authEnforceMiddleware(mux)),

		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
		WriteTimeout:      10 * time.Minute,
	}

	shutdownTriggerCh = make(chan struct{}, 1)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case <-shutdownTriggerCh:
			log.Printf("flow_router: shutdown requested via API")
		case s := <-sigCh:
			log.Printf("flow_router: signal %s received", s)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		stopMITMOnShutdown()
		_ = srv.Shutdown(ctx)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("flow_router serve error: %v", err)
		os.Exit(1)
	}
	log.Printf("flow_router stopped cleanly")
}

func maybeAutostartLocalAI(providers []store.ProviderConnection) {

	if !localAIAutostartEnabled() {
		log.Printf("localai autostart: OFF — nyalain lewat toggle GUI router (Settings → Local AI autostart)")
		return
	}
	want := false
	for _, p := range providers {
		if !p.IsActive {
			continue
		}
		if base, _ := p.Data[store.CfgBaseURL].(string); strings.Contains(base, ":8088") {
			want = true
			break
		}
	}
	if !want {
		return
	}
	gguf := localai.ResolveFloworkBrain()
	bin := localai.ResolveLlamaBin()
	if gguf == "" || bin == "" {
		log.Printf("localai autostart: skip — gguf=%q bin=%q (set FLOWORK_BRAIN_GGUF / FLOWORK_LLAMA_BIN or install the model)", gguf, bin)
		return
	}
	localAIRuntimeMu.Lock()
	if localAIRuntimeRef == nil {
		localAIRuntimeRef = localai.NewRuntime(bin, 0)
	}
	rt := localAIRuntimeRef
	localAIRuntimeMu.Unlock()
	log.Printf("localai autostart: starting flowork-brain (bin=%s)", bin)
	if err := rt.Start(localai.FloworkBrainModel, gguf); err != nil {
		log.Printf("localai autostart: failed (%v) — agents fall back to cloud/priority", err)
		return
	}
	log.Printf("localai autostart: flowork-brain UP on :8088")
}
