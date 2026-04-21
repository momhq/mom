// Package config handles reading and writing .mom/config.yaml.
package config

import (
	"fmt"
	"os"
	"path/filepath"

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
	Runtimes      map[string]RuntimeConfig `yaml:"runtimes"`
	User          UserConfig               `yaml:"user"`
	Communication CommunicationConfig      `yaml:"communication"`
	Memory        MemoryConfig             `yaml:"memory"`
	Telemetry     TelemetryConfig          `yaml:"telemetry,omitempty"`
	Bootstrap     BootstrapConfig          `yaml:"bootstrap,omitempty"`
}

// BootstrapConfig holds settings for the cartographer bootstrap pass.
type BootstrapConfig struct {
	// Enabled controls whether bootstrap is offered during init. Default: true.
	Enabled *bool `yaml:"enabled,omitempty"`
	// CommitDepth is how many recent commits to scan. Default: 200.
	CommitDepth int `yaml:"commit_depth,omitempty"`
	// Extensions is the list of text file extensions to scan for markdown extraction.
	Extensions []string `yaml:"extensions,omitempty"`
	// SkipPatterns is a list of glob patterns to exclude from scanning.
	SkipPatterns []string `yaml:"skip_patterns,omitempty"`
	// MaxFileSizeMB skips files larger than this value. Default: 2.
	MaxFileSizeMB int64 `yaml:"max_file_size_mb,omitempty"`
}

// BootstrapEnabled returns true unless Bootstrap.Enabled is explicitly set to false.
func (bc BootstrapConfig) BootstrapEnabled() bool {
	return bc.Enabled == nil || *bc.Enabled
}

// TelemetryConfig holds telemetry settings.
type TelemetryConfig struct {
	// Enabled controls whether events are written to disk. Default: true (nil == enabled).
	Enabled *bool `yaml:"enabled,omitempty"`
	// Path overrides the default telemetry directory (<leoDir>/telemetry/).
	Path string `yaml:"path,omitempty"`
}

// TelemetryEnabled returns true unless Enabled is explicitly set to false.
func (tc TelemetryConfig) TelemetryEnabled() bool {
	return tc.Enabled == nil || *tc.Enabled
}

// RuntimeConfig holds per-runtime settings.
type RuntimeConfig struct {
	Enabled bool `yaml:"enabled"`
	// Tiers was retired in v0.9.0 (#74). The field is intentionally absent from
	// this struct so that go-yaml silently drops it on load. The upgrade command
	// strips any residual tiers: keys from config files on disk.
}

// UserConfig holds user preferences.
type UserConfig struct {
	Language string `yaml:"language"`
	// Autonomy was retired in v0.9.0 (#74). The field is intentionally absent
	// so that go-yaml silently drops it on load. The upgrade command strips any
	// residual autonomy: keys from config files on disk.
}

// CommunicationConfig holds communication style settings.
type CommunicationConfig struct {
	// Mode controls verbosity: concise | normal | verbose | caveman. Default: concise.
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
		Runtimes: map[string]RuntimeConfig{
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

// EnabledRuntimes returns the names of all runtimes where enabled is true.
func (c *Config) EnabledRuntimes() []string {
	var runtimes []string
	for name, rc := range c.Runtimes {
		if rc.Enabled {
			runtimes = append(runtimes, name)
		}
	}
	return runtimes
}

// PrimaryRuntime returns the first enabled runtime name, for backward
// compatibility with code that expects a single runtime.
func (c *Config) PrimaryRuntime() string {
	for name, rc := range c.Runtimes {
		if rc.Enabled {
			return name
		}
	}
	return "claude"
}

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

// Load reads a config.yaml from the given .mom/ directory.
// Handles both v0.6.0 (single runtime) and v0.7.0 (multi-runtime) formats,
// and migrates legacy kb: keys to memory: on load.
func Load(leoDir string) (*Config, error) {
	path := filepath.Join(leoDir, "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	// Try new format first.
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// If Runtimes is populated, it's the new format.
	if len(cfg.Runtimes) > 0 {
		// Back-fill communication.mode if absent (pre-v0.8 configs that had
		// user.mode but no communication block are handled via legacyConfig).
		if cfg.Communication.Mode == "" {
			cfg.Communication.Mode = "concise"
		}
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
	if cfg.Runtimes == nil {
		cfg.Runtimes = Default().Runtimes
	}
	return &cfg, nil
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

	// Infer communication.mode from legacy user.mode.
	// Preserve "caveman" if set; default everything else to "concise".
	commMode := "concise"
	if legacyUser.Mode == "caveman" {
		commMode = "caveman"
	}

	// Autonomy and tiers were retired in v0.9.0 (#74) — not propagated.
	user := UserConfig{
		Language: legacyUser.Language,
	}

	return &Config{
		Version:    legacy.Version,
		CoreSource: legacy.CoreSource,
		Runtimes: map[string]RuntimeConfig{
			rt: {Enabled: true},
		},
		User:          user,
		Communication: CommunicationConfig{Mode: commMode},
		Memory:        legacy.KB,
	}
}

// Save writes a config.yaml to the given .mom/ directory.
func Save(leoDir string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}

	path := filepath.Join(leoDir, "config.yaml")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}

// MomDir returns the .mom/ directory path relative to the given project root.
func MomDir(projectRoot string) string {
	return filepath.Join(projectRoot, ".mom")
}
