// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package tts

import (
	"context"
	"errors"
)

func init() { Register(&localDeviceProvider{}) }

type localDeviceProvider struct{}

func (l *localDeviceProvider) Name() string { return "localDevice" }

func (l *localDeviceProvider) Speak(ctx context.Context, req Request) ([]byte, string, error) {

	return nil, "", errors.New("localDevice TTS is browser-rendered; route via dashboard's Web Speech API")
}
