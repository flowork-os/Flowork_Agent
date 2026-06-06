package loket

import "testing"

// TestCatalogIntegrity guards the frozen vocabulary: no duplicate names, every
// entry indexed, names are non-empty dotted tokens.
func TestCatalogIntegrity(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range Catalog {
		if c.Name == "" {
			t.Fatal("capability with empty name")
		}
		if seen[c.Name] {
			t.Errorf("duplicate capability in Catalog: %q", c.Name)
		}
		seen[c.Name] = true
		if got, ok := LookupCap(c.Name); !ok || got.Name != c.Name {
			t.Errorf("LookupCap(%q) failed", c.Name)
		}
	}
	if _, ok := LookupCap("does.not.exist"); ok {
		t.Error("LookupCap returned ok for an unknown capability")
	}
}

// TestTierGatedCapsAreShared documents that the only tier-gated caps are the 5M
// shared brain — the S1 rule, now enforced at the contract level.
func TestTierGatedCapsAreShared(t *testing.T) {
	for _, c := range Catalog {
		if c.Grant == GrantTier {
			if c.Name != "brain.shared.search" && c.Name != "brain.shared.promote" {
				t.Errorf("unexpected tier-gated cap %q (only shared brain should be tier-gated)", c.Name)
			}
		}
	}
}

func validAgentManifest() []byte {
	return []byte(`{
		"id": "title-writer",
		"kind": "agent",
		"name": "Title Writer",
		"version": "1.0.0",
		"abi_version": "1",
		"entry": "handle",
		"tier": "extension",
		"consumes": ["llm.complete", "store.brain.search", "bus.send"]
	}`)
}

func TestParseManifestValid(t *testing.T) {
	m, err := ParseManifest(validAgentManifest())
	if err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
	if m.ID != "title-writer" || m.Kind != KindAgent {
		t.Errorf("parsed manifest wrong: %+v", m)
	}
}

func TestParseManifestRejects(t *testing.T) {
	cases := map[string]string{
		"bad id":   `{"id":"Bad_ID","kind":"agent","name":"X","entry":"handle"}`,
		"bad kind": `{"id":"x-mod","kind":"wizard","name":"X","entry":"handle"}`,
		"no name":  `{"id":"x-mod","kind":"agent","name":"","entry":"handle"}`,
		"no entry": `{"id":"x-mod","kind":"agent","name":"X","entry":""}`,
		"unknown cap": `{"id":"x-mod","kind":"agent","name":"X","entry":"handle",
			"consumes":["llm.complete","fly.to.moon"]}`,
		"bad abi": `{"id":"x-mod","kind":"agent","name":"X","entry":"handle","abi_version":"99"}`,
		// S1 architectural rule: an extension cannot consume the 5M shared brain.
		"extension wants shared brain": `{"id":"x-mod","kind":"agent","name":"X","entry":"handle",
			"tier":"extension","consumes":["brain.shared.search"]}`,
		"group without members": `{"id":"x-grp","kind":"group","name":"X","entry":"handle"}`,
	}
	for name, raw := range cases {
		if _, err := ParseManifest([]byte(raw)); err == nil {
			t.Errorf("[%s] expected rejection, got none", name)
		}
	}
}

// TestPrimaryMayConsumeSharedBrain — the same shared-brain cap is allowed when
// the module is primary tier.
func TestPrimaryMayConsumeSharedBrain(t *testing.T) {
	raw := `{"id":"mr-flow","kind":"agent","name":"Mr Flow","entry":"handle",
		"tier":"primary","consumes":["brain.shared.search"]}`
	if _, err := ParseManifest([]byte(raw)); err != nil {
		t.Errorf("primary consuming shared brain rejected: %v", err)
	}
}
