// === LOCKED FILE ===
// Status: STABLE — DO NOT MODIFY without owner approval.
// Owner: Aola Sahidin (Mr.Dev)
// Repo: https://github.com/flowork-os/flowork-ai-agent
// Locked at: 2026-06-12
// Reason: ToggleAgent — the reusable core of ToggleHandler, so a group on/off
//   (groupsapi) can disable/enable a coordinator AND every member at once.
//
// toggle.go — programmatic enable/disable for one agent.
package agentmgr

import (
	"errors"

	"flowork-gui/internal/agentdb"
)

// ToggleAgent enables/disables one agent programmatically: persist the disabled
// flag in its store, then reload so the kernel UNLOADS it (disabled) or LOADS it
// (enabled). A disabled agent's instance leaves the runtime, so neither the
// scheduler/triggers (InvokeAgentMessage) nor the loket bus (invokeLoketModule) can
// reach it — it simply never runs and never receives a command.
func ToggleAgent(id string, disabled bool) error {
	if !reID.MatchString(id) {
		return errors.New("invalid id")
	}
	dir, ok := resolveAgentDir(id)
	if !ok {
		return errors.New("agent not found")
	}
	store, err := agentdb.Open(agentdb.Resolve(id, dir))
	if err != nil {
		return err
	}
	if err := store.SetDisabled(disabled); err != nil {
		store.Close()
		return err
	}
	store.Close()
	if Reload != nil {
		return Reload(id)
	}
	return nil
}
