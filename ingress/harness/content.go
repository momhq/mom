package harness

import (
	"strings"
)

// BuildContextContent generates the always-loaded MOM context block shared by
// every harness adapter. It is delivered as a context file (CLAUDE.md,
// AGENTS.md, …) — the only delivery mode since the MCP server was retired in
// v0.50. Memory now lives as markdown under .mom/vault/; the agent navigates
// it directly with file tools.
//
// The constraints, skills, and identity parameters are retained for interface
// compatibility but are no longer rendered: agent behavior is delivered through
// installed skills, and memory through the vault.
func BuildContextContent(config Config, _ []Constraint, _ []Skill, _ *Identity) string {
	var b strings.Builder

	b.WriteString("# MOM — Memory Oriented Machine\n\n")

	// Deterministic session-start procedure. The agent loads this block on
	// every session, including in directories that are not MOM projects, so
	// step 1 must short-circuit cleanly when nothing is bound.
	b.WriteString("At session start, do this in order:\n")
	b.WriteString("1. Run `mom project`.\n")
	b.WriteString("   - Prints a project id → say \"MOM project: `<id>`\" and continue to step 2.\n")
	b.WriteString("   - Reports no binding → say \"MOM project: **not bound** — run `mom project <id>` to bind\", then stop here.\n")
	b.WriteString("2. Read `.mom/vault/INDEX.md`.\n")
	b.WriteString("   - It exists → read it; it maps what MOM remembers about this project.\n")
	b.WriteString("   - It is missing → say \"MOM vault not built yet — run `mom vault fold`\", then stop here.\n\n")

	b.WriteString("While working:\n")
	b.WriteString("- Before you state what was decided, tried, or preferred on this project, read the matching file under `.mom/vault/` (navigate from `INDEX.md`). Answer from the file, not from memory.\n")
	b.WriteString("- If no vault file matches the topic, say so — never invent past decisions.\n")
	b.WriteString("- Capture is automatic: a background watcher records sessions and folds them into the vault. Never record memories by hand.\n\n")

	b.WriteString("Skills: /mom-status, /mom-project, /mom-fold, /mom-rebuild.\n")

	// User-preference directives (language, communication mode, autonomy)
	// are orthogonal to memory delivery and still apply.
	b.WriteString("\n")
	b.WriteString(LanguageInstructions(config.User.Language))
	b.WriteString("\n\n")
	b.WriteString(CommunicationModeInstructions(config.User.CommunicationMode))
	b.WriteString("\n\n")
	b.WriteString(AutonomyInstructions(config.User.Autonomy))
	b.WriteString("\n")

	return b.String()
}
