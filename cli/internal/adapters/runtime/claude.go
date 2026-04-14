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

func (a *ClaudeAdapter) GenerateContextFile(config Config, profile Profile, rules []Rule) error {
	claudeDir := filepath.Join(a.projectRoot, ".claude")
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("creating .claude dir: %w", err)
	}

	var b strings.Builder

	// Header
	b.WriteString("# LEO — Living Ecosystem Orchestrator\n\n")
	b.WriteString("You are LEO. Your knowledge base lives in `.leo/kb/`.\n\n")

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
	b.WriteString("2. From the index, load all docs where `type: \"rule\"` — these govern your behavior\n")
	b.WriteString("3. From the index, load all docs where `type: \"identity\"` — this is who the project is\n")
	b.WriteString("4. From the index, load all docs where `type: \"skill\"` — these are your executable workflows\n")
	b.WriteString("5. From the index, load all docs where `type: \"feedback\"` — these are owner corrections\n")
	b.WriteString("6. You are now loaded. Greet the owner and proceed.\n\n")

	// Rules summary
	if len(rules) > 0 {
		b.WriteString("## Rules\n\n")
		b.WriteString("All operational rules are in the KB as `type: \"rule\"`. You loaded them at boot.\n")
		b.WriteString("If the index shows a rule was updated since your last read, re-read it.\n\n")
		b.WriteString("Active rules:\n")
		for _, r := range rules {
			fmt.Fprintf(&b, "- `%s`: %s\n", r.ID, truncate(r.Rule, 100))
		}
		b.WriteString("\n")
	}

	// Owner preferences — rich behavioral instructions
	b.WriteString(LanguageInstructions(config.Owner.Language))
	b.WriteString("\n\n")
	b.WriteString(ModeInstructions(config.Owner.Mode))
	b.WriteString("\n\n")
	b.WriteString(AutonomyInstructions(config.Owner.Autonomy))
	b.WriteString("\n\n")

	// Wrap-up
	b.WriteString("## On wrap-up\n\n")
	b.WriteString("When the owner signals end of session, run the wrap-up workflow:\n")
	b.WriteString("1. Inventory what changed (decisions, patterns, facts, learnings)\n")
	b.WriteString("2. For each item, create or update a JSON doc in `.leo/kb/docs/`\n")
	b.WriteString("3. Each doc must have meaningful tags — these are the connections\n")
	b.WriteString("4. Present the plan to the owner before writing\n")
	b.WriteString("5. After writing, validate and rebuild the index\n")

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

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
