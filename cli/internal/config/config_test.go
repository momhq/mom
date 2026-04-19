package config

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestDefault_HasSaneValues(t *testing.T) {
	cfg := Default()

	if cfg.Version != "1" {
		t.Errorf("expected version %q, got %q", "1", cfg.Version)
	}
	if len(cfg.Runtimes) == 0 {
		t.Fatal("expected at least one runtime in defaults")
	}
	rc, ok := cfg.Runtimes["claude"]
	if !ok {
		t.Fatal("expected claude runtime in defaults")
	}
	if !rc.Enabled {
		t.Error("expected claude runtime to be enabled")
	}
	if rc.Tiers["orchestration"] != "opus" {
		t.Errorf("expected orchestration tier %q, got %q", "opus", rc.Tiers["orchestration"])
	}
	if rc.Tiers["execution"] != "sonnet" {
		t.Errorf("expected execution tier %q, got %q", "sonnet", rc.Tiers["execution"])
	}
	if cfg.Communication.Mode != "concise" {
		t.Errorf("expected communication.mode %q, got %q", "concise", cfg.Communication.Mode)
	}
	if !cfg.KB.AutoPropagate {
		t.Error("expected auto_propagate to be true")
	}
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()

	original := Default()
	original.Runtimes["codex"] = RuntimeConfig{
		Enabled: true,
		Tiers:   map[string]string{"orchestration": "o3", "execution": "gpt-4.1", "review": "gpt-4.1-mini"},
	}
	original.User.Language = "pt-BR"

	if err := Save(dir, &original); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists.
	path := filepath.Join(dir, "config.yaml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config.yaml not created: %v", err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if _, ok := loaded.Runtimes["codex"]; !ok {
		t.Error("expected codex runtime after round-trip")
	}
	if loaded.User.Language != "pt-BR" {
		t.Errorf("expected language %q, got %q", "pt-BR", loaded.User.Language)
	}
	if loaded.Version != original.Version {
		t.Errorf("version mismatch: %q vs %q", original.Version, loaded.Version)
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	if _, err := Load("/nonexistent/dir"); err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(":\n  :\n    - :\n  ]["), 0644)

	if _, err := Load(dir); err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLeoDir(t *testing.T) {
	got := LeoDir("/home/user/project")
	expected := filepath.Join("/home/user/project", ".leo")
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestConfigMigrationFromV06(t *testing.T) {
	dir := t.TempDir()
	legacyCfg := `version: "1"
runtime: claude
core_source: /tmp/leo-core
user:
  language: en
  mode: concise
  autonomy: balanced
  default_profile: general-manager
kb:
  auto_propagate: true
  wrap_up: prompt
  stale_threshold: 30d
specialists:
  orchestrator_model: opus
  default_model: sonnet
  simple_task_model: haiku
  validation: always
`
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(legacyCfg), 0644)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	rc, ok := cfg.Runtimes["claude"]
	if !ok {
		t.Fatal("expected claude runtime after migration")
	}
	if !rc.Enabled {
		t.Error("expected claude to be enabled after migration")
	}
	if rc.Tiers["orchestration"] != "opus" {
		t.Errorf("expected orchestration=opus, got %q", rc.Tiers["orchestration"])
	}
	if rc.Tiers["execution"] != "sonnet" {
		t.Errorf("expected execution=sonnet, got %q", rc.Tiers["execution"])
	}
	if cfg.CoreSource != "/tmp/leo-core" {
		t.Errorf("expected core_source preserved, got %q", cfg.CoreSource)
	}
	// communication.mode must be inferred.
	if cfg.Communication.Mode == "" {
		t.Error("expected communication.mode to be inferred from legacy config")
	}
}

// TestLegacyConfigWithDefaultProfile verifies that a v0.7 config carrying
// user.default_profile loads without error and drops the profile field.
func TestLegacyConfigWithDefaultProfile(t *testing.T) {
	dir := t.TempDir()
	legacyCfg := `version: "1"
runtimes:
  claude:
    enabled: true
user:
  language: en
  mode: concise
  autonomy: balanced
  default_profile: cto
kb:
  auto_propagate: true
  wrap_up: prompt
  stale_threshold: 30d
`
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(legacyCfg), 0644)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load failed on legacy config with default_profile: %v", err)
	}

	// communication.mode must be back-filled.
	if cfg.Communication.Mode == "" {
		t.Error("expected communication.mode to be back-filled")
	}
	// Other user settings must be preserved.
	if cfg.User.Language != "en" {
		t.Errorf("expected language=en, got %q", cfg.User.Language)
	}
	if cfg.User.Autonomy != "balanced" {
		t.Errorf("expected autonomy=balanced, got %q", cfg.User.Autonomy)
	}
}

