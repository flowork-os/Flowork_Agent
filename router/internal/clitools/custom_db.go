// custom_db.go — CLI tool custom dari DB → daftar ke registry (RegisterCLITool) saat boot,
// jadi masuk All()/DetectAll(). User tambah/hapus dari GUI (handler /api/cli-tools/custom).
// NON-frozen, deletable.
//
// Dev: Aola Sahidin — github.com/flowork-os/Flowork-OS · floworkos.com
package clitools

import (
	"encoding/json"

	"github.com/flowork-os/flowork_Router/internal/store"
)

// LoadCustomCLITools — baca definisi custom dari DB jadi []Tool.
func LoadCustomCLITools() []Tool {
	d, err := store.Open()
	if err != nil {
		return nil
	}
	rawList, err := store.ListCustomCLITools(d)
	if err != nil {
		return nil
	}
	var out []Tool
	for _, raw := range rawList {
		var t Tool
		if json.Unmarshal([]byte(raw), &t) == nil && t.ID != "" {
			out = append(out, t)
		}
	}
	return out
}

// RegisterCustomCLITool — simpan ke DB + daftar live ke registry (muncul langsung di All()).
func RegisterCustomCLITool(t Tool) error {
	d, err := store.Open()
	if err != nil {
		return err
	}
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	if err := store.UpsertCustomCLITool(d, t.ID, string(b)); err != nil {
		return err
	}
	RegisterCLITool(t)
	return nil
}

// DeleteCustomCLITool — hapus dari DB (drop penuh dari registry saat restart).
func DeleteCustomCLITool(id string) error {
	d, err := store.Open()
	if err != nil {
		return err
	}
	return store.DeleteCustomCLITool(d, id)
}

func init() {
	for _, t := range LoadCustomCLITools() {
		RegisterCLITool(t)
	}
}
