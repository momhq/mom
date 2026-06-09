package harness

import (
	"strings"
	"testing"
)

// TestBuildContextContent_VaultFirst verifies the context block delivers the
// vault-first, MCP-free session-start protocol.
func TestBuildContextContent_VaultFirst(t *testing.T) {
	cfg := Config{
		Version: "1",
		User: UserConfig{
			Language:          "en",
			Autonomy:          "balanced",
			CommunicationMode: "concise",
		},
	}

	content := BuildContextContent(cfg, nil, nil, nil)

	for _, want := range []string{
		"MOM — Memory Oriented Machine",
		"mom project",
		".mom/vault/INDEX.md",
		"mom vault fold",
		"/mom-status",
		"/mom-project",
		"/mom-fold",
		"/mom-rebuild",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("context content must mention %q", want)
		}
	}

	// MCP and the retired JSON-memory model must be gone.
	forbidden := []string{
		"mom_status", "mom_recall", "mom_get", "mom_landmarks",
		"MCP", "mcp", "## Voice", "## Memory", ".mom/memory/", "schema.json",
		"mom recall", "mom record",
	}
	for _, f := range forbidden {
		if strings.Contains(content, f) {
			t.Errorf("context content must not contain %q", f)
		}
	}
}

// TestBuildContextContent_KeepsUserDirectives verifies language and
// communication-mode directives still render (they are orthogonal to memory
// delivery).
func TestBuildContextContent_KeepsUserDirectives(t *testing.T) {
	cfg := Config{
		Version: "1",
		User: UserConfig{
			Language:          "es",
			CommunicationMode: "concise",
		},
	}

	content := BuildContextContent(cfg, nil, nil, nil)
	if !strings.Contains(content, LanguageInstructions("es")) {
		t.Error("context content missing language directive")
	}
}
