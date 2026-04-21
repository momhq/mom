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
	b.WriteString("# LEO — Living Ecosystem Orchestrator\n\n")
	if identity != nil {
		b.WriteString(identity.What)
		b.WriteString("\n\n")
	} else {
		b.WriteString("You are LEO. Your memory lives in `.mom/memory/`.\n\n")
	}

	// Boot sequence
	b.WriteString("## Boot sequence\n\n")
	b.WriteString("1. Read `.mom/index.json` — this is your neural map\n")
	b.WriteString("2. From the index, load all docs where `boot: true` — these govern your behavior\n")
	b.WriteString("3. You are now loaded. Greet the user and proceed.\n\n")

	// During work
	b.WriteString("## During work\n\n")
	b.WriteString("- When you need context on a topic, check the index for relevant tags\n")
	b.WriteString("- Read only the docs you need — never load the entire KB\n")
	b.WriteString("- When you create or update knowledge, write JSON docs to `.mom/memory/`\n")
	b.WriteString("- Follow the schema at `.mom/schema.json`\n")
	b.WriteString("- Every doc needs: id, type, lifecycle, scope, tags, created, created_by, updated, updated_by, content\n\n")

	// Constraints
	if len(constraints) > 0 {
		b.WriteString("## Constraints\n\n")
		b.WriteString("Always-active guardrails loaded from the KB. Read the full doc when you need detailed guidance.\n\n")
		for _, c := range constraints {
			fmt.Fprintf(&b, "- **%s**: %s → `.mom/constraints/%s.json`\n", c.ID, c.Summary, c.ID)
		}
		b.WriteString("\n")
	}

	// Skills
	if len(skills) > 0 {
		b.WriteString("## Skills\n\n")
		b.WriteString("Composable procedures invoked by trigger or by Leo. Read the full doc for steps and output format.\n\n")
		for _, s := range skills {
			fmt.Fprintf(&b, "- **%s**: %s → `.mom/skills/%s.json`\n", s.ID, s.Summary, s.ID)
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
