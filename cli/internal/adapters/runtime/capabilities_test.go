package runtime

import (
	"testing"
)

// TestClaudeAdapter_Capabilities verifies the claude adapter loads its
// embedded YAML and reports full MRP v0 support with no experimental events.
func TestClaudeAdapter_Capabilities(t *testing.T) {
	a := NewClaudeAdapter("/tmp/test")
	cap := a.Capabilities()

	if cap.Name != "claude-code" {
		t.Errorf("expected adapter name %q, got %q", "claude-code", cap.Name)
	}
	if cap.Version == "" {
		t.Error("expected non-empty version")
	}

	wantSupports := []string{
		"session.start",
		"session.end",
		"turn.complete",
		"compact.triggered",
		"clear.triggered",
	}
	for _, event := range wantSupports {
		if !containsString(cap.Supports, event) {
			t.Errorf("claude Supports missing %q", event)
		}
	}

	if len(cap.Experimental) != 0 {
		t.Errorf("claude Experimental should be empty, got %v", cap.Experimental)
	}
}

// TestCodexAdapter_Capabilities verifies the codex adapter loads its YAML
// and correctly marks compact/clear as experimental.
func TestCodexAdapter_Capabilities(t *testing.T) {
	a := NewCodexAdapter("/tmp/test")
	cap := a.Capabilities()

	if cap.Name != "codex" {
		t.Errorf("expected adapter name %q, got %q", "codex", cap.Name)
	}

	wantSupports := []string{"session.start", "session.end"}
	for _, event := range wantSupports {
		if !containsString(cap.Supports, event) {
			t.Errorf("codex Supports missing %q", event)
		}
	}

	// turn.complete must NOT be in Supports.
	if containsString(cap.Supports, "turn.complete") {
		t.Error("codex must not support turn.complete natively")
	}

	wantExperimental := []string{"compact.triggered", "clear.triggered"}
	for _, event := range wantExperimental {
		if !containsString(cap.Experimental, event) {
			t.Errorf("codex Experimental missing %q", event)
		}
	}
}

// TestAdapterCapability_NoOverlap verifies that no event appears in both
// Supports and Experimental for any adapter.
func TestAdapterCapability_NoOverlap(t *testing.T) {
	adapters := []Adapter{
		NewClaudeAdapter("/tmp/test"),
		NewCodexAdapter("/tmp/test"),
		NewWindsurfAdapter("/tmp/test"),
	}
	for _, a := range adapters {
		cap := a.Capabilities()
		supportsSet := make(map[string]bool, len(cap.Supports))
		for _, e := range cap.Supports {
			supportsSet[e] = true
		}
		for _, e := range cap.Experimental {
			if supportsSet[e] {
				t.Errorf("adapter %q: event %q appears in both Supports and Experimental", a.Name(), e)
			}
		}
	}
}

// containsString is a small helper to avoid importing slices for Go <1.21 compat.
func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