// TestLegacyConfigCavemanModePreserved verifies caveman mode is preserved through migration.
func TestLegacyConfigCavemanModePreserved(t *testing.T) {
	dir := t.TempDir()
	legacyCfg := `version: "1"
runtime: claude
owner:
  language: pt
  mode: caveman
  default_profile: cto
  autonomy: autonomous
kb:
  auto_propagate: true
  wrap_up: prompt
  stale_threshold: 30d
`
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(legacyCfg), 0644)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Communication.Mode != "caveman" {
		t.Errorf("expected caveman mode to be preserved, got %q", cfg.Communication.Mode)
	}
}

func TestConfigNilTiers(t *testing.T) {
	dir := t.TempDir()
	cfgYaml := `version: "1"
runtimes:
  cline:
    enabled: true
user:
  language: en
  mode: concise
  autonomy: balanced
kb:
  auto_propagate: true
  wrap_up: prompt
  stale_threshold: 30d
`
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfgYaml), 0644)

	cfg, err := Load(dir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	rc, ok := cfg.Runtimes["cline"]
	if !ok {
		t.Fatal("expected cline runtime")
	}
	if rc.Tiers != nil {
		t.Errorf("expected nil tiers for cline, got %v", rc.Tiers)
	}
}

func TestConfigEnabledRuntimes(t *testing.T) {
	cfg := Config{
		Runtimes: map[string]RuntimeConfig{
			"claude": {Enabled: true},
			"codex":  {Enabled: true},
			"cline":  {Enabled: false},
		},
	}

	enabled := cfg.EnabledRuntimes()
	sort.Strings(enabled)

	if len(enabled) != 2 {
		t.Fatalf("expected 2 enabled runtimes, got %d", len(enabled))
	}
	if enabled[0] != "claude" || enabled[1] != "codex" {
		t.Errorf("expected [claude, codex], got %v", enabled)
	}
}

func TestTelemetryEnabledDefault(t *testing.T) {
	// Absent Telemetry config (nil Enabled) must default to enabled.
	cfg := Config{}
	if !cfg.Telemetry.TelemetryEnabled() {
		t.Error("expected telemetry to be enabled by default (nil Enabled)")
	}
}

func TestTelemetryExplicitFalse(t *testing.T) {
	f := false
	cfg := Config{Telemetry: TelemetryConfig{Enabled: &f}}
	if cfg.Telemetry.TelemetryEnabled() {
		t.Error("expected telemetry to be disabled when Enabled=false")
	}
}

func TestTelemetryExplicitTrue(t *testing.T) {
	tr := true
	cfg := Config{Telemetry: TelemetryConfig{Enabled: &tr}}
	if !cfg.Telemetry.TelemetryEnabled() {
		t.Error("expected telemetry to be enabled when Enabled=true")
	}
}

func TestTelemetryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	f := false
	cfg := Default()
	cfg.Telemetry = TelemetryConfig{Enabled: &f, Path: "/custom/path"}

	if err := Save(dir, &cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.Telemetry.Enabled == nil || *loaded.Telemetry.Enabled != false {
		t.Error("expected telemetry.enabled=false after round-trip")
	}
	if loaded.Telemetry.Path != "/custom/path" {
		t.Errorf("expected path=/custom/path, got %q", loaded.Telemetry.Path)
	}
}

func TestConfigMultiRuntime(t *testing.T) {
	dir := t.TempDir()

	cfg := Config{
		Version: "1",
		Runtimes: map[string]RuntimeConfig{
			"claude": {Enabled: true, Tiers: map[string]string{"orchestration": "opus", "execution": "sonnet", "review": "sonnet"}},
			"codex":  {Enabled: true, Tiers: map[string]string{"orchestration": "o3", "execution": "gpt-4.1", "review": "gpt-4.1-mini"}},
			"cline":  {Enabled: true},
		},
		User:          UserConfig{Language: "en", Autonomy: "balanced"},
		Communication: CommunicationConfig{Mode: "concise"},
		KB:            KBConfig{AutoPropagate: true, WrapUp: "prompt", StaleThreshold: "30d"},
	}

	if err := Save(dir, &cfg); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load(dir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(loaded.Runtimes) != 3 {
		t.Errorf("expected 3 runtimes, got %d", len(loaded.Runtimes))
	}
	if loaded.Runtimes["codex"].Tiers["orchestration"] != "o3" {
		t.Error("codex orchestration tier not preserved")
	}
}
