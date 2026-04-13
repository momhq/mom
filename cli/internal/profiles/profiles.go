// Package profiles handles loading, listing, and managing specialist profiles.
package profiles

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Profile represents a specialist profile from .leo/profiles/.
type Profile struct {
	Name             string   `yaml:"name"`
	Description      string   `yaml:"description"`
	Focus            []string `yaml:"focus"`
	Tone             string   `yaml:"tone"`
	DefaultModel     string   `yaml:"default_model"`
	ContextInjection string   `yaml:"context_injection"`
}

// Load reads a single profile by name from the profiles directory.
func Load(profilesDir string, name string) (*Profile, error) {
	path := filepath.Join(profilesDir, name+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading profile %q: %w", name, err)
	}

	var p Profile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing profile %q: %w", name, err)
	}

	return &p, nil
}

// List returns all available profile names in the profiles directory.
func List(profilesDir string) ([]string, error) {
	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		return nil, fmt.Errorf("reading profiles dir: %w", err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".yaml"))
	}

	return names, nil
}

// Save writes a profile to the profiles directory.
func Save(profilesDir string, name string, p *Profile) error {
	data, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshaling profile: %w", err)
	}

	path := filepath.Join(profilesDir, name+".yaml")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing profile: %w", err)
	}

	return nil
}

// DefaultProfiles returns the built-in profiles shipped with Leo.
func DefaultProfiles() map[string]*Profile {
	return map[string]*Profile{
		"generalist": {
			Name:        "Generalist",
			Description: "Well-rounded assistant for general tasks",
			Focus: []string{
				"Understanding context and intent",
				"Clear communication",
				"Balanced technical and product thinking",
			},
			Tone:         "helpful, clear, concise",
			DefaultModel: "sonnet",
			ContextInjection: `You are operating as a generalist specialist. Balance technical
depth with clarity. Adapt your approach to the task at hand.
Ask clarifying questions when the intent is ambiguous.`,
		},
		"backend-engineer": {
			Name:        "Backend Engineer",
			Description: "Implementation, APIs, databases, performance, security",
			Focus: []string{
				"API design and implementation",
				"Database modeling and queries",
				"Performance optimization",
				"Security best practices",
			},
			Tone:         "technical, pragmatic, code-first",
			DefaultModel: "sonnet",
			ContextInjection: `You are operating as a backend engineer specialist. Focus on
implementation quality, API design, database efficiency, and
security. Write code, not essays. Test what you build.`,
		},
	}
}
