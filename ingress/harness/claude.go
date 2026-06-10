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

//go:embed capabilities/claude.yaml
var claudeCapabilitiesYAML []byte

// ClaudeAdapter implements the Adapter interface for Claude Code.
// It reads from .mom/ and generates .claude/CLAUDE.md + settings.json.
type ClaudeAdapter struct {
	projectRoot string
}

// NewClaudeAdapter creates a ClaudeAdapter for the given project root.
func NewClaudeAdapter(projectRoot string) *ClaudeAdapter {
	return &ClaudeAdapter{projectRoot: projectRoot}
}

func (a *ClaudeAdapter) Name() string {
	return "claude"
}

func (a *ClaudeAdapter) Tier() Tier {
	return Fluent
}

func (a *ClaudeAdapter) GenerateContextFile(config Config, constraints []Constraint, skills []Skill, identity *Identity) error {
	claudeDir := filepath.Join(a.projectRoot, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("creating .claude dir: %w", err)
	}

	body := BuildContextContent(config, constraints, skills, identity)
	content := a.Watermark() + "\n\n" + body

	contextFile := filepath.Join(claudeDir, "CLAUDE.md")
	if err := os.WriteFile(contextFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing CLAUDE.md: %w", err)
	}

	return nil
}

// DefaultTranscriptDir returns Claude Code's transcript directory.
func (a *ClaudeAdapter) DefaultTranscriptDir() string {
	return "~/.claude/projects/"
}

func (a *ClaudeAdapter) RegisterHooks() error {
	claudeDir := filepath.Join(a.projectRoot, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.json")

	// Ensure .claude/ exists.
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("creating .claude dir: %w", err)
	}

	// Load existing settings or start fresh.
	settings := make(map[string]any)
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parsing settings.json: %w", err)
		}
	}

	settings["hooks"] = claudeHookSettings()

	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling settings: %w", err)
	}

	data = append(data, '\n')
	if err := os.WriteFile(settingsPath, data, 0644); err != nil {
		return fmt.Errorf("writing settings.json: %w", err)
	}

	return nil
}

func (a *ClaudeAdapter) DetectHarness() bool {
	if commandExists("claude") {
		return true
	}
	if path, err := homePath(".claude"); err == nil && pathExists(path) {
		return true
	}
	if path, err := homePath(".claude.json"); err == nil && pathExists(path) {
		return true
	}
	return false
}

func (a *ClaudeAdapter) GenerateGlobalContextFile(config Config, constraints []Constraint, skills []Skill, identity *Identity) error {
	path, err := homePath(".claude", "CLAUDE.md")
	if err != nil {
		return err
	}
	return upsertManagedBlock(path, buildGlobalContext(a.Watermark(), config, constraints, skills, identity))
}

func (a *ClaudeAdapter) RegisterGlobalHooks() error {
	// Best-effort: strip MOM's retired MCP registration from Claude's JSON
	// configs. v0.50 removed the MCP server, so a lingering mcpServers.mom
	// points at the absent `mom serve mcp` and makes every Claude Code launch
	// try to spawn a dead server. Never fatal — malformed user files are left
	// untouched.
	_, _ = removeStaleClaudeMCP()

	settingsPath, err := homePath(".claude", "settings.json")
	if err != nil {
		return err
	}
	claudeDir := filepath.Dir(settingsPath)
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("creating .claude dir: %w", err)
	}
	settings := make(map[string]any)
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parsing settings.json: %w", err)
		}
	}
	settings["hooks"] = claudeHookSettings()
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling settings: %w", err)
	}
	data = append(data, '\n')
	return os.WriteFile(settingsPath, data, 0644)
}

// removeStaleClaudeMCP deletes MOM's pre-v0.50 MCP server entry from Claude's
// JSON configs (~/.claude.json and ~/.mcp.json). It returns the paths it
// actually rewrote. A file is only rewritten when a mom MCP entry was present,
// so installs without the stale entry are left byte-for-byte untouched.
func removeStaleClaudeMCP() (changed []string, err error) {
	for _, rel := range [][]string{{".claude.json"}, {".mcp.json"}} {
		path, e := homePath(rel...)
		if e != nil {
			continue
		}
		did, e := stripMomMCPFromJSONFile(path)
		if e != nil {
			return changed, e
		}
		if did {
			changed = append(changed, path)
		}
	}
	return changed, nil
}

