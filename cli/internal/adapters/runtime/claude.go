package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ClaudeAdapter implements the Adapter interface for Claude Code.
// It reads from .leo/ and generates .claude/CLAUDE.md + settings.json.
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

func (a *ClaudeAdapter) GenerateContextFile(config Config, profile Profile, constraints []Constraint, skills []Skill, identity *Identity) error {
	claudeDir := filepath.Join(a.projectRoot, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("creating .claude dir: %w", err)
	}

	var b strings.Builder

	// Header
	b.WriteString("# LEO — Living Ecosystem Orchestrator\n\n")
	if identity != nil {
		b.WriteString(identity.What)
		b.WriteString("\n\n")
	} else {
		b.WriteString("You are LEO. Your knowledge base lives in `.leo/kb/`.\n\n")
	}

	// Active profile
	fmt.Fprintf(&b, "## Active Profile: %s\n\n", profile.Name)
	fmt.Fprintf(&b, "%s\n\n", profile.Description)
	if profile.ContextInjection != "" {
		b.WriteString(profile.ContextInjection)
		b.WriteString("\n\n")
	}

	// Boot sequence
	b.WriteString("## Boot sequence\n\n")
	b.WriteString("1. Read `.leo/kb/index.json` — this is your neural map\n")
	b.WriteString("2. From the index, load all docs where `boot: true` — these govern your behavior\n")
	b.WriteString("3. You are now loaded. Greet the user and proceed.\n\n")

	// During work
	b.WriteString("## During work\n\n")
	b.WriteString("- When you need context on a topic, check the index for relevant tags\n")
	b.WriteString("- Read only the docs you need — never load the entire KB\n")
	b.WriteString("- When you create or update knowledge, write JSON docs to `.leo/kb/docs/`\n")
	b.WriteString("- Follow the schema at `.leo/kb/schema.json`\n")
	b.WriteString("- Every doc needs: id, type, lifecycle, scope, tags, created, created_by, updated, updated_by, content\n\n")

	// Constraints (from KB data)
	if len(constraints) > 0 {
		b.WriteString("## Constraints\n\n")
		b.WriteString("Always-active guardrails loaded from the KB. Read the full doc when you need detailed guidance.\n\n")
		for _, c := range constraints {
			fmt.Fprintf(&b, "- **%s**: %s → `.leo/kb/constraints/%s.json`\n", c.ID, c.Summary, c.ID)
		}
		b.WriteString("\n")
	}

	// Skills (from KB data)
	if len(skills) > 0 {
		b.WriteString("## Skills\n\n")
		b.WriteString("Composable procedures invoked by trigger or by Leo. Read the full doc for steps and output format.\n\n")
		for _, s := range skills {
			fmt.Fprintf(&b, "- **%s**: %s → `.leo/kb/skills/%s.json`\n", s.ID, s.Summary, s.ID)
		}
		b.WriteString("\n")
	}

	// User preferences — rich behavioral instructions
	b.WriteString(LanguageInstructions(config.User.Language))
	b.WriteString("\n\n")
	b.WriteString(ModeInstructions(config.User.Mode))
	b.WriteString("\n\n")
	b.WriteString(AutonomyInstructions(config.User.Autonomy))
	b.WriteString("\n")

	contextFile := filepath.Join(claudeDir, "CLAUDE.md")
	if err := os.WriteFile(contextFile, []byte(b.String()), 0644); err != nil {
		return fmt.Errorf("writing CLAUDE.md: %w", err)
	}

	return nil
}

func (a *ClaudeAdapter) SupportsHooks() bool {
	return true
}

func (a *ClaudeAdapter) RegisterHooks(hooks []HookDef) error {
	claudeDir := filepath.Join(a.projectRoot, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.json")

	// Load existing settings or start fresh.
	settings := make(map[string]any)
	if data, err := os.ReadFile(settingsPath); err == nil {
		if err := json.Unmarshal(data, &settings); err != nil {
			return fmt.Errorf("parsing settings.json: %w", err)
		}
	}

	// Build hooks structure.
	hooksList := make([]map[string]any, 0, len(hooks))
	for _, h := range hooks {
		hook := map[string]any{
			"type":    h.Event,
			"command": h.Command,
		}
		if h.Matcher != "" {
			hook["matcher"] = h.Matcher
		}
		hooksList = append(hooksList, hook)
	}

	settings["hooks"] = hooksList

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

func (a *ClaudeAdapter) DetectRuntime() bool {
	info, err := os.Stat(filepath.Join(a.projectRoot, ".claude"))
	return err == nil && info.IsDir()
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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
