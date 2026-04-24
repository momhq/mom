package runtime

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

//go:embed capabilities/windsurf.yaml
var windsurfCapabilitiesYAML []byte

// WindsurfAdapter implements the Adapter interface for Windsurf.
// It reads from .mom/ and generates .windsurf/rules/mom.md + hooks.json.
type WindsurfAdapter struct {
	projectRoot string
}

// NewWindsurfAdapter creates a WindsurfAdapter for the given project root.
func NewWindsurfAdapter(projectRoot string) *WindsurfAdapter {
	return &WindsurfAdapter{projectRoot: projectRoot}
}

func (a *WindsurfAdapter) Name() string {
	return "windsurf"
}

func (a *WindsurfAdapter) GenerateContextFile(config Config, constraints []Constraint, skills []Skill, identity *Identity) error {
	rulesDir := filepath.Join(a.projectRoot, ".windsurf", "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		return fmt.Errorf("creating .windsurf/rules dir: %w", err)
	}

	var body string
	if config.Delivery == "context-file" {
		body = BuildContextContent(config, constraints, skills, identity)
	} else {
		body = BuildMinimalContextContent()
	}

	// Windsurf rules require YAML frontmatter
	frontmatter := "---\ntrigger: always_on\n---\n\n"
	content := frontmatter + a.Watermark() + "\n\n" + body

	contextFile := filepath.Join(rulesDir, "mom.md")
	if err := os.WriteFile(contextFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing mom.md: %w", err)
	}
	return nil
}

func (a *WindsurfAdapter) SupportsHooks() bool {
	return true
}

func (a *WindsurfAdapter) RegisterHooks(hooks []HookDef) error {
	windsurfDir := filepath.Join(a.projectRoot, ".windsurf")
	if err := os.MkdirAll(windsurfDir, 0755); err != nil {
		return fmt.Errorf("creating .windsurf dir: %w", err)
	}

	hooksPath := filepath.Join(windsurfDir, "hooks.json")

	// Windsurf hooks.json format: { "hooks": { "event": [ { "command": "...", "working_directory": "..." }, ... ] } }
	// working_directory is required because Windsurf may invoke hooks from a different cwd.
	byEvent := make(map[string][]map[string]any)
	for _, h := range hooks {
		entry := map[string]any{
			"command":           h.Command,
			"working_directory": a.projectRoot,
		}
		byEvent[h.Event] = append(byEvent[h.Event], entry)
	}

	root := map[string]any{
		"hooks": byEvent,
	}

	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling hooks: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(hooksPath, data, 0644); err != nil {
		return fmt.Errorf("writing hooks.json: %w", err)
	}
	return nil
}

// WindsurfHooks returns the standard MOM hooks for Windsurf.
// post_cascade_response_with_transcript provides transcript_path in the hook
// JSON input — same format as Claude Code hooks, so we use plain "mom record".
func WindsurfHooks() []HookDef {
	return []HookDef{
		{Event: "post_cascade_response_with_transcript", Command: "mom record"},
		{Event: "post_cascade_response_with_transcript", Command: "mom draft"},
	}
}

func (a *WindsurfAdapter) DetectRuntime() bool {
	info, err := os.Stat(filepath.Join(a.projectRoot, ".windsurf"))
	return err == nil && info.IsDir()
}

// RegisterMCP writes MOM's MCP server entry to both the project-level .mcp.json
// and Windsurf's global config at ~/.codeium/windsurf/mcp_config.json.
//
// Windsurf only reads the global config for MCP servers. The global entry includes
// MOM_PROJECT_DIR so the MCP server resolves the correct scope. In multi-project
// setups the last project to call RegisterMCP wins — run `mom upgrade` in the
// active project to point the global config at it.
func (a *WindsurfAdapter) RegisterMCP() error {
	// 1. Project-level .mcp.json (shared with other runtimes).
	mcpPath := filepath.Join(a.projectRoot, ".mcp.json")
	if err := upsertMCPEntryWithEnv(mcpPath, a.projectRoot); err != nil {
		return err
	}

	// 2. Windsurf global config.
	home, err := os.UserHomeDir()
	if err == nil {
		globalConfig := filepath.Join(home, ".codeium", "windsurf", "mcp_config.json")
		if _, err := os.Stat(filepath.Dir(globalConfig)); err == nil {
			_ = upsertMCPEntryWithEnv(globalConfig, a.projectRoot)
		}
	}

	return nil
}

// upsertMCPEntryWithEnv is like upsertMCPEntry but adds MOM_PROJECT_DIR env var.
// Used by runtimes that start MCP subprocesses from a different cwd (Windsurf, Cline VS Code).
func upsertMCPEntryWithEnv(path, projectRoot string) error {
	root := make(map[string]any)
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &root); err != nil {
			return fmt.Errorf("parsing %s: %w", filepath.Base(path), err)
		}
	}

	servers, _ := root["mcpServers"].(map[string]any)
	if servers == nil {
		servers = make(map[string]any)
	}

	servers["mom"] = map[string]any{
		"command": "mom",
		"args":    []string{"serve", "mcp"},
		"env": map[string]string{
			"MOM_PROJECT_DIR": projectRoot,
		},
	}
	root["mcpServers"] = servers

	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling %s: %w", filepath.Base(path), err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", filepath.Base(path), err)
	}
	return nil
}

func (a *WindsurfAdapter) GeneratedFiles() []string {
	return []string{
		filepath.Join(".windsurf", "rules", "mom.md"),
		filepath.Join(".windsurf", "hooks.json"),
		".mcp.json",
	}
}

func (a *WindsurfAdapter) GeneratedDirs() []string {
	return []string{".windsurf"}
}

func (a *WindsurfAdapter) Watermark() string {
	return "<!-- Generated by MOM — do not edit manually -->"
}

func (a *WindsurfAdapter) GitIgnorePaths() []string {
	return []string{".windsurf/"}
}

func (a *WindsurfAdapter) Capabilities() AdapterCapability {
	var cap AdapterCapability
	if err := yaml.Unmarshal(windsurfCapabilitiesYAML, &cap); err != nil {
		// Fallback: return minimal capability if YAML is malformed.
		return AdapterCapability{Name: "windsurf", Version: "1.0"}
	}
	return cap
}
