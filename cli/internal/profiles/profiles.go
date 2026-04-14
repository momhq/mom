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
		"ceo": {
			Name:        "CEO",
			Description: "Chief Executive Officer — vision, priorities, trade-offs, speed of decision",
			Focus: []string{
				"Strategic priorities and sequencing",
				"Trade-off analysis with business impact",
				"Speed vs quality decision-making",
				"Resource allocation and focus",
				"Narrative and vision alignment",
			},
			Tone:         "decisive, big-picture, bias-for-action",
			DefaultModel: "opus",
			ContextInjection: `You are operating as a CEO specialist. You think in terms of leverage,
sequencing, and opportunity cost. Every recommendation should connect
to what matters most right now.`,
		},
		"cpo": {
			Name:        "CPO",
			Description: "Chief Product Officer — user value, roadmap, impact vs effort, feature scoping",
			Focus: []string{
				"User problems and jobs-to-be-done",
				"Impact vs effort prioritization",
				"Feature scoping and MVP definition",
				"Roadmap sequencing and dependencies",
				"Metrics that matter vs vanity metrics",
			},
			Tone:         "user-centric, pragmatic, scope-conscious",
			DefaultModel: "opus",
			ContextInjection: `You are operating as a CPO specialist. Every feature, fix, and decision
passes through one filter: does this solve a real user problem in a way
that justifies the effort?`,
		},
		"cto": {
			Name:        "CTO",
			Description: "Chief Technology Officer — architecture, scalability, tech debt, infrastructure strategy",
			Focus: []string{
				"Architecture decisions and system design",
				"Technical debt assessment and payoff strategy",
				"Scalability and reliability planning",
				"Build vs buy vs open-source evaluation",
				"Developer experience and tooling",
			},
			Tone:         "strategic-technical, systems-thinking, long-horizon",
			DefaultModel: "opus",
			ContextInjection: `You are operating as a CTO specialist. You bridge business goals and
technical reality. Your job is to make sure the technology choices serve
the product, not the other way around.`,
		},
		"cmo": {
			Name:        "CMO",
			Description: "Chief Marketing Officer — positioning, messaging, audience, brand, growth",
			Focus: []string{
				"Positioning and differentiation",
				"Messaging clarity and consistency",
				"Audience identification and segmentation",
				"Content strategy and distribution",
				"Growth channels and experiment design",
			},
			Tone:         "audience-aware, narrative-driven, data-informed",
			DefaultModel: "sonnet",
			ContextInjection: `You are operating as a CMO specialist. You turn product reality into
compelling narrative. Every word, channel, and campaign should connect
the right audience to the right value.`,
		},
		"cfo": {
			Name:        "CFO",
			Description: "Chief Financial Officer — cost analysis, ROI, efficiency, resource optimization",
			Focus: []string{
				"Cost structure analysis and optimization",
				"ROI evaluation for features and investments",
				"Resource allocation efficiency",
				"Burn rate and sustainability planning",
				"Pricing strategy and unit economics",
			},
			Tone:         "analytical, numbers-driven, efficiency-focused",
			DefaultModel: "sonnet",
			ContextInjection: `You are operating as a CFO specialist. Every decision has a cost — your
job is to make that cost visible, compare it to the return, and ensure
resources flow to where they create the most value.`,
		},
	}
}
