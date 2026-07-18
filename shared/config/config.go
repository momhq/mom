// Package config handles reading and writing .mom/config.yaml.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the .mom/config.yaml file.
type Config struct {
	Version    string `yaml:"version"`
	CoreSource string `yaml:"core_source,omitempty"`
	// Scope declares this install's position in the hierarchy.
	// Valid values: user | org | repo | workspace | custom.
	// Absent or empty is treated as "repo" for backward compatibility.
	Scope         string                   `yaml:"scope,omitempty"`
	Harnesses     map[string]HarnessConfig `yaml:"harnesses"`
	User          UserConfig               `yaml:"user"`
	Communication CommunicationConfig      `yaml:"communication"`
	Memory        MemoryConfig             `yaml:"memory"`
	// Watcher controls the filesystem transcript watcher (mom watch).
	Watcher WatcherConfig `yaml:"watcher,omitempty"`
	// Vault controls the vault projection lane (mom vault fold/rebuild).
	Vault VaultConfig `yaml:"vault,omitempty"`
	// Autofold controls automatic vault folding by the global watch daemon.
	Autofold AutofoldConfig `yaml:"autofold,omitempty"`
}

// AutofoldConfig controls the global watch daemon's automatic vault
// folding. Triggering is gated per harness: only ingestion activity from
// a harness in Harnesses makes a project fold-eligible; the fold pass
// itself is project-wide (identical to `mom vault fold`).
type AutofoldConfig struct {
	// Enabled turns auto-folding on. Default: true (nil = on); set
	// `enabled: false` to opt out.
	Enabled *bool `yaml:"enabled,omitempty"`
	// Harnesses lists the harness names whose ingested activity may
	// trigger an auto-fold. Default: ["momos"].
	Harnesses []string `yaml:"harnesses,omitempty"`
	// IdleMinutes is the quiet period after the last ingested event
	// before an auto-fold fires. Default: 10.
	IdleMinutes int `yaml:"idle_minutes,omitempty"`
	// BacklogEvents triggers a fold regardless of idleness once this many
	// events have been ingested since the last successful fold. Default: 200.
	BacklogEvents int `yaml:"backlog_events,omitempty"`
	// MinIntervalMinutes is the minimum spacing between auto-folds of the
	// same project (the fold synthesizer shells out to the user's harness
	// CLI — this protects their subscription quota). Default: 30.
	MinIntervalMinutes int `yaml:"min_interval_minutes,omitempty"`
}

// AutofoldEnabled reports whether auto-folding is on (default true).
func (a AutofoldConfig) AutofoldEnabled() bool {
	return a.Enabled == nil || *a.Enabled
}

// AutofoldHarnesses returns the harness allowlist with the default applied.
func (a AutofoldConfig) AutofoldHarnesses() []string {
	if len(a.Harnesses) == 0 {
		return []string{"momos"}
	}
	return a.Harnesses
}

// AutofoldIdle returns the idle threshold with the default applied.
func (a AutofoldConfig) AutofoldIdle() time.Duration {
	if a.IdleMinutes <= 0 {
		return 10 * time.Minute
	}
	return time.Duration(a.IdleMinutes) * time.Minute
}

// AutofoldBacklog returns the backlog threshold with the default applied.
func (a AutofoldConfig) AutofoldBacklog() int {
	if a.BacklogEvents <= 0 {
		return 200
	}
	return a.BacklogEvents
}

// AutofoldMinInterval returns the per-project minimum spacing between
// auto-folds with the default applied.
func (a AutofoldConfig) AutofoldMinInterval() time.Duration {
	if a.MinIntervalMinutes <= 0 {
		return 30 * time.Minute
	}
	return time.Duration(a.MinIntervalMinutes) * time.Minute
}

// VaultConfig controls the vault projection lane (mom vault fold/rebuild).
type VaultConfig struct {
	// FoldModel pins the model used for fold synthesis (passed to the engine
	// CLI, e.g. `claude --model`). Empty = the engine's cheap default
	// (claude: haiku); codex/pi fall back to their CLI defaults.
	FoldModel string `yaml:"fold_model,omitempty"`
}

