// Tools untuk warga akses brain DB via abstraction — zero-trust, warga ga
// boleh tau schema / column names / tabel mentah. Semua query di-isolate
// lewat Go function yang sanitize + policy-enforce.
//
// Security rationale (Ayah 2026-04-24):
//   "AI ngak boleh tahu sistem memori kita. Jika kelak ada AI jahat, dia
//   bisa merusak. Jadi semua harus pake tools ini demi keamanan warga."
//
// Tools di file ini read-only — tulis pakai memorize_brain.go separate.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	braindb "github.com/teetah2402/flowork/brain/db"
	"github.com/teetah2402/flowork/internal/provider"
)

// ── ListRolesTool ───────────────────────────────────────────────────────
// Warga kepo role apa aja yang ada — boleh, ini public info.

type ListRolesTool struct{ workspace string }

type listRolesArgs struct {
	PublicOnly bool `json:"public_only,omitempty"`
}

func NewListRolesTool(workspace string) *ListRolesTool {
	if workspace == "" {
		workspace, _ = os.Getwd()
	}
	return &ListRolesTool{workspace: workspace}
}

func (t *ListRolesTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "list_roles",
		Description: "Daftar role canonical yang ada di Flowork. Pakai ini buat " +
			"klasifikasi intent user ke role yang tepat. Return: list role + description.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"public_only": map[string]any{
					"type":        "boolean",
					"description": "Hanya role yang public-delegable (pelayan boleh dispatch). Default: false.",
				},
			},
		},
	}
}

func (t *ListRolesTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args listRolesArgs
	// W18 fix: log unmarshal err. Args optional → fallback to public_only=false.
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil && len(invocation.Arguments) > 0 {
		log.Printf("list_roles: arg parse err (using defaults): %v", err)
	}

	db, err := braindb.Shared(t.workspace)
	if err != nil {
		return Result{}, fmt.Errorf("open brain: %w", err)
	}

	query := `SELECT role_id, COALESCE(description,''), public_delegable, sensitive FROM role_registry`
	if args.PublicOnly {
		query += ` WHERE public_delegable = 1`
	}
	query += ` ORDER BY role_id`

	rows, err := db.Query(query)
	if err != nil {
		return Result{}, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()

	var roles []map[string]any
	for rows.Next() {
		var id, desc string
		var pub, sens int
		if err := rows.Scan(&id, &desc, &pub, &sens); err != nil {
			continue
		}
		roles = append(roles, map[string]any{
			"role_id":           id,
			"description":       desc,
			"public_delegable":  pub == 1,
			"sensitive":         sens == 1,
		})
	}
	if err := rows.Err(); err != nil {
		return Result{}, fmt.Errorf("list roles rows: %w", err)
	}

	payload := map[string]any{
		"count": len(roles),
		"roles": roles,
	}
	out, _ := json.MarshalIndent(payload, "", "  ")
	return Result{ToolName: "list_roles", OK: true, Output: string(out)}, nil
}

// ── GetDoktrinTool ─────────────────────────────────────────────────────
// Warga baca doktrin/constitution lewat tool — bukan raw DB. Abstraction
// enforce bahwa warga cuma bisa akses yang "published", bukan sensitive rows.

type GetDoktrinTool struct{ workspace string }

type getDoktrinArgs struct {
	Name string `json:"name" validate:"required"`
}

func NewGetDoktrinTool(workspace string) *GetDoktrinTool {
	if workspace == "" {
		workspace, _ = os.Getwd()
	}
	return &GetDoktrinTool{workspace: workspace}
}

func (t *GetDoktrinTool) Definition() provider.ToolDefinition {
	return provider.ToolDefinition{
		Name: "get_doktrin",
		Description: "Baca satu doktrin Flowork (SOUL, prinsip-kuantum, agents-operasional, dll). " +
			"Pakai ini buat cek aturan sebelum action penting. Return: content doktrin.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Nama doktrin. Contoh: 'soul', 'prinsip-flowork-kuantum', 'agents-operasional', 'plug-and-play'.",
				},
			},
			"required": []string{"name"},
		},
	}
}

