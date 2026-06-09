package harness

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed capabilities/codex.yaml
var codexCapabilitiesYAML []byte

// CodexAdapter implements the Adapter interface for OpenAI Codex.
// It reads from .mom/ and generates AGENTS.md at the project root.
type CodexAdapter struct {
	projectRoot string
}

// NewCodexAdapter creates a CodexAdapter for the given project root.
func NewCodexAdapter(projectRoot string) *CodexAdapter {
	return &CodexAdapter{projectRoot: projectRoot}
}

func (a *CodexAdapter) Name() string {
	return "codex"
}

func (a *CodexAdapter) Tier() Tier {
	return Fluent
}

// DefaultTranscriptDir returns Codex's session transcript directory.
// Honors $CODEX_HOME when set (per Codex docs); otherwise falls back to
// ~/.codex/sessions.
func (a *CodexAdapter) DefaultTranscriptDir() string {
	if home := os.Getenv("CODEX_HOME"); home != "" {
		return filepath.Join(home, "sessions")
	}
	return "~/.codex/sessions"
}

func (a *CodexAdapter) GenerateContextFile(config Config, constraints []Constraint, skills []Skill, identity *Identity) error {
	body := BuildContextContent(config, constraints, skills, identity)
	content := a.Watermark() + "\n\n" + body

	agentsFile := filepath.Join(a.projectRoot, "AGENTS.md")
	if err := os.WriteFile(agentsFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing AGENTS.md: %w", err)
	}

	return nil
}

func (a *CodexAdapter) RegisterHooks() error {
	if err := writeCodexHooks(filepath.Join(a.projectRoot, ".codex", "hooks.json")); err != nil {
		return err
	}
	return writeCodexFeatures(filepath.Join(a.projectRoot, ".codex", "config.toml"))
}

// RegisterGlobalHooks writes the same hook contract to the user-level
// Codex config dir (~/.codex/hooks.json, or $CODEX_HOME/hooks.json when
// set) so Codex Desktop sessions fire `mom watch --sweep` after each
// Cascade response — same defensive sweep wired for project-local
// installs, scoped to the user.
func (a *CodexAdapter) RegisterGlobalHooks() error {
	hooksPath, err := codexHomePath("hooks.json")
	if err != nil {
		return err
	}
	if err := writeCodexHooks(hooksPath); err != nil {
		return err
	}
	cfgPath, err := codexHomePath("config.toml")
	if err != nil {
		return err
	}
	return writeCodexFeatures(cfgPath)
}

// writeCodexHooks renders Codex's hooks.json format at the given path,
// creating parent dirs as needed. The hook set is intentionally small:
// one Stop hook running the resolved MOM binary. Auxiliary signal — the
// daemon's fsnotify watcher catches new transcripts even when this
// hook never fires.
func writeCodexHooks(hooksPath string) error {
	hooks := []HookDef{
		{Event: "Stop", Command: "mom watch --sweep --global"},
	}
	if err := os.MkdirAll(filepath.Dir(hooksPath), 0755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(hooksPath), err)
	}

	// Codex hooks.json format: { "hooks": { "Event": [ { "hooks": [ {...} ] } ] } }
	byEvent := make(map[string][]map[string]any)
	for _, h := range hooks {
		entry := map[string]any{
			"type":    "command",
			"command": h.Command,
			"timeout": 10,
		}
		group := map[string]any{
			"hooks": []map[string]any{entry},
		}
		if h.Matcher != "" {
			group["matcher"] = h.Matcher
		}
		byEvent[h.Event] = append(byEvent[h.Event], group)
	}

	root := map[string]any{"hooks": byEvent}
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling hooks: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(hooksPath, data, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", hooksPath, err)
	}
	return nil
}

// codexFeaturesBlock enables Codex hooks. Codex deprecated the old
// `codex_hooks` flag in favour of `hooks`.
const codexFeaturesBlock = `
[features]
hooks = true
`

// writeCodexFeatures ensures [features].hooks = true exists in a Codex
// config.toml so Codex honors the hooks.json contract. Idempotent and
// tolerant of older MOM writes that left duplicate [features] tables, the
// deprecated codex_hooks key, or a now-retired [mcp_servers.mom] block.
func writeCodexFeatures(path string) error {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", filepath.Base(path), err)
	}

	content := string(existing)
	// Strip any [mcp_servers.mom] block left by a pre-v0.50 install — MOM no
	// longer ships an MCP server.
	content = stripCodexMCPBlock(content)
	content = normalizeCodexFeaturesBlock(content)

	if content == string(existing) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", filepath.Base(path), err)
	}
	return nil
}

