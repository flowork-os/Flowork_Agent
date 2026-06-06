package agentmgr

import "testing"

// TestReconcileMaskedSecrets proves the config-Save round-trip can't clobber a real
// secret with the ••••<last4> mask the GET handed the GUI, while still letting a
// genuinely-edited value through and dropping a mask that has no stored original.
func TestReconcileMaskedSecrets(t *testing.T) {
	existing := map[string]string{
		"TELEGRAM_TOKEN": "123456:realToken",
		"OPENAI_KEY":     "sk-realkey-xyz",
	}
	incoming := map[string]any{
		"TELEGRAM_TOKEN": maskSecretValue("123456:realToken"), // untouched → mask posted back
		"OPENAI_KEY":     "sk-NEW-edited-value",               // edited → plaintext
		"GHOST_KEY":      maskSecretValue("whatever"),         // mask but no stored original
	}

	reconcileMaskedSecrets(incoming, existing)

	if got := incoming["TELEGRAM_TOKEN"]; got != "123456:realToken" {
		t.Fatalf("untouched secret clobbered: got %q, want real plaintext restored", got)
	}
	if got := incoming["OPENAI_KEY"]; got != "sk-NEW-edited-value" {
		t.Fatalf("edited secret altered: got %q", got)
	}
	if _, present := incoming["GHOST_KEY"]; present {
		t.Fatalf("a mask with no stored original must be dropped, not persisted")
	}
}
