package agentmgr

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Path-traversal ids must be refused at the choke points before they become
// filesystem paths (agentdb.Resolve / agentFolder). Locks the fix for the
// "?id=../other" cross-agent / out-of-folder access class.
func TestOpenAgentStore_RejectsTraversal(t *testing.T) {
	for _, id := range []string{"../mr-flow", "../../tmp/pwn", "a/b", "", "..", "MrFlow"} {
		if _, err := openAgentStore(id); err == nil {
			t.Errorf("openAgentStore(%q) accepted a malformed id", id)
		}
	}
}

func TestBuildRouterClient_RejectsTraversal(t *testing.T) {
	for _, id := range []string{"../mr-flow", "../../etc/x", "a/b", ""} {
		if _, err := buildRouterClient(id); err == nil {
			t.Errorf("buildRouterClient(%q) accepted a malformed id", id)
		}
	}
}

// SchedulerTriggerHandler must bind to the VERIFIED caller over the loopback
// self-API: an agent cannot fire ANOTHER agent's schedule via ?id=<other>.
func TestSchedulerTrigger_CallerBinding(t *testing.T) {
	const secret = "test-secret-123"
	t.Setenv("FLOWORK_LOOPBACK_SECRET", secret)

	r := httptest.NewRequest(http.MethodPost, "/api/agents/scheduler/trigger?id=victim-agent&schedule_id=1", nil)
	r.Header.Set("X-Flowork-Secret", secret)
	r.Header.Set("X-Flowork-Caller", "attacker-agent")
	w := httptest.NewRecorder()
	SchedulerTriggerHandler(w, r)

	if body := w.Body.String(); !strings.Contains(body, "identity mismatch") {
		t.Errorf("cross-agent trigger not rejected; body=%s", body)
	}
}

// A matching caller (or no secret) must still pass identity binding and reach the
// engine wiring check — proving the guard does not block legitimate use.
func TestSchedulerTrigger_SameCallerPasses(t *testing.T) {
	const secret = "test-secret-123"
	t.Setenv("FLOWORK_LOOPBACK_SECRET", secret)
	SchedulerFireFunc = nil // ensure deterministic downstream branch

	r := httptest.NewRequest(http.MethodPost, "/api/agents/scheduler/trigger?id=my-agent&schedule_id=1", nil)
	r.Header.Set("X-Flowork-Secret", secret)
	r.Header.Set("X-Flowork-Caller", "my-agent")
	w := httptest.NewRecorder()
	SchedulerTriggerHandler(w, r)

	body := w.Body.String()
	if strings.Contains(body, "identity mismatch") {
		t.Errorf("same-caller request wrongly rejected; body=%s", body)
	}
	if !strings.Contains(body, "not wired") {
		t.Errorf("expected to reach engine-wiring branch, got body=%s", body)
	}
}
