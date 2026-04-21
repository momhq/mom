// Package runtime defines the RuntimeAdapter interface for AI runtime integrations.
package runtime

// Config represents the user's .mom/config.yaml configuration.
type Config struct {
	Version string
	User    UserConfig
	HasMCP  bool
}

// UserConfig holds user preferences.
type UserConfig struct {
	Language          string
	Autonomy          string
	CommunicationMode string
}

// Constraint represents a memory constraint document.
type Constraint struct {
	ID      string
	Summary string
	Tags    []string
}

// Skill represents a memory skill document.
type Skill struct {
	ID      string
	Summary string
	Tags    []string
}

// Identity represents the .mom/identity.json file.
type Identity struct {
	What        string
	Philosophy  string
	Constraints []string
}

// AdapterCapability describes which MRP v0 events an adapter natively supports.
// Loaded from the adapter's embedded YAML capability file.
type AdapterCapability struct {
	// Name is the adapter identifier (matches Name()).
	Name string `yaml:"adapter"`
	// Version is the adapter version string.
	Version string `yaml:"version"`
	// Supports lists MRP events natively supported by this adapter.
	Supports []string `yaml:"supports"`
	// Experimental lists MRP events emitted best-effort — may fire unreliably.
	Experimental []string `yaml:"experimental"`
}

// HookDef defines a hook to register with the runtime.
type HookDef struct {
	Event   string // e.g. "PostToolUse"
	Matcher string // e.g. "Write"
	Command string
}

// Adapter is the interface that runtime integrations must implement.
// Each runtime (Claude, Codex, Cline, etc.) provides an adapter
// that reads from .mom/ and generates runtime-specific files.
type Adapter interface {
	// Name returns the runtime identifier (e.g. "claude", "codex", "cline").
	Name() string

	// GenerateContextFile generates the runtime's boot file
	// (e.g. CLAUDE.md, AGENTS.md, .clinerules/leo-context.md) from Leo's config,
	// constraints, skills, and identity.
	GenerateContextFile(config Config, constraints []Constraint, skills []Skill, identity *Identity) error

	// SupportsHooks returns whether this runtime supports hooks.
	SupportsHooks() bool

	// RegisterHooks registers hooks with the runtime if supported.
	RegisterHooks(hooks []HookDef) error

	// DetectRuntime checks whether this runtime is present in the project.
	DetectRuntime() bool

	// GeneratedFiles returns the list of file paths (relative to project root)
	// that this adapter generates. Used by uninstall to clean up.
	GeneratedFiles() []string

	// GeneratedDirs returns directories (relative to project root) that this
	// adapter creates and that can be removed if empty after file cleanup.
	GeneratedDirs() []string

	// Watermark returns the header comment inserted into generated files.
	// Used to distinguish Leo-generated files from user-created ones.
	Watermark() string

	// Capabilities returns the MRP v0 capability declaration for this adapter.
	// Loaded from the embedded YAML file in capabilities/.
	Capabilities() AdapterCapability
}
