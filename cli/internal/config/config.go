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
	Version     string            `yaml:"version"`
	Runtime     string            `yaml:"runtime"`
	CoreSource  string            `yaml:"core_source,omitempty"`
	Owner       OwnerConfig       `yaml:"owner"`
	KB          KBConfig          `yaml:"kb"`
	Specialists SpecialistsConfig `yaml:"specialists"`
}

// OwnerConfig holds owner preferences.
type OwnerConfig struct {
	Language       string `yaml:"language"`
	Mode           string `yaml:"mode"`
	Autonomy       string `yaml:"autonomy"`
	DefaultProfile string `yaml:"default_profile"`
}

// KBConfig holds KB settings.
type KBConfig struct {
	AutoPropagate  bool   `yaml:"auto_propagate"`
	WrapUp         string `yaml:"wrap_up"`
	StaleThreshold string `yaml:"stale_threshold"`
}

// SpecialistsConfig holds specialist delegation settings.
type SpecialistsConfig struct {
	DefaultModel    string `yaml:"default_model"`
	SimpleTaskModel string `yaml:"simple_task_model"`
	Validation      string `yaml:"validation"`
}

// Default returns a Config with sane defaults.
func Default() Config {
	return Config{
		Version: "1",
		Runtime: "claude",
		Owner: OwnerConfig{
			Language:       "en",
			Mode:           "concise",
			Autonomy:       "balanced",
			DefaultProfile: "generalist",
		},
		KB: KBConfig{
			AutoPropagate:  true,
			WrapUp:         "prompt",
			StaleThreshold: "30d",
		},
		Specialists: SpecialistsConfig{
			DefaultModel:    "sonnet",
			SimpleTaskModel: "haiku",
			Validation:      "always",
		},
	}
}

// Load reads a config.yaml from the given .leo/ directory.
func Load(leoDir string) (*Config, error) {
	path := filepath.Join(leoDir, "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	return &cfg, nil
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
