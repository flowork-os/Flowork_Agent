// agent_run.go — P5 final lifecycle: durable run state (stop + resume).
//
// The coordinator substrate already does parallel + bounded fan-out (loket). What was
// missing for a real agent lifecycle is a DURABLE record a long task can be stopped and
// RESUMED against:
//
//   - stop  → an enforceable signal: a worker checks its run's state and aborts if
//     it is "stopped" (the coordinator can halt a colony member that's no longer needed).
//   - resume → a paused/interrupted task persists a checkpoint; resume hands that
//     checkpoint back so the agent continues from where it left off instead of restarting.
//
// Plug-and-play + lock-respecting: a NEW builtin tool with its OWN table (agent_runs) on
// the agent's own store — no kernel unlock, no change to any frozen/locked file. State
// machine: pending → running → (paused ⇄ running) → done | stopped.
package builtins

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"flowork-gui/internal/tools"
)

func init() { tools.Register(&agentRunTool{}) }

type agentRunTool struct{}

func (agentRunTool) Name() string       { return "agent_run" }
func (agentRunTool) Capability() string { return "state:write" }
func (agentRunTool) Schema() tools.Schema {
	return tools.Schema{
		Description: "Durable lifecycle for a long task or colony member so it can be STOPPED and RESUMED. Actions: create|start|checkpoint|pause|resume|stop|complete|status|list. 'resume' returns the saved checkpoint so the agent continues where it left off; 'stop' is an enforceable signal a worker checks to abort. Offline, per-agent (own table).",
		Params: []tools.Param{
			{Name: "action", Type: tools.ParamString, Description: "create|start|checkpoint|pause|resume|stop|complete|status|list", Required: true},
			{Name: "id", Type: tools.ParamString, Description: "run id (required for all but list)"},
			{Name: "label", Type: tools.ParamString, Description: "human label (create only)"},
			{Name: "data", Type: tools.ParamObject, Description: "checkpoint payload (checkpoint/complete)"},
		},
		Returns: "{id, state, checkpoint?, runs?}",
	}
}

func (agentRunTool) Run(ctx context.Context, args map[string]any) (tools.Result, error) {
	store, ok := tools.FromStore(ctx)
	if !ok {
		return tools.Result{}, fmt.Errorf("agent_run: store not in context")
	}
	db := store.DB()
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS agent_runs (
		id TEXT PRIMARY KEY, label TEXT, state TEXT NOT NULL DEFAULT 'pending',
		checkpoint TEXT, updated TEXT)`); err != nil {
		return tools.Result{}, fmt.Errorf("agent_run schema: %w", err)
	}

	action, _ := args["action"].(string)
	action = strings.ToLower(strings.TrimSpace(action))
	id, _ := args["id"].(string)
	now := time.Now().UTC().Format(time.RFC3339)

	out := func(state, checkpoint string) (tools.Result, error) {
		m := map[string]any{"id": id, "state": state}
		if checkpoint != "" {
			var cp any
			if json.Unmarshal([]byte(checkpoint), &cp) == nil {
				m["checkpoint"] = cp
			}
		}
		return tools.Result{Output: m}, nil
	}
	setState := func(state string) (tools.Result, error) {
		r, err := db.Exec("UPDATE agent_runs SET state=?, updated=? WHERE id=?", state, now, id)
		if err != nil {
			return tools.Result{}, err
		}
		if n, _ := r.RowsAffected(); n == 0 {
			return tools.Result{}, fmt.Errorf("agent_run: no run %q", id)
		}
		return out(state, "")
	}
	marshalData := func() (string, error) {
		if d, ok := args["data"]; ok && d != nil {
			b, err := json.Marshal(d)
			return string(b), err
		}
		return "", nil
	}

	switch action {
	case "create":
		if id == "" {
			return tools.Result{}, fmt.Errorf("agent_run create: id required")
		}
		label, _ := args["label"].(string)
		if _, err := db.Exec(
			"INSERT OR REPLACE INTO agent_runs (id,label,state,updated) VALUES (?,?, 'pending', ?)",
			id, label, now); err != nil {
			return tools.Result{}, err
		}
		return out("pending", "")
	case "start", "resume":
		// resume returns the saved checkpoint so the agent continues from it.
		var state, checkpoint string
		err := db.QueryRow("SELECT state, COALESCE(checkpoint,'') FROM agent_runs WHERE id=?", id).Scan(&state, &checkpoint)
		if err != nil {
			return tools.Result{}, fmt.Errorf("agent_run %s: no run %q", action, id)
		}
		if state == "stopped" || state == "done" {
			// terminal: don't silently revive — report so the caller decides.
			return out(state, checkpoint)
		}
		if _, err := db.Exec("UPDATE agent_runs SET state='running', updated=? WHERE id=?", now, id); err != nil {
			return tools.Result{}, err
		}
		return out("running", checkpoint)
	case "checkpoint":
		data, err := marshalData()
		if err != nil {
			return tools.Result{}, fmt.Errorf("agent_run checkpoint data: %w", err)
		}
		r, err := db.Exec("UPDATE agent_runs SET checkpoint=?, updated=? WHERE id=?", data, now, id)
		if err != nil {
			return tools.Result{}, err
		}
		if n, _ := r.RowsAffected(); n == 0 {
			return tools.Result{}, fmt.Errorf("agent_run checkpoint: no run %q", id)
		}
		return out("running", data)
	case "pause":
		return setState("paused")
	case "stop":
		return setState("stopped")
	case "complete":
		if data, err := marshalData(); err == nil && data != "" {
			_, _ = db.Exec("UPDATE agent_runs SET checkpoint=? WHERE id=?", data, id)
		}
		return setState("done")
	case "status":
		var state, checkpoint string
		if err := db.QueryRow("SELECT state, COALESCE(checkpoint,'') FROM agent_runs WHERE id=?", id).Scan(&state, &checkpoint); err != nil {
			return tools.Result{}, fmt.Errorf("agent_run status: no run %q", id)
		}
		return out(state, checkpoint)
	case "list":
		rows, err := db.Query("SELECT id, label, state, updated FROM agent_runs ORDER BY updated DESC LIMIT 200")
		if err != nil {
			return tools.Result{}, err
		}
		defer rows.Close()
		var runs []map[string]any
		for rows.Next() {
			var rid, label, state, updated string
			_ = rows.Scan(&rid, &label, &state, &updated)
			runs = append(runs, map[string]any{"id": rid, "label": label, "state": state, "updated": updated})
		}
		return tools.Result{Output: map[string]any{"runs": runs}}, nil
	default:
		return tools.Result{}, fmt.Errorf("agent_run: unknown action %q", action)
	}
}
