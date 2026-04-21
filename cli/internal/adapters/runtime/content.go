package runtime

import (
	"fmt"
	"strings"
)

// BuildContextContent generates the shared Markdown content used by all adapters.
// Each adapter calls this and writes the result to its specific output file.
func BuildContextContent(config Config, constraints []Constraint, skills []Skill, identity *Identity) string {
	var b strings.Builder

	// Header
	b.WriteString("# MOM — Memory Oriented Machine\n\n")
	if identity != nil {
		b.WriteString(identity.What)
		b.WriteString("\n\n")
	} else {
		b.WriteString("You are MOM. Your memory lives in `.leo/memory/`.\n\n")
	}

	// Knowledge base orientation
	b.WriteString("## Knowledge base\n\n")
	b.WriteString("Your knowledge lives in `.mom/` (index, memory, constraints, skills, logs).\n")
	if config.HasMCP {
		b.WriteString("You have MOM tools via MCP — prefer them over raw file reads where available.\n")
	}
	b.WriteString("When you need context on a topic, consult `.mom/index.json` by tags and read only the docs you need.\n")
	b.WriteString("Never load the entire KB upfront.\n\n")

	// During work
	b.WriteString("## During work\n\n")
	b.WriteString("- Read only the docs you need — never load the entire KB\n")
	b.WriteString("- When you create or update knowledge, write JSON docs to `.mom/memory/`\n")
	b.WriteString("- Follow the schema at `.mom/schema.json`\n")
	b.WriteString("- Every doc needs: id, type, lifecycle, scope, tags, created, created_by, updated, updated_by, content\n\n")

	// Constraints
	if len(constraints) > 0 {
		b.WriteString("## Constraints\n\n")
		b.WriteString("Always-active guardrails loaded from the KB. Read the full doc when you need detailed guidance.\n\n")
		for _, c := range constraints {
			fmt.Fprintf(&b, "- **%s**: %s → `.leo/constraints/%s.json`\n", c.ID, c.Summary, c.ID)
		}
		b.WriteString("\n")
	}

	// Skills
	if len(skills) > 0 {
		b.WriteString("## Skills\n\n")
		b.WriteString("Composable procedures invoked by trigger or by MOM. Read the full doc for steps and output format.\n\n")
		for _, s := range skills {
			fmt.Fprintf(&b, "- **%s**: %s → `.leo/skills/%s.json`\n", s.ID, s.Summary, s.ID)
		}
		b.WriteString("\n")
	}

	// Language, autonomy, communication-mode directives
	b.WriteString(LanguageInstructions(config.User.Language))
	b.WriteString("\n\n")
	b.WriteString(CommunicationModeInstructions(config.User.CommunicationMode))
	b.WriteString("\n\n")
	b.WriteString(AutonomyInstructions(config.User.Autonomy))
	b.WriteString("\n")

	return b.String()
}
