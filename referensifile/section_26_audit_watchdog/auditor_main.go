package main

import (
	"context"
	"fmt"
	"log"
	"os"

	brainproxy "github.com/teetah2402/flowork/brain/proxy"
	"github.com/teetah2402/flowork/internal/config"
	"github.com/teetah2402/flowork/internal/core"
	"github.com/teetah2402/flowork/internal/provider"
	"github.com/teetah2402/flowork/internal/tools"
)

// The Log Auditor: Specialty "WATCHER_AGENT_MODEL" (Llama-3 free-tier)
func main() {
	log.SetFlags(log.Ltime)
	log.Println("[AUDITOR] Satpam 24/7 penjaga terminal melaporkan diri...")

	os.Setenv("FLOWORK_MODEL_OVERRIDE", os.Getenv("WATCHER_AGENT_MODEL"))

	cfg, _ := config.Load(".")
	modelID := cfg.EffectiveModel()
	if modelID == "" {
		modelID = "meta-llama/llama-3-8b-instruct:free"
	}

	// BUG-W14 fix Sprint 3.5e: P-2 funnel — kalau KERNEL_URL set, route via
	// kernel /v1/chat (audit + budget centralized). Fallback ke direct OpenAI
	// kalau kernel ngga configured.
	var rawClient provider.Client
	if kurl := os.Getenv("KERNEL_URL"); kurl != "" {
		kc, kerr := provider.NewKernelProxyClient(provider.KernelProxyConfig{
			KernelURL: kurl,
			Token:     os.Getenv("FLOWORK_KERNEL_TOKEN"),
			WargaID:   "flowork-auditor",
			Model:     modelID,
		})
		if kerr != nil {
			log.Fatalf("Auditor: kernel proxy init: %v", kerr)
		}
		rawClient = kc
	} else {
		c, err := provider.NewOpenAIClient(provider.OpenAIConfig{
			BaseURL: os.Getenv("OPENAI_BASE_URL"),
			APIKey:  os.Getenv("OPENROUTER_API_KEY"),
			Model:   modelID,
		})
		if err != nil {
			log.Fatalf("Gagal terhubung dengan specialist OpenRouter: %v", err)
		}
		rawClient = c
	}
	workspace, _ := os.Getwd()
	client := brainproxy.WrapWithBrain(rawClient, workspace)

	sysPrompt := "Tugas Utama: Auditor Log Error. Kamu tidak memegang kode sumber. Kamu hanya dikirimkan patahan-patahan string log error dari terminal stderr atau crash dump. Saring log yang dikirimkan, abaikan jika ampas, namun bila kamu mendeteksi 'panic', 'fatal', atau 'nil pointer', segera buatkan rangkuman 1 kalimat ancamannya."

	// Gunakan regis kosong karena agen ini pasif cuma baca.
	reg := tools.NewRegistry(nil)
	agentConfig := core.AgentConfig{
		MaxSteps: 2,
	}

	agent := core.NewAgent(client, reg, agentConfig)
	session := core.NewSession(sysPrompt)

	log.Printf("[AUDITOR] Menyuntang sinyal menggunakan %s...", modelID)

	// Simulasikan input pipa log terakhir
	input := "Simulasi Log Tail: process flowork-chat exit code 1. Fatal: concurrent map read and map write at internal/hub.go:42"

	res, err := agent.RunTurn(context.Background(), session, input)
	if err != nil {
		log.Fatalf("Auditor tewas: %v", err)
	}

	fmt.Println("\n📢 [PANGGILAN S.O.S DARI AUDITOR LOKAL]")
	for _, ev := range res.Events {
		if ev.Kind == core.EventAssistant && ev.Content != "" {
			fmt.Println(ev.Content)
		}
	}
}
