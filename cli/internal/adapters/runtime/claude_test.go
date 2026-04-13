package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClaudeAdapter_Name(t *testing.T) {
	a := NewClaudeAdapter("/tmp/test")
	if a.Name() != "claude" {
		t.Errorf("expected %q, got %q", "claude", a.Name())
	}
}

func TestClaudeAdapter_SupportsHooks(t *testing.T) {
	a := NewClaudeAdapter("/tmp/test")
	if !a.SupportsHooks() {
		t.Error("expected SupportsHooks to be true")
	}
}

func TestClaudeAdapter_DetectRuntime(t *testing.T) {
	dir := t.TempDir()

	a := NewClaudeAdapter(dir)
	if a.DetectRuntime() {
		t.Error("expected false when .claude/ does not exist")
	}

	os.MkdirAll(filepath.Join(dir, ".claude"), 0755)
	if !a.DetectRuntime() {
		t.Error("expected true when .claude/ exists")
	}
}

func TestClaudeAdapter_GenerateContextFile(t *testing.T) {
	dir := t.TempDir()
	a := NewClaudeAdapter(dir)

	config := Config{
		Version: "1",
		Runtime: "claude",
		Owner: OwnerConfig{
			Language: "en",
			Mode:     "concise",
			Autonomy: "balanced",
		},
	}

	profile := Profile{
		Name:             "Backend Engineer",
		Description:      "APIs, databases, performance",
		ContextInjection: "Focus on code quality.",
	}

	rules := []Rule{
		{ID: "anti-hallucination", Rule: "When you're not sure, say you don't know."},
	}

	if err := a.GenerateContextFile(config, profile, rules); err != nil {
		t.Fatalf("GenerateContextFile failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, ".claude", "CLAUDE.md"))
	if err != nil {
		t.Fatalf("reading CLAUDE.md: %v", err)
	}

	s := string(content)
	checks := []string{
		"LEO — Living Ecosystem Orchestrator",
		"Backend Engineer",
		"Focus on code quality.",
		"anti-hallucination",
		"Language: en",
	}
	for _, check := range checks {
		if !strings.Contains(s, check) {
			t.Errorf("CLAUDE.md missing %q", check)
		}
	}
}

func TestClaudeAdapter_RegisterHooks(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".claude"), 0755)
	a := NewClaudeAdapter(dir)

	hooks := []HookDef{
		{Event: "PostToolUse", Matcher: "Write", Command: "validate.sh"},
	}

	if err := a.RegisterHooks(hooks); err != nil {
		t.Fatalf("RegisterHooks failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("reading settings.json: %v", err)
	}

	s := string(content)
	if !strings.Contains(s, "PostToolUse") {
		t.Error("settings.json missing hook event")
	}
	if !strings.Contains(s, "validate.sh") {
		t.Error("settings.json missing hook command")
	}
}