// stripMomMCPFromJSONFile removes the "mom" key from any mcpServers object in
// a Claude JSON config — both the top-level map and per-project maps under
// projects.<path>.mcpServers (the shape ~/.claude.json uses). Returns whether
// the file changed. Malformed JSON is left untouched (returns false, nil).
func stripMomMCPFromJSONFile(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return false, nil
	}
	changed := deleteMomMCPServer(root)
	if projects, ok := root["projects"].(map[string]any); ok {
		for _, v := range projects {
			if pm, ok := v.(map[string]any); ok {
				if deleteMomMCPServer(pm) {
					changed = true
				}
			}
		}
	}
	if !changed {
		return false, nil
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return false, err
	}
	out = append(out, '\n')
	if err := os.WriteFile(path, out, 0644); err != nil {
		return false, err
	}
	return true, nil
}

// deleteMomMCPServer removes obj["mcpServers"]["mom"] if present, reporting
// whether anything was deleted.
func deleteMomMCPServer(obj map[string]any) bool {
	servers, ok := obj["mcpServers"].(map[string]any)
	if !ok {
		return false
	}
	if _, exists := servers["mom"]; !exists {
		return false
	}
	delete(servers, "mom")
	return true
}

func (a *ClaudeAdapter) GeneratedFiles() []string {
	return []string{
		filepath.Join(".claude", "CLAUDE.md"),
		filepath.Join(".claude", "settings.json"),
	}
}

func (a *ClaudeAdapter) GeneratedDirs() []string {
	return []string{".claude"}
}

func (a *ClaudeAdapter) Watermark() string {
	return "<!-- Generated by MOM — do not edit manually -->"
}

func (a *ClaudeAdapter) Capabilities() AdapterCapability {
	var cap AdapterCapability
	if err := yaml.Unmarshal(claudeCapabilitiesYAML, &cap); err != nil {
		// Fallback: return minimal capability if YAML is malformed.
		return AdapterCapability{Name: "claude-code", Version: "1.0"}
	}
	return cap
}

func claudeHookSettings() map[string]any {
	hooks := []HookDef{
		{Event: "Stop", Command: "mom watch --sweep"},
		{Event: "SessionEnd", Command: "mom watch --sweep"},
	}
	hooksMap := make(map[string]any)
	byEvent := make(map[string][]HookDef)
	for _, h := range hooks {
		byEvent[h.Event] = append(byEvent[h.Event], h)
	}
	for event, defs := range byEvent {
		var matcherGroups []map[string]any
		for _, d := range defs {
			entry := map[string]any{
				"type":    "command",
				"command": d.Command,
				"timeout": 10,
			}
			group := map[string]any{
				"hooks": []map[string]any{entry},
			}
			if d.Matcher != "" {
				group["matcher"] = d.Matcher
			}
			matcherGroups = append(matcherGroups, group)
		}
		hooksMap[event] = matcherGroups
	}
	return hooksMap
}

var (
	_ GlobalAdapter       = (*ClaudeAdapter)(nil)
	_ GlobalHookInstaller = (*ClaudeAdapter)(nil)
	_ HookInstaller       = (*ClaudeAdapter)(nil)
	_ TranscriptSource    = (*ClaudeAdapter)(nil)
)

// HasWatermark checks if a file contains the MOM watermark (or the legacy L.E.O. watermark).
func HasWatermark(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	s := string(data)
	return strings.Contains(s, "Generated by MOM") || strings.Contains(s, "Generated by L.E.O.")
}

// BackupIfNeeded creates a .bkp copy of a file if it exists and was NOT
// generated by MOM (i.e., it's a user file). Returns true if a backup
// was created.
func BackupIfNeeded(path string) (bool, error) {
	if _, err := os.Stat(path); err != nil {
		return false, nil // file doesn't exist, no backup needed
	}

	if HasWatermark(path) {
		return false, nil // it's ours, overwrite freely
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("reading file for backup: %w", err)
	}

	bkpPath := path + ".bkp"
	if err := os.WriteFile(bkpPath, data, 0644); err != nil {
		return false, fmt.Errorf("writing backup: %w", err)
	}

	return true, nil
}
