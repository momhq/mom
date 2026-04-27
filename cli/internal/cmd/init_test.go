package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/momhq/mom/cli/internal/config"
)

func TestInitCmd_CreatesLeoStructure(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"init", "--runtimes", "claude"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Verify .mom/ structure.
	expected := []string{
		".mom/config.yaml",
		".mom/identity.json",
		".mom/schema.json",
		".mom/logs",
		".claude/CLAUDE.md",
		".mcp.json",
		".mom/constraints/anti-hallucination.json",
		".mom/skills/session-wrap-up.json",
	}
	// Retired files must NOT exist.
	retired := []string{
		".mom/profiles/general-manager.yaml",
		".mom/profiles/backend-engineer.yaml",
		".mom/constraints/delegation-mandatory.json",
		".mom/skills/task-intake.json",
		".mom/kb",
	}
	for _, path := range retired {
		full := filepath.Join(dir, path)
		if _, err := os.Stat(full); err == nil {
			t.Errorf("retired file should not exist: %s", path)
		}
	}

	for _, path := range expected {
		full := filepath.Join(dir, path)
		if _, err := os.Stat(full); err != nil {
			t.Errorf("missing expected file: %s", path)
		}
	}

	// Verify directories.
	dirs := []string{".mom/memory", ".mom/skills", ".mom/constraints", ".mom/logs", ".mom/cache"}
	for _, d := range dirs {
		full := filepath.Join(dir, d)
		info, err := os.Stat(full)
		if err != nil {
			t.Errorf("missing expected dir: %s", d)
		} else if !info.IsDir() {
			t.Errorf("expected %s to be a directory", d)
		}
	}
}

func TestInitCmd_SkipsScaffoldIfAlreadyExists(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	os.MkdirAll(filepath.Join(dir, ".mom"), 0755)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"init", "--runtimes", "claude"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("expected graceful skip when .mom/ already exists, got error: %v", err)
	}
	if !strings.Contains(buf.String(), "already exists") {
		t.Errorf("expected skip message in output, got: %s", buf.String())
	}
}

func TestInitCmd_ForceOverwrite(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	os.MkdirAll(filepath.Join(dir, ".mom"), 0755)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"init", "--runtimes", "claude", "--force"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init --force failed: %v", err)
	}

	// Should have created the structure despite existing .mom/.
	if _, err := os.Stat(filepath.Join(dir, ".mom", "config.yaml")); err != nil {
		t.Error("config.yaml not created with --force")
	}
}

func TestInitCmd_MultiRuntime(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"init", "--runtimes", "claude,codex"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Both runtime outputs should exist.
	files := map[string]string{
		".claude/CLAUDE.md": "Claude",
		"AGENTS.md":         "Codex",
	}

	for path, name := range files {
		full := filepath.Join(dir, path)
		if _, err := os.Stat(full); err != nil {
			t.Errorf("missing %s output: %s", name, path)
		}
	}

	// Config should have both runtimes.
	cfg, err := config.Load(filepath.Join(dir, ".mom"))
	if err != nil {
		t.Fatalf("loading config: %v", err)
	}

	enabled := cfg.EnabledRuntimes()
	if len(enabled) != 2 {
		t.Errorf("expected 2 enabled runtimes, got %d: %v", len(enabled), enabled)
	}
}

// Experimental warnings were removed from init output in v0.12 — too noisy for onboarding.

// TestInitCmd_DefaultDeliversMinimalContent verifies that init with default config
// generates minimal MCP-first boot content (not the legacy full content).
func TestInitCmd_DefaultDeliversMinimalContent(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"init", "--runtimes", "claude"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, ".claude", "CLAUDE.md"))
	if err != nil {
		t.Fatalf("reading CLAUDE.md: %v", err)
	}

	s := string(content)

	// Must contain the MCP-first directive.
	if !strings.Contains(s, "mom_status") {
		t.Error("CLAUDE.md must contain mom_status for MCP-first delivery")
	}

	// Must NOT contain the verbose legacy sections.
	legacy := []string{"## Voice", "## Constraints", "## Skills", "## During work"}
	for _, section := range legacy {
		if strings.Contains(s, section) {
			t.Errorf("CLAUDE.md must not contain legacy section %q with default (mcp) delivery", section)
		}
	}
}

func TestInitCmd_BackupExistingFile(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Create a user-owned AGENTS.md
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# My custom agents"), 0644)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"init", "--runtimes", "codex"})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Original should be backed up
	bkpContent, err := os.ReadFile(filepath.Join(dir, "AGENTS.md.bkp"))
	if err != nil {
		t.Fatal("backup file not created")
	}
	if string(bkpContent) != "# My custom agents" {
		t.Error("backup content doesn't match original")
	}
}