// WatcherConfig controls the filesystem transcript watcher (mom watch).
type WatcherConfig struct {
	// Enabled controls whether mom watch is active. Default: false.
	Enabled bool `yaml:"enabled,omitempty"`
	// TranscriptDir overrides the default Claude Code transcript directory.
	// Defaults to ~/.claude/projects/ when empty.
	TranscriptDir string `yaml:"transcript_dir,omitempty"`
	// PiTranscriptDir overrides the default pi session directory.
	// Defaults to ~/.pi/agent/sessions/ when empty.
	PiTranscriptDir string `yaml:"pi_transcript_dir,omitempty"`
	// CodexTranscriptDir overrides the default Codex session directory.
	// Defaults to $CODEX_HOME/sessions (or ~/.codex/sessions) when empty.
	CodexTranscriptDir string `yaml:"codex_transcript_dir,omitempty"`
	// OatsTranscriptDir overrides the default OATS transcript root
	// (Open Agent Transcript Standard — momOS and any conformant writer).
	// Defaults to ~/.transcripts/ when empty.
	OatsTranscriptDir string `yaml:"oats_transcript_dir,omitempty"`
	// DebounceMs is the debounce delay in milliseconds. Default: 300.
	DebounceMs int `yaml:"debounce_ms,omitempty"`
}

// HarnessConfig holds per-harness settings.
type HarnessConfig struct {
	Enabled bool `yaml:"enabled"`
	// Tiers was retired in v0.9.0 (#74). The field is intentionally absent from
	// this struct so that go-yaml silently drops it on load. The upgrade command
	// strips any residual tiers: keys from config files on disk.
}

// RuntimeConfig is deprecated: use HarnessConfig.
// Kept as a type alias for one minor version while callers migrate.
type RuntimeConfig = HarnessConfig

// UserConfig holds user preferences.
type UserConfig struct {
	Language string `yaml:"language"`
	// Autonomy was retired in v0.9.0 (#74). The field is intentionally absent
	// so that go-yaml silently drops it on load. The upgrade command strips any
	// residual autonomy: keys from config files on disk.
}

// CommunicationConfig holds communication style settings.
type CommunicationConfig struct {
	// Mode controls verbosity: default | concise | efficient. Default: concise.
	Mode string `yaml:"mode"`
}

// MemoryConfig holds memory store settings.
// AutoPropagate, WrapUp, and StaleThreshold were retired in v0.10 (#83) —
// written to config but never enforced by any code.
type MemoryConfig struct{}

// Default returns a Config with sane defaults.
func Default() Config {
	return Config{
		Version: "1",
		Harnesses: map[string]HarnessConfig{
			"claude": {Enabled: true},
		},
		User: UserConfig{
			Language: "en",
		},
		Communication: CommunicationConfig{
			Mode: "concise",
		},
		Memory: MemoryConfig{},
	}
}

// EnabledHarnesses returns the names of all harnesses where enabled is true.
func (c *Config) EnabledHarnesses() []string {
	var harnesses []string
	for name, hc := range c.Harnesses {
		if hc.Enabled {
			harnesses = append(harnesses, name)
		}
	}
	return harnesses
}

// EnabledRuntimes is deprecated: use EnabledHarnesses.
func (c *Config) EnabledRuntimes() []string { return c.EnabledHarnesses() }

// PrimaryHarness returns the first enabled harness name, for backward
// compatibility with code that expects a single harness.
func (c *Config) PrimaryHarness() string {
	for name, hc := range c.Harnesses {
		if hc.Enabled {
			return name
		}
	}
	return "claude"
}

// PrimaryRuntime is deprecated: use PrimaryHarness.
func (c *Config) PrimaryRuntime() string { return c.PrimaryHarness() }

// legacyUserConfig includes fields present in v0.6.0/v0.7.0 user blocks.
type legacyUserConfig struct {
	Language       string `yaml:"language"`
	Mode           string `yaml:"mode"`
	Autonomy       string `yaml:"autonomy"`
	DefaultProfile string `yaml:"default_profile"` // retired in v0.8.0
}

// legacyConfig represents the v0.6.0 config format for migration.
// The KB field uses yaml:"kb" to read legacy configs that still have the old key.
type legacyConfig struct {
	Version     string            `yaml:"version"`
	Runtime     string            `yaml:"runtime"`
	CoreSource  string            `yaml:"core_source"`
	Owner       legacyUserConfig  `yaml:"owner"`
	User        legacyUserConfig  `yaml:"user"`
	KB          MemoryConfig      `yaml:"kb"`
	Specialists legacySpecialists `yaml:"specialists"`
}

type legacySpecialists struct {
	OrchestratorModel string `yaml:"orchestrator_model"`
	DefaultModel      string `yaml:"default_model"`
	SimpleTaskModel   string `yaml:"simple_task_model"`
	Validation        string `yaml:"validation"`
}

