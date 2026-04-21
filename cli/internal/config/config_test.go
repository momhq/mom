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
	if cfg.Communication.Mode != "concise" {
		t.Errorf("expected communication.mode %q, got %q", "concise", cfg.Communication.Mode)
	}
}

func TestSaveAndLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()

	original := Default()
	original.Runtimes["codex"] = RuntimeConfig{Enabled: true}
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

func TestMomDir(t *testing.T) {
	got := MomDir("/home/user/project")
	expected := filepath.Join("/home/user/project", ".mom")
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestConfigMigrationFromV06(t *testing.T) {
	dir := t.TempDir()
	legacyCfg := `version: "1"
runtime: claude
core_source: /tmp/mom
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
	if cfg.CoreSource != "/tmp/mom" {
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

// TestConfigLegacyFieldsDropped verifies that configs with legacy tiers/autonomy
// fields in YAML are loaded without error and the fields are silently dropped.
func TestConfigLegacyFieldsDropped(t *testing.T) {
	dir := t.TempDir()
	// This YAML still has the retired fields — they must be silently ignored.
	cfgYaml := `version: "1"
runtimes:
  cline:
    enabled: true
    tiers:
      orchestration: opus
      execution: sonnet
user:
  language: en
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

	_, ok := cfg.Runtimes["cline"]
	if !ok {
		t.Fatal("expected cline runtime")
	}
	// Verify the config loaded correctly — the struct no longer has Tiers/Autonomy
	// fields so go-yaml silently drops them. Verify other fields are intact.
	if cfg.User.Language != "en" {
		t.Errorf("expected language=en, got %q", cfg.User.Language)
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
			"claude": {Enabled: true},
			"codex":  {Enabled: true},
			"cline":  {Enabled: true},
		},
		User:          UserConfig{Language: "en"},
		Communication: CommunicationConfig{Mode: "concise"},
		Memory:        MemoryConfig{},
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
}