func (t *GetDoktrinTool) Execute(ctx context.Context, invocation Invocation) (Result, error) {
	var args getDoktrinArgs
	if err := json.Unmarshal(invocation.Arguments, &args); err != nil {
		return Result{}, fmt.Errorf("decode args: %w", err)
	}
	if err := ValidateRequired(&args); err != nil {
		return Result{}, fmt.Errorf("validation: %w", err)
	}
	name := strings.ToLower(strings.TrimSpace(args.Name))

	// Map friendly name → section LIKE pattern di constitution table.
	// Public whitelist aman untuk semua warga.
	publicSectionMap := map[string]string{
		"soul":                    "Flowork SOUL%",
		"prinsip-flowork-kuantum": "Prinsip Flowork Kuantum%",
		"plug-and-play":           "Flowork Plug-and-Play%",
		"agents-operasional":      "Agents Operasional%",
		"adr-017-standar-agent":   "ADR-017%",
		// 2026-05-08 alias keyword umum yang warga sering search:
		"konstitusi":              "Prinsip Flowork Kuantum%",
		"doktrin":                 "Prinsip Flowork Kuantum%",
		"konstitusi-flowork":      "Prinsip Flowork Kuantum%",
		"prinsip":                 "Prinsip Flowork Kuantum%",
		"fqp":                     "Prinsip Flowork Kuantum%",
		"sop":                     "Agents Operasional%",
		"protokol":                "Agents Operasional%",
		"identity":                "Flowork SOUL%",
	}
	// 2026-05-08: doktrin_documents path REMOVED (A2 cleanup).
	// quality-control/ folder dihapus + table 0 rows + folder source gone.
	// Doktrin sakral (WASIAT/SEJARAH/PASAL 10/SOUL.md/gol.md/TRAINING_DISCIPLINE)
	// sekarang via constitution table amp >= 999998 + auto-inject ke prompt
	// via SacredDoctrines bridge (commit 8b7dd10). Friendly-name lookup
	// publicDoktrinDocuments di-drop — caller pakai brain_search atau
	// constitution lookup langsung.

	// Admin-only — meta-arsitektur, tidak aman untuk non-admin warga.
	adminSectionMap := map[string]string{
		"gol-flowork":                    "Gol Flowork%",
		"adr-009-soul-operasional-split": "ADR-009%",
		"zero-trust-db":                  "Zero-Trust%",
	}

	if _, isAdmin := adminSectionMap[name]; isAdmin {
		return Result{
			ToolName: "get_doktrin",
			OK:       false,
			Output:   fmt.Sprintf("Doktrin %q admin-only (meta-arsitektur). Akses via tool get_doktrin_admin (role=admin/security required).", name),
		}, nil
	}

	sectionPattern, ok := publicSectionMap[name]
	if !ok {
		return Result{
			ToolName: "get_doktrin",
			OK:       false,
			Output:   fmt.Sprintf("Doktrin %q ga terdaftar. Public whitelist constitution: soul, prinsip-flowork-kuantum, plug-and-play, agents-operasional, adr-017-standar-agent, konstitusi, doktrin, prinsip, fqp, sop, protokol, identity. Untuk doktrin sakral (WASIAT/SEJARAH/PASAL 10/TRAINING_DISCIPLINE) auto-inject di prompt via SacredDoctrines amp>=999998 — pakai brain_search kalau butuh keyword query.", name),
		}, nil
	}

	db, err := braindb.Shared(t.workspace)
	if err != nil {
		return Result{}, fmt.Errorf("open brain: %w", err)
	}

	// Canonical source: constitution table (amplitude 999999 = immutable).
	var content string
	err = db.QueryRow(`SELECT content FROM constitution WHERE section LIKE ? ORDER BY id LIMIT 1`, sectionPattern).Scan(&content)
	if err != nil {
		return Result{
			ToolName: "get_doktrin",
			OK:       false,
			Output:   fmt.Sprintf("Doktrin %q belum ada di constitution.", name),
		}, nil
	}

	return Result{ToolName: "get_doktrin", OK: true, Output: content}, nil
}
