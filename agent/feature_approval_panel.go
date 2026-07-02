// feature_approval_panel.go — SIBLING feature (⚠️ FROZEN 2026-07-02 seizin owner — stabil+live): endpoint
// AGREGAT antrian approval PENDING lintas-agent → GUI panel (tab Autonomy) cukup 1
// call (bukan iterate per-agent dari browser). Pola sama feature_approval_notify
// (AgentIDs + OpenAgentStore + skip no-table). Approve/reject tetep lewat endpoint
// per-agent frozen yg udah ada. 📄 Dok: lock/approval-gate.md
package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

func init() {
	RegisterFeature(Feature{Name: "approval-panel", Phase: PhaseRoute, Apply: func(d *Deps) {
		if d.Mux == nil || d.Host == nil {
			return
		}
		d.Mux.HandleFunc("/api/agents/protector/approval/pending-all", func(w http.ResponseWriter, r *http.Request) {
			type item struct {
				Agent       string `json:"agent"`
				ID          int64  `json:"id"`
				ToolName    string `json:"tool_name"`
				ArgsJSON    string `json:"args_json"`
				Reason      string `json:"reason"`
				Caller      string `json:"caller"`
				RequestedAt string `json:"requested_at"`
			}
			items := []item{}
			for _, id := range d.Host.AgentIDs() {
				store, err := d.Host.OpenAgentStore(id)
				if err != nil {
					continue
				}
				// Anti polusi DB: skip agent yg belum pernah punya antrian (pola wakeup_engine).
				var tbl string
				if store.DB().QueryRow(
					"SELECT name FROM sqlite_master WHERE type='table' AND name='approval_queue'").
					Scan(&tbl) != nil {
					store.Close()
					continue
				}
				rows, lerr := store.ListApprovalQueue("pending", 50)
				store.Close()
				if lerr != nil {
					continue
				}
				for _, rr := range rows {
					items = append(items, item{
						Agent: id, ID: rr.ID, ToolName: rr.ToolName, ArgsJSON: rr.ArgsJSON,
						Reason: strings.TrimSpace(rr.Reason), Caller: rr.Caller, RequestedAt: rr.RequestedAt,
					})
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"items": items, "count": len(items)})
		})
	}})
}
