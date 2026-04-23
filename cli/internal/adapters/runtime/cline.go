package runtime

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed capabilities/cline.yaml
var clineCapabilitiesYAML []byte

// ClineAdapter implements the Adapter interface for Cline.
// It reads from .mom/ and generates .clinerules/mom-context.md.
type ClineAdapter struct {
	projectRoot string
}

// NewClineAdapter creates a ClineAdapter for the given project root.
func NewClineAdapter(projectRoot string) *ClineAdapter {
	return &ClineAdapter{projectRoot: projectRoot}
}

func (a *ClineAdapter) Name() string {
	return "cline"
}

func (a *ClineAdapter) GenerateContextFile(config Config, constraints []Constraint, skills []Skill, identity *Identity) error {
	rulesDir := filepath.Join(a.projectRoot, ".clinerules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		return fmt.Errorf("creating .clinerules dir: %w", err)
	}

	var body string
	if config.Delivery == "context-file" {
		body = BuildContextContent(config, constraints, skills, identity)
	} else {
		body = BuildMinimalContextContent()
	}
	content := a.Watermark() + "\n\n" + body

	contextFile := filepath.Join(rulesDir, "mom-context.md")
	if err := os.WriteFile(contextFile, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing mom-context.md: %w", err)
	}

	return nil
}

func (a *ClineAdapter) SupportsHooks() bool {
	return true
}

func (a *ClineAdapter) RegisterHooks(hooks []HookDef) error {
	hooksDir := filepath.Join(a.projectRoot, ".clinerules", "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return fmt.Errorf("creating hooks dir: %w", err)
	}

	// Group commands by event so multiple commands go into one script.
	byEvent := make(map[string][]string)
	for _, h := range hooks {
		byEvent[h.Event] = append(byEvent[h.Event], h.Command)
	}

	for event, commands := range byEvent {
		scriptName := clineEventToFilename(event)
		scriptPath := filepath.Join(hooksDir, scriptName)

		var sb strings.Builder
		sb.WriteString("#!/bin/sh\n")
		for _, cmd := range commands {
			if strings.Contains(cmd, "record") {
				// Record reads stdin from Cline (conversation data).
				sb.WriteString(cmd + " || true\n")
			} else {
				// Other commands (draft, etc.) don't need stdin.
				sb.WriteString(cmd + " < /dev/null\n")
			}
		}
		sb.WriteString("echo '{\"cancel\": false}'\n")

		if err := os.WriteFile(scriptPath, []byte(sb.String()), 0755); err != nil {
			return fmt.Errorf("writing hook script %s: %w", scriptName, err)
		}
	}
	return nil
}

// clineEventToFilename converts "TaskComplete" to "task-complete.sh".
func clineEventToFilename(event string) string {
	var result []byte
	for i, r := range event {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result = append(result, '-')
		}
		result = append(result, byte(r|0x20)) // toLower
	}
	return string(result) + ".sh"
}

// ClineHooks returns the standard MOM hooks for Cline.
// TaskComplete → mom record --raw + mom draft: captures text and processes it.
// TaskCancel → mom record --raw: captures raw text on cancellation.
// Cline doesn't provide Claude Code's hook JSON, so --raw reads plain text.
func ClineHooks() []HookDef {
	return []HookDef{
		{Event: "TaskComplete", Command: "mom record --raw"},
		{Event: "TaskComplete", Command: "mom draft"},
		{Event: "TaskCancel", Command: "mom record --raw"},
	}
}

func (a *ClineAdapter) DetectRuntime() bool {
	info, err := os.Stat(filepath.Join(a.projectRoot, ".clinerules"))
	return err == nil && info.IsDir()
}

// RegisterMCP writes MOM's MCP server entry to both the project-level .mcp.json
// and Cline's global settings (cline_mcp_settings.json in VS Code storage).
// The project-level file is shared with other runtimes; the global file is what
// Cline actually reads at startup.
func (a *ClineAdapter) RegisterMCP() error {
	// 1. Project-level .mcp.json (shared with other runtimes).
	mcpPath := filepath.Join(a.projectRoot, ".mcp.json")
	if err := upsertMCPEntry(mcpPath); err != nil {
		return err
	}

	// 2. Cline's global settings — includes MOM_PROJECT_DIR env var because
	// Cline VS Code starts MCP subprocesses from a different cwd than the project.
	for _, p := range clineSettingsPaths() {
		if _, err := os.Stat(filepath.Dir(p)); err != nil {
			continue // VS Code / Cursor storage dir doesn't exist.
		}
		_ = upsertMCPEntryWithEnv(p, a.projectRoot) // Silently ignore write errors.
	}

	return nil
}

// upsertMCPEntry reads a JSON file with mcpServers, adds/updates the "mom"
// entry, and writes it back. Creates the file if it doesn't exist.
func upsertMCPEntry(path string) error {
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

// clineSettingsPaths returns candidate paths for Cline's cline_mcp_settings.json.
// It checks: (1) Cline CLI/TUI at ~/.cline/, (2) VS Code extension storage,
// and (3) Cursor extension storage.
func clineSettingsPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	const extID = "saoudrizwan.claude-dev"
	const settingsFile = "settings/cline_mcp_settings.json"

	// Cline CLI/TUI uses ~/.cline/data/settings/cline_mcp_settings.json.
	paths := []string{
		filepath.Join(home, ".cline", "data", settingsFile),
	}

	// VS Code / Cursor extension storage.
	var bases []string
	switch goruntime.GOOS {
	case "darwin":
		bases = []string{
			filepath.Join(home, "Library", "Application Support", "Code", "User", "globalStorage"),
			filepath.Join(home, "Library", "Application Support", "Cursor", "User", "globalStorage"),
		}
	case "linux":
		bases = []string{
			filepath.Join(home, ".config", "Code", "User", "globalStorage"),
			filepath.Join(home, ".config", "Cursor", "User", "globalStorage"),
		}
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData != "" {
			bases = []string{
				filepath.Join(appData, "Code", "User", "globalStorage"),
				filepath.Join(appData, "Cursor", "User", "globalStorage"),
			}
		}
	}

	for _, base := range bases {
		paths = append(paths, filepath.Join(base, extID, settingsFile))
	}
	return paths
}

func (a *ClineAdapter) GeneratedFiles() []string {
	return []string{
		filepath.Join(".clinerules", "mom-context.md"),
		filepath.Join(".clinerules", "hooks", "task-complete.sh"),
		filepath.Join(".clinerules", "hooks", "task-cancel.sh"),
		".mcp.json",
	}
}

// GeneratedDirs returns nil — .clinerules/ may contain user's workflows, hooks,
// and custom rules. Uninstall never removes the directory, only mom-context.md.
func (a *ClineAdapter) GeneratedDirs() []string {
	return nil
}

func (a *ClineAdapter) Watermark() string {
	return "<!-- Generated by MOM — do not edit manually -->"
}

func (a *ClineAdapter) GitIgnorePaths() []string {
	return []string{".clinerules/"}
}

func (a *ClineAdapter) Capabilities() AdapterCapability {
	var cap AdapterCapability
	if err := yaml.Unmarshal(clineCapabilitiesYAML, &cap); err != nil {
		return AdapterCapability{Name: "cline", Version: "0.1"}
	}
	return cap
}
