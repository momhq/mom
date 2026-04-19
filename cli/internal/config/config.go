// Package config handles reading and writing .leo/config.yaml.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the .leo/config.yaml file.
type Config struct {
	Version       string                   `yaml:"version"`
	CoreSource    string                   `yaml:"core_source,omitempty"`
	// Scope declares this install's position in the hierarchy.
	// Valid values: user | org | repo | workspace | custom.
	// Absent or empty is treated as "repo" for backward compatibility.
	Scope         string                   `yaml:"scope,omitempty"`
	Runtimes      map[string]RuntimeConfig `yaml:"runtimes"`
	User          UserConfig               `yaml:"user"`
	Communication CommunicationConfig      `yaml:"communication"`
	KB            KBConfig                 `yaml:"kb"`
}

// RuntimeConfig holds per-runtime settings.
type RuntimeConfig struct {
	Enabled bool              `yaml:"enabled"`
	Tiers   map[string]string `yaml:"tiers,omitempty"`
}

// UserConfig holds user preferences.
type UserConfig struct {
	Language string `yaml:"language"`
	Autonomy string `yaml:"autonomy"`
}

// CommunicationConfig holds communication style settings.
type CommunicationConfig struct {
	// Mode controls verbosity: concise | normal | verbose | caveman. Default: concise.
	Mode string `yaml:"mode"`
}

// KBConfig holds KB settings.
type KBConfig struct {
	AutoPropagate  bool   `yaml:"auto_propagate"`
	WrapUp         string `yaml:"wrap_up"`
	StaleThreshold string `yaml:"stale_threshold"`
}

// Default returns a Config with sane defaults.
func Default() Config {
	return Config{
		Version: "1",
		Runtimes: map[string]RuntimeConfig{
			"claude": {
				Enabled: true,
				Tiers: map[string]string{
					"orchestration": "opus",
					"execution":     "sonnet",
					"review":        "sonnet",
				},
			},
		},
		User: UserConfig{
			Language: "en",
			Autonomy: "balanced",
		},
		Communication: CommunicationConfig{
			Mode: "concise",
		},
		KB: KBConfig{
			AutoPropagate:  true,
			WrapUp:         "prompt",
			StaleThreshold: "30d",
		},
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
type legacyConfig struct {
	Version     string            `yaml:"version"`
	Runtime     string            `yaml:"runtime"`
	CoreSource  string            `yaml:"core_source"`
	Owner       legacyUserConfig  `yaml:"owner"`
	User        legacyUserConfig  `yaml:"user"`
	KB          KBConfig          `yaml:"kb"`
	Specialists legacySpecialists `yaml:"specialists"`
}

type legacySpecialists struct {
	OrchestratorModel string `yaml:"orchestrator_model"`
	DefaultModel      string `yaml:"default_model"`
	SimpleTaskModel   string `yaml:"simple_task_model"`
	Validation        string `yaml:"validation"`
}

// Load reads a config.yaml from the given .leo/ directory.
// Handles both v0.6.0 (single runtime) and v0.7.0 (multi-runtime) formats.
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

// migrateFromLegacy converts a v0.6.0 config to the new format.
func migrateFromLegacy(legacy *legacyConfig) *Config {
	tiers := map[string]string{
		"orchestration": "opus",
		"execution":     "sonnet",
		"review":        "sonnet",
	}

	// Map old specialist fields to tiers if they exist.
	if legacy.Specialists.OrchestratorModel != "" {
		tiers["orchestration"] = legacy.Specialists.OrchestratorModel
	}
	if legacy.Specialists.DefaultModel != "" {
		tiers["execution"] = legacy.Specialists.DefaultModel
		tiers["review"] = legacy.Specialists.DefaultModel
	}

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

	user := UserConfig{
		Language: legacyUser.Language,
		Autonomy: legacyUser.Autonomy,
	}

	return &Config{
		Version:    legacy.Version,
		CoreSource: legacy.CoreSource,
		Runtimes: map[string]RuntimeConfig{
			rt: {
				Enabled: true,
				Tiers:   tiers,
			},
		},
		User:          user,
		Communication: CommunicationConfig{Mode: commMode},
		KB:            legacy.KB,
	}
}

// Save writes a config.yaml to the given .leo/ directory.
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

// LeoDir returns the .leo/ directory path relative to the given project root.
func LeoDir(projectRoot string) string {
	return filepath.Join(projectRoot, ".leo")
}
