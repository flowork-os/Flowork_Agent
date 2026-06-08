// brain_dream.go — P3: dream consolidation for the local brain (lock-respecting).
//
// "Sleep consolidates the day": memories that are never recalled should FADE, while
// reinforced ones stay strong. brain_dream gently DECAYS the importance of drawers
// with amplitude 0 (never recalled), so over time unused memories sink in ranking.
//
// SAFE + plug-and-play: it touches only the `importance` column via an UPDATE — it
// NEVER deletes and NEVER touches the FTS index (which mirrors content, not
// importance), so no risk of the brain_fts desync that a raw delete would cause. That
// is why this needs NO unlock of the locked brain storage (agentdb/brain_drawers.go):
// a new tool + a column UPDATE is enough. Exact-duplicate consolidation is already
// handled at write time (AddBrainDrawer dedups by content_hash), so dream only forgets.
package builtins

import (
	"context"
	"fmt"
	"strings"

	"flowork-gui/internal/tools"
)

func init() { tools.Register(&brainDreamTool{}) }

type brainDreamTool struct{}

func (brainDreamTool) Name() string       { return "brain_dream" }
func (brainDreamTool) Capability() string { return "state:write" }
func (brainDreamTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Consolidate this agent's local brain like sleep: gently DECAY the importance of memories never recalled (amplitude 0) so unused ones fade and reinforced ones stay strong. Safe + offline (importance-only UPDATE; never deletes, never touches the search index). Returns how many decayed.",
		Params: []tools.Param{
			{Name: "factor", Type: tools.ParamFloat, Description: "decay multiplier 0.5..0.99 (default 0.9)"},
		},
		Returns: "{decayed, floor, factor}",
	}
}

func (brainDreamTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok {
		return tools.Result{}, fmt.Errorf("brain_dream: store not in context")
	}
	factor := 0.9
	if f, ok := args["factor"].(float64); ok && f >= 0.5 && f <= 0.99 {
		factor = f
	}
	const floor = 1.0
	res, err := store.DB().Exec(
		"UPDATE brain_drawers SET importance = MAX(?, importance * ?) WHERE amplitude = 0 AND importance > ?",
		floor, factor, floor)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			// fresh agent, no brain yet → nothing to consolidate
			return tools.Result{Output: map[string]any{"decayed": int64(0), "floor": floor, "factor": factor}}, nil
		}
		return tools.Result{}, fmt.Errorf("brain_dream decay: %w", err)
	}
	n, _ := res.RowsAffected()
	return tools.Result{Output: map[string]any{"decayed": n, "floor": floor, "factor": factor}}, nil
}