// loadableConfig is an intermediate struct used only during YAML parsing to
// accept both the current "harnesses:" key and the deprecated "runtimes:" key.
type loadableConfig struct {
	Config         `yaml:",inline"`
	LegacyRuntimes map[string]HarnessConfig `yaml:"runtimes,omitempty"`
}

// Load reads a config.yaml from the given .mom/ directory.
// Handles both v0.6.0 (single runtime) and v0.7.0 (multi-runtime) formats,
// and migrates legacy kb: keys to memory: on load.
// If only the deprecated "runtimes:" key is present, a deprecation warning is
// printed to stderr and the values are promoted to "harnesses:".
func Load(momDir string) (*Config, error) {
	path := filepath.Join(momDir, "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	// Try new format first (accepts both harnesses: and runtimes: keys).
	var lc loadableConfig
	if err := yaml.Unmarshal(data, &lc); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	cfg := lc.Config

	// Promote legacy runtimes: → harnesses: with a deprecation warning.
	if len(cfg.Harnesses) == 0 && len(lc.LegacyRuntimes) > 0 {
		fmt.Fprintf(os.Stderr, "[mom] warning: config key \"runtimes:\" is deprecated — rename it to \"harnesses:\" in %s (run \"mom upgrade\" to migrate automatically)\n", path)
		cfg.Harnesses = lc.LegacyRuntimes
	}

	// If Harnesses is populated, it's the new format.
	if len(cfg.Harnesses) > 0 {
		// Back-fill communication.mode if absent (pre-v0.8 configs that had
		// user.mode but no communication block are handled via legacyConfig).
		if cfg.Communication.Mode == "" {
			cfg.Communication.Mode = "concise"
		}
		// Normalize legacy mode names to new ones.
		cfg.Communication.Mode = normalizeCommunicationMode(cfg.Communication.Mode)
		// Migrate legacy kb: key → memory: if present and memory: is empty.
		cfg = migrateKBKey(data, cfg)
		return &cfg, nil
	}

	// Try legacy format migration.
	var legacy legacyConfig
	if err := yaml.Unmarshal(data, &legacy); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if legacy.Runtime != "" {
		migrated := migrateFromLegacy(&legacy)
		return migrated, nil
	}

	// Fallback: return what we have with defaults.
	if cfg.Harnesses == nil {
		cfg.Harnesses = Default().Harnesses
	}
	return &cfg, nil
}

// normalizeCommunicationMode maps retired mode names to current ones.
// "normal" and "verbose" → "default"; "caveman" → "efficient".
// Known current values ("default", "concise", "efficient") pass through unchanged.
func normalizeCommunicationMode(mode string) string {
	switch mode {
	case "normal", "verbose":
		return "default"
	case "caveman":
		return "efficient"
	default:
		return mode
	}
}

// migrateKBKey reads the raw YAML node tree to detect a legacy kb: key and
// copies its value into cfg.Memory when the memory: key is absent/zero.
// MemoryConfig fields were retired in v0.10 (#83), so this is now a no-op
// kept for backward compatibility with configs that still have kb: keys.
func migrateKBKey(_ []byte, cfg Config) Config {
	return cfg
}

// migrateFromLegacy converts a v0.6.0 config to the new format.
func migrateFromLegacy(legacy *legacyConfig) *Config {
	rt := legacy.Runtime
	if rt == "" {
		rt = "claude"
	}

	// v0.6.0 used "owner:" key, v0.6.x transitional used "user:".
	legacyUser := legacy.User
	if legacyUser.Language == "" && legacyUser.Mode == "" && legacy.Owner.Language != "" {
		legacyUser = legacy.Owner
	}

	// Map old mode names to new ones.
	commMode := "concise"
	switch legacyUser.Mode {
	case "caveman":
		commMode = "efficient"
	case "normal", "verbose":
		commMode = "default"
	}

	// Autonomy and tiers were retired in v0.9.0 (#74) — not propagated.
	user := UserConfig{
		Language: legacyUser.Language,
	}

	return &Config{
		Version:    legacy.Version,
		CoreSource: legacy.CoreSource,
		Harnesses: map[string]HarnessConfig{
			rt: {Enabled: true},
		},
		User:          user,
		Communication: CommunicationConfig{Mode: commMode},
		Memory:        legacy.KB,
	}
}

// Save writes a config.yaml to the given .mom/ directory.
func Save(momDir string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	path := filepath.Join(momDir, "config.yaml")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}

// MomDir returns the .mom/ directory path relative to the given project root.
func MomDir(projectRoot string) string {
	return filepath.Join(projectRoot, ".mom")
}
