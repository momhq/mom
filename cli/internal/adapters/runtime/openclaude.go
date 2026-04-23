package runtime

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

//go:embed capabilities/openclaude.yaml
var openclaudeCapabilitiesYAML []byte

// OpenClaudeAdapter implements the Adapter interface for OpenClaude.
// It reads from .mom/ and generates .openclaude/CLAUDE.md + settings.json.
type OpenClaudeAdapter struct {
	projectRoot string
}

// NewOpenClaudeAdapter creates an OpenClaudeAdapter for the given project root.
func NewOpenClaudeAdapter(projectRoot string) *OpenClaudeAdapter {
	return &OpenClaudeAdapter{projectRoot: projectRoot}
}

func (a *OpenClaudeAdapter) Name() string {
	return "openclaude"
}

func (a *OpenClaudeAdapter) GenerateContextFile(config Config, constraints []Constraint, skills []Skill, identity *Identity) error {
	openclaudeDir := filepath.Join(a.projectRoot, ".openclaude")
	if err := os.MkdirAll(openclaudeDir, 0755); err != nil {
		return fmt.Errorf("creating .openclaude dir: %w", err)
	}

	var body string
	if config.Delivery == "context-file" {
		body = BuildContextContent(config, constraints, skills, identity)
	} else {
		body = BuildMinimalContextContent()
	}
	content := a.Watermark() + "\n\n" + body

	contextFile := filepath.Join(openclaudeDir, "CLAUDE.md")
	if err := os.WriteFile(contextFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing CLAUDE.md: %w", err)
	}

	return nil
}

func (a *OpenClaudeAdapter) SupportsHooks() bool {
	return true
}

func (a *OpenClaudeAdapter) RegisterHooks(hooks []HookDef) error {
	openclaudeDir := filepath.Join(a.projectRoot, ".openclaude")
	settingsPath := filepath.Join(openclaudeDir, "settings.json")

	// Ensure .openclaude/ exists.
	if err := os.MkdirAll(openclaudeDir, 0755); err != nil {
		return fmt.Errorf("creating .openclaude dir: %w", err)
	}

	// Load existing settings or start fresh.
	settings := make(map[string]any)
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parsing settings.json: %w", err)
		}
	}

	// Build hooks structure (same format as Claude Code):
	//   "hooks": { "EventName": [ { "matcher": "...", "hooks": [ {...} ] } ] }
	hooksMap := make(map[string]any)

	// Group HookDefs by event.
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

	settings["hooks"] = hooksMap

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

// OpenClaudeHooks returns the standard MOM hooks for OpenClaude.
// Stop → mom record: captures raw transcript after each response.
// SessionEnd → mom draft: processes raw into draft memories at session close.
func OpenClaudeHooks() []HookDef {
	return []HookDef{
		{
			Event:   "Stop",
			Command: "mom record",
		},
		{
			Event:   "SessionEnd",
			Command: "mom draft",
		},
	}
}

func (a *OpenClaudeAdapter) DetectRuntime() bool {
	info, err := os.Stat(filepath.Join(a.projectRoot, ".openclaude"))
	return err == nil && info.IsDir()
}

// RegisterMCP writes or updates .mcp.json at the project root, injecting the
// MOM MCP server entry. Existing entries for other servers are preserved.
func (a *OpenClaudeAdapter) RegisterMCP() error {
	mcpPath := filepath.Join(a.projectRoot, ".mcp.json")

	// Load existing .mcp.json or start fresh.
	root := make(map[string]any)
	if data, err := os.ReadFile(mcpPath); err == nil {
		if err := json.Unmarshal(data, &root); err != nil {
			return fmt.Errorf("parsing .mcp.json: %w", err)
		}
	}

	servers, _ := root["mcpServers"].(map[string]any)
	if servers == nil {
		servers = make(map[string]any)
	}

	servers["mom"] = map[string]any{
		"command": "mom",
		"args":    []string{"serve", "mcp"},
	}
	root["mcpServers"] = servers

	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling .mcp.json: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(mcpPath, data, 0644); err != nil {
		return fmt.Errorf("writing .mcp.json: %w", err)
	}

	return nil
}

func (a *OpenClaudeAdapter) GeneratedFiles() []string {
	return []string{
		filepath.Join(".openclaude", "CLAUDE.md"),
		filepath.Join(".openclaude", "settings.json"),
		".mcp.json",
	}
}

func (a *OpenClaudeAdapter) GeneratedDirs() []string {
	return []string{".openclaude"}
}

func (a *OpenClaudeAdapter) Watermark() string {
	return "<!-- Generated by MOM — do not edit manually -->"
}

func (a *OpenClaudeAdapter) GitIgnorePaths() []string {
	return []string{".openclaude/", "CLAUDE.md"}
}

func (a *OpenClaudeAdapter) Capabilities() AdapterCapability {
	var cap AdapterCapability
	if err := yaml.Unmarshal(openclaudeCapabilitiesYAML, &cap); err != nil {
		// Fallback: return minimal capability if YAML is malformed.
		return AdapterCapability{Name: "openclaude", Version: "1.0"}
	}
	return cap
}