// TestInitCmd_InheritsConstraintsFromParent verifies that when a parent scope
// already has constraints/ and skills/, child init skips creating local copies.
func TestInitCmd_InheritsConstraintsFromParent(t *testing.T) {
	// Create org/repo directory structure.
	orgDir := t.TempDir()
	repoDir := filepath.Join(orgDir, "repo-a")
	os.MkdirAll(repoDir, 0755)

	// Initialize the org scope using runInitWithConfig directly to avoid
	// cobra global state issues when tests run in parallel/sequence.
	cmd := &cobra.Command{}
	cmd.SetOut(new(bytes.Buffer))

	orgResult := OnboardingResult{
		Runtimes:   []string{"claude"},
		Language:   "en",
		Mode:       "concise",
		InstallDir: orgDir,
		ScopeLabel: "org",
	}
	if err := runInitWithConfig(cmd, orgDir, false, orgResult); err != nil {
		t.Fatalf("org init failed: %v", err)
	}

	// Verify org has constraints.
	orgConstraints := filepath.Join(orgDir, ".mom", "constraints")
	entries, err := os.ReadDir(orgConstraints)
	if err != nil || len(entries) == 0 {
		t.Fatal("org scope should have constraint files")
	}

	// Now init the child repo — should inherit constraints from org.
	repoResult := OnboardingResult{
		Runtimes:   []string{"claude"},
		Language:   "en",
		Mode:       "concise",
		InstallDir: repoDir,
		ScopeLabel: "repo",
	}
	if err := runInitWithConfig(cmd, repoDir, false, repoResult); err != nil {
		t.Fatalf("repo init failed: %v", err)
	}

	// Child constraints dir should exist but be empty (no duplicated files).
	repoConstraints := filepath.Join(repoDir, ".mom", "constraints")
	repoEntries, err := os.ReadDir(repoConstraints)
	if err != nil {
		t.Fatalf("repo constraints dir should exist: %v", err)
	}
	if len(repoEntries) > 0 {
		t.Errorf("repo constraints should be empty (inherited from org), got %d files", len(repoEntries))
	}

	// Child skills dir should also be empty.
	repoSkills := filepath.Join(repoDir, ".mom", "skills")
	repoSkillEntries, err := os.ReadDir(repoSkills)
	if err != nil {
		t.Fatalf("repo skills dir should exist: %v", err)
	}
	if len(repoSkillEntries) > 0 {
		t.Errorf("repo skills should be empty (inherited from org), got %d files", len(repoSkillEntries))
	}
}

// TestInitCmd_CreatesConstraintsWhenNoParent verifies that a standalone repo
// (no parent scope) still gets local constraints and skills.
func TestInitCmd_CreatesConstraintsWhenNoParent(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"init", "--runtimes", "claude"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Standalone repo should have constraints written locally.
	constraintsDir := filepath.Join(dir, ".mom", "constraints")
	entries, err := os.ReadDir(constraintsDir)
	if err != nil {
		t.Fatalf("constraints dir should exist: %v", err)
	}
	if len(entries) == 0 {
		t.Error("standalone repo should have local constraint files")
	}

	// And skills too.
	skillsDir := filepath.Join(dir, ".mom", "skills")
	skillEntries, err := os.ReadDir(skillsDir)
	if err != nil {
		t.Fatalf("skills dir should exist: %v", err)
	}
	if len(skillEntries) == 0 {
		t.Error("standalone repo should have local skill files")
	}
}

// TestParentScopeHasDir_Unit tests the parentScopeHasDir helper directly.
func TestParentScopeHasDir_Unit(t *testing.T) {
	// Setup: org/.mom/constraints/ with a file, repo under org.
	orgDir := t.TempDir()
	constraintsDir := filepath.Join(orgDir, ".mom", "constraints")
	os.MkdirAll(constraintsDir, 0755)
	os.WriteFile(filepath.Join(constraintsDir, "test.json"), []byte(`{}`), 0644)

	repoDir := filepath.Join(orgDir, "repo-a")
	os.MkdirAll(repoDir, 0755)

	// From repo dir, parent should have constraints.
	if !parentScopeHasDir(repoDir, "constraints") {
		t.Error("expected parentScopeHasDir to find constraints in parent")
	}

	// From repo dir, parent should NOT have skills (none created).
	if parentScopeHasDir(repoDir, "skills") {
		t.Error("expected parentScopeHasDir to not find skills in parent")
	}

	// From org dir itself, no parent has constraints.
	if parentScopeHasDir(orgDir, "constraints") {
		t.Error("expected parentScopeHasDir to not find constraints above org")
	}
}
