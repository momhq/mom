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
	b.WriteString("2. From the index, load all docs where `boot: true` — these govern your behavior, identity, skills, and corrections\n")
	b.WriteString("3. You are now loaded. Greet the owner and proceed.\n\n")

	// During work
	b.WriteString("## During work\n\n")
	b.WriteString("- When you need context on a topic, check the index for relevant tags\n")
	b.WriteString("- Read only the docs you need — never load the entire KB\n")
	b.WriteString("- When you create or update knowledge, write JSON docs to `.leo/kb/docs/`\n")
	b.WriteString("- Follow the schema at `.leo/kb/schema.json`\n")
	b.WriteString("- Every doc needs: id, type, lifecycle, scope, tags, created, created_by, updated, updated_by, content\n\n")

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

	// Delegation
	b.WriteString("## Delegation\n\n")
	b.WriteString("You are the orchestrator — you route, judge, and synthesize. You do NOT execute.\n")
	b.WriteString("Before every task, consult the `task-pipeline-selection` rule to size the pipeline:\n")
	b.WriteString("- Small (one-file fix): delegate `execute` to one Specialist\n")
	b.WriteString("- Medium (multi-file): delegate `execute` → `review-code`\n")
	b.WriteString("- Large (new feature/architecture): delegate `analyze-architecture` → `execute` → `write-tests` → `review-code`\n")
	b.WriteString("- Security-sensitive: add `review-security` to any pipeline\n\n")
	b.WriteString("Resolve each function to a concrete profile from `.leo/profiles/` based on project context.\n")
	b.WriteString("The only work you do directly: routing, propagation, memory management, synthesis for the owner.\n\n")

	// Owner preferences — rich behavioral instructions
	b.WriteString(LanguageInstructions(config.Owner.Language))
	b.WriteString("\n\n")
	b.WriteString(ModeInstructions(config.Owner.Mode))
	b.WriteString("\n\n")
	b.WriteString(AutonomyInstructions(config.Owner.Autonomy))
	b.WriteString("\n\n")

	// Feedback and corrections
	b.WriteString("## Feedback and corrections\n\n")
	b.WriteString("When the owner corrects your behavior, persist it as a KB doc (`type: \"feedback\"`, `boot: true`)\n")
	b.WriteString("in `.leo/kb/docs/`, NOT as an auto-memory `.md` file. Behavioral feedback is organizational\n")
	b.WriteString("knowledge — it must be versioned, validated by schema, and loaded at boot so you never repeat\n")
	b.WriteString("the same mistake. Auto-memory is only for user-specific preferences and platform quirks\n")
	b.WriteString("that don't belong in the versioned KB.\n\n")

	// Memory boundaries
	b.WriteString("## Memory boundaries\n\n")
	b.WriteString("| Destination | What goes there |\n")
	b.WriteString("|---|---|\n")
	b.WriteString("| KB (`.leo/kb/docs/`) | Everything about LEO's behavior, rules, decisions, feedback, patterns, facts — versioned, schema-validated, loaded via boot or tags |\n")
	b.WriteString("| Auto-memory (`~/.claude/projects/.../memory/`) | User-specific preferences (tone, cognitive style), platform-specific notes (Claude Code quirks). NOT behavioral rules or feedback. |\n")
	b.WriteString("| Neither | Implementation details derivable from code, git history, or temporary task state |\n\n")
	b.WriteString("When in doubt, use the KB. Auto-memory is the exception, not the default.\n\n")

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
