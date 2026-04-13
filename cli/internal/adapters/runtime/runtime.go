// Package runtime defines the RuntimeAdapter interface for AI runtime integrations.
package runtime

// Config represents the owner's .leo/config.yaml configuration.
type Config struct {
	Version string
	Runtime string
	Owner   OwnerConfig
}

// OwnerConfig holds owner preferences.
type OwnerConfig struct {
	Language       string
	Mode           string
	Autonomy       string
	DefaultProfile string
}

// Profile represents a specialist profile from .leo/profiles/.
type Profile struct {
	Name             string
	Description      string
	Focus            []string
	Tone             string
	DefaultModel     string
	ContextInjection string
}

// Rule represents a KB rule document.
type Rule struct {
	ID   string
	Type string
	Tags []string
	Rule string
}

// HookDef defines a hook to register with the runtime.
type HookDef struct {
	Event   string // e.g. "PostToolUse"
	Matcher string // e.g. "Write"
	Command string
}

// Adapter is the interface that runtime integrations must implement.
// Each runtime (Claude, Cursor, Windsurf, etc.) provides an adapter
// that reads from .leo/ and generates runtime-specific files.
type Adapter interface {
	// Name returns the runtime identifier (e.g. "claude", "cursor").
	Name() string

	// GenerateContextFile generates the runtime's boot file
	// (e.g. CLAUDE.md, .cursorrules) from Leo's config, active profile, and KB rules.
	GenerateContextFile(config Config, profile Profile, rules []Rule) error

	// SupportsHooks returns whether this runtime supports hooks.
	SupportsHooks() bool

	// RegisterHooks registers hooks with the runtime if supported.
	RegisterHooks(hooks []HookDef) error

	// DetectRuntime checks whether this runtime is present in the project.
	DetectRuntime() bool
}
