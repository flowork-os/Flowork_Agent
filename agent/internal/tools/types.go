// Flowork OS — Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
// Cara kerja sistem: lihat os/.  ⚠️ FROZEN — jangan edit file ini.
// Nambah/ubah fitur TANPA buka frozen: pakai SEAM non-frozen + SWITCH
// (internal/fwswitch/registry.go). Pola lengkap: lock/frozen-core.md

package tools

import (
	"context"
	"encoding/json"
)

const AlgoVersion = "v1"

type ParamType string

const (
	ParamString ParamType = "string"
	ParamInt    ParamType = "int"
	ParamFloat  ParamType = "float"
	ParamBool   ParamType = "bool"
	ParamObject ParamType = "object"
	ParamArray  ParamType = "array"
)

type Param struct {
	Name        string    `json:"name"`
	Type        ParamType `json:"type"`
	Description string    `json:"description"`
	Required    bool      `json:"required"`
	Default     any       `json:"default,omitempty"`
}

type Schema struct {
	Description string  `json:"description"`
	Params      []Param `json:"params"`
	Returns     string  `json:"returns,omitempty"`
}

type Result struct {
	Output any    `json:"output"`
	Note   string `json:"note,omitempty"`
}

type Tool interface {
	Name() string

	Schema() Schema

	Capability() string

	Run(ctx context.Context, args map[string]any) (Result, error)
}

func MarshalArgs(args map[string]any) string {
	if len(args) == 0 {
		return "{}"
	}
	b, err := json.Marshal(args)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func MarshalResult(r Result) string {
	b, err := json.Marshal(r)
	if err != nil {
		return "{}"
	}
	return string(b)
}