// stripCodexMCPBlock removes a [mcp_servers.mom] section (and any nested
// [mcp_servers.mom.*] tables) from a Codex config.toml. The block ends at the
// next top-level [section] header. Used to clean up pre-v0.50 MCP writes.
func stripCodexMCPBlock(content string) string {
	if !strings.Contains(content, "[mcp_servers.mom]") {
		return content
	}
	lines := strings.Split(content, "\n")
	var out []string
	inBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		isHeader := strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")
		if trimmed == "[mcp_servers.mom]" {
			inBlock = true
			continue
		}
		if inBlock {
			if isHeader && !strings.HasPrefix(trimmed, "[mcp_servers.mom.") {
				inBlock = false
				out = append(out, line)
				continue
			}
			// drop the block's key/value lines and nested mom.* tables.
			continue
		}
		out = append(out, line)
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n") + "\n"
}

func normalizeCodexFeaturesBlock(content string) string {
	lines := strings.Split(content, "\n")
	var out []string
	inFeatures := false
	skipDuplicateFeatures := false
	featuresSeen := false
	hooksSeen := false
	changed := false

	flushFeatures := func() {
		if inFeatures && !hooksSeen {
			out = append(out, "hooks = true")
			changed = true
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		isHeader := strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")
		if isHeader {
			flushFeatures()
			inFeatures = trimmed == "[features]"
			skipDuplicateFeatures = false
			hooksSeen = false
			if inFeatures {
				if featuresSeen {
					inFeatures = false
					skipDuplicateFeatures = true
					changed = true
					continue
				}
				featuresSeen = true
			}
			out = append(out, line)
			continue
		}

		if skipDuplicateFeatures {
			changed = true
			continue
		}

		if inFeatures {
			if strings.HasPrefix(trimmed, "codex_hooks") {
				if !hooksSeen {
					out = append(out, "hooks = true")
					hooksSeen = true
				}
				changed = true
				continue
			}
			if strings.HasPrefix(trimmed, "hooks") {
				if hooksSeen {
					changed = true
					continue
				}
				out = append(out, "hooks = true")
				hooksSeen = true
				if trimmed != "hooks = true" {
					changed = true
				}
				continue
			}
		}
		out = append(out, line)
	}
	flushFeatures()

	if !featuresSeen {
		trimmed := strings.TrimRight(strings.Join(out, "\n"), "\n")
		if trimmed != "" {
			trimmed += "\n"
		}
		return trimmed + strings.TrimLeft(codexFeaturesBlock, "\n")
	}

	result := strings.TrimRight(strings.Join(out, "\n"), "\n") + "\n"
	if !changed && result == content {
		return content
	}
	return result
}

func (a *CodexAdapter) DetectHarness() bool {
	if commandExists("codex") {
		return true
	}
	if home := os.Getenv("CODEX_HOME"); home != "" && pathExists(home) {
		return true
	}
	if path, err := homePath(".codex"); err == nil && pathExists(path) {
		return true
	}
	return false
}

func (a *CodexAdapter) GenerateGlobalContextFile(config Config, constraints []Constraint, skills []Skill, identity *Identity) error {
	path, err := codexHomePath("AGENTS.md")
	if err != nil {
		return err
	}
	return upsertManagedBlock(path, buildGlobalContext(a.Watermark(), config, constraints, skills, identity))
}

func (a *CodexAdapter) GeneratedFiles() []string {
	return []string{
		"AGENTS.md",
		filepath.Join(".codex", "hooks.json"),
		filepath.Join(".codex", "config.toml"),
	}
}

func (a *CodexAdapter) GeneratedDirs() []string {
	return []string{".codex"}
}

func (a *CodexAdapter) Watermark() string {
	return "<!-- Generated by MOM — do not edit manually -->"
}

func (a *CodexAdapter) Capabilities() AdapterCapability {
	var cap AdapterCapability
	if err := yaml.Unmarshal(codexCapabilitiesYAML, &cap); err != nil {
		return AdapterCapability{Name: "codex", Version: "0.1"}
	}
	return cap
}

func codexHomePath(parts ...string) (string, error) {
	home := os.Getenv("CODEX_HOME")
	if home == "" {
		var err error
		home, err = homePath(".codex")
		if err != nil {
			return "", err
		}
	}
	items := append([]string{home}, parts...)
	return filepath.Join(items...), nil
}

var (
	_ GlobalAdapter       = (*CodexAdapter)(nil)
	_ HookInstaller       = (*CodexAdapter)(nil)
	_ GlobalHookInstaller = (*CodexAdapter)(nil)
	_ TranscriptSource    = (*CodexAdapter)(nil)
)
