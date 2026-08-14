package harness

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

//go:embed capabilities/droid.yaml
var droidCapabilitiesYAML []byte

// DroidAdapter implements the Adapter interface for Factory Droid
// (https://factory.ai), a coding agent CLI (`droid`).
// It reads from .mom/ and generates AGENTS.md at the project root, and
// registers Stop/SessionEnd hooks in Droid's settings.json — Droid shares
// Claude Code's hook schema ({"hooks": {"Event": [...]}} inside
// settings.json), unlike Codex's separate hooks.json file.
type DroidAdapter struct {
	projectRoot string
}

// NewDroidAdapter creates a DroidAdapter for the given project root.
func NewDroidAdapter(projectRoot string) *DroidAdapter {
	return &DroidAdapter{projectRoot: projectRoot}
}

func (a *DroidAdapter) Name() string { return "droid" }

func (a *DroidAdapter) Tier() Tier { return Fluent }

func (a *DroidAdapter) GenerateContextFile(config Config, constraints []Constraint, skills []Skill, identity *Identity) error {
	body := BuildContextContent(config, constraints, skills, identity)
	content := a.Watermark() + "\n\n" + body

	agentsFile := filepath.Join(a.projectRoot, "AGENTS.md")
	if err := os.WriteFile(agentsFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing AGENTS.md: %w", err)
	}
	return nil
}

// DefaultTranscriptDir returns Droid's session transcript directory.
// Confirmed on-disk layout: ~/.factory/sessions/<project-slug>/<session-uuid>.jsonl
// (no env var override is known, unlike Codex's $CODEX_HOME).
func (a *DroidAdapter) DefaultTranscriptDir() string {
	return "~/.factory/sessions/"
}

func (a *DroidAdapter) RegisterHooks() error {
	return writeDroidHooks(filepath.Join(a.projectRoot, ".factory", "settings.json"))
}

// RegisterGlobalHooks writes the same hook contract to the user-level
// Droid settings file (~/.factory/settings.json) so every Droid session
// fires `mom watch --sweep` after each response, same defensive sweep
// wired for project-local installs, scoped to the user.
func (a *DroidAdapter) RegisterGlobalHooks() error {
	path, err := homePath(".factory", "settings.json")
	if err != nil {
		return err
	}
	return writeDroidHooks(path)
}

// writeDroidHooks merges MOM's hook entries into a Droid settings.json at
// the given path, preserving any other keys already present. Droid's hooks
// live under the "hooks" key of settings.json, keyed by event name, using
// the same {"matcher","hooks":[{"type":"command","command","timeout"}]}
// shape as Claude Code.
func writeDroidHooks(settingsPath string) error {
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(settingsPath), err)
	}

	settings := make(map[string]any)
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parsing settings.json: %w", err)
		}
	}

	settings["hooks"] = droidHookSettings()

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

// droidHookSettings mirrors claudeHookSettings — Droid uses the same
// {"hooks": {"Event": [{"hooks": [{"type":"command","command","timeout"}]}]}}
// contract as Claude Code.
func droidHookSettings() map[string]any {
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

func (a *DroidAdapter) DetectHarness() bool {
	if commandExists("droid") {
		return true
	}
	if path, err := homePath(".factory"); err == nil && pathExists(path) {
		return true
	}
	return false
}

func (a *DroidAdapter) GenerateGlobalContextFile(config Config, constraints []Constraint, skills []Skill, identity *Identity) error {
	path, err := homePath(".factory", "AGENTS.md")
	if err != nil {
		return err
	}
	return upsertManagedBlock(path, buildGlobalContext(a.Watermark(), config, constraints, skills, identity))
}

func (a *DroidAdapter) GeneratedFiles() []string {
	return []string{
		"AGENTS.md",
		filepath.Join(".factory", "settings.json"),
	}
}

func (a *DroidAdapter) GeneratedDirs() []string {
	return []string{".factory"}
}

func (a *DroidAdapter) Watermark() string {
	return "<!-- Generated by MOM — do not edit manually -->"
}

func (a *DroidAdapter) Capabilities() AdapterCapability {
	var cap AdapterCapability
	if err := yaml.Unmarshal(droidCapabilitiesYAML, &cap); err != nil {
		return AdapterCapability{Name: "droid", Version: "1.0"}
	}
	return cap
}

var (
	_ GlobalAdapter       = (*DroidAdapter)(nil)
	_ HookInstaller       = (*DroidAdapter)(nil)
	_ GlobalHookInstaller = (*DroidAdapter)(nil)
	_ TranscriptSource    = (*DroidAdapter)(nil)
)
