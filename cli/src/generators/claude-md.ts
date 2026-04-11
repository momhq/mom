import type { ManagerType } from "./managers.js";
import type { SuggestedSpecialist } from "../scanners/stack.js";

// ─── KB Bootloader (new architecture) ────────────────────────────────────────

/** Generate the KB bootloader CLAUDE.md for a project */
export function generateBootloaderClaudeMd(projectName: string): string {
  return `# ${projectName} — LEO Bootloader

You are LEO (Living Ecosystem Orchestrator). Your knowledge base lives in \`.claude/kb/\`.

## Boot sequence

1. Read \`.claude/kb/index.json\` — this is your neural map
2. From the index, load all docs where \`type: "rule"\` — these govern your behavior
3. From the index, load all docs where \`type: "identity"\` — this is who the project is
4. From the index, load all docs where \`type: "skill"\` — these are your executable workflows
5. From the index, load all docs where \`type: "feedback"\` — these are owner corrections to your behavior
6. You are now loaded. Greet the owner and proceed.

## During work

- When you need context on a topic, check the index for relevant tags
- Read only the docs you need — never load the entire KB
- When you create or update knowledge, write JSON docs to \`.claude/kb/docs/\`
- Follow the schema at \`.claude/kb/schema.json\`
- Every doc needs: id, type, lifecycle, scope, tags, created, created_by, updated, updated_by, content

## On wrap-up

When the owner signals end of session, run the wrap-up workflow:
1. Inventory what changed (decisions, patterns, facts, learnings)
2. For each item, create or update a JSON doc in \`.claude/kb/docs/\`
3. Each doc must have meaningful tags — these are the connections
4. Present the plan to the owner (R2) before writing
5. After writing, hooks will automatically validate and rebuild the index

## Rules

All operational rules are in the KB as \`type: "rule"\`. You loaded them at boot.
If the index shows a rule was updated since your last read, re-read it.
`;
}

// ─── Legacy CLAUDE.md (old architecture, kept for compatibility) ─────────────

const MANAGER_LABELS: Record<ManagerType, string> = {
  engineer: "Engineering lead",
  designer: "Design lead",
  pm: "Product lead",
  marketing: "Marketing lead",
};

export function generateClaudeMd(opts: {
  projectName: string;
  description: string;
  managers: ManagerType[];
  specialists: SuggestedSpecialist[];
  constraints?: string;
}): string {
  const managerRows = opts.managers
    .map(
      (m) =>
        `| ${MANAGER_LABELS[m]} | ${m === "engineer" ? "Engineer" : m === "designer" ? "Designer" : m === "pm" ? "PM" : "Marketing"} Manager | .claude/agents/managers/${m === "engineer" ? "engineer" : m === "designer" ? "designer" : m === "pm" ? "pm" : "marketing"}.md |`
    )
    .join("\n");

  const specialistSection =
    opts.specialists.length > 0
      ? `
## Specialists (project-level)

| Name | Domain | File | Status |
|------|--------|------|--------|
${opts.specialists.map((s) => `| ${s.name} | ${s.domain} | .claude/specialists/${s.domain}/${s.name}.md | scaffold — playbook pending |`).join("\n")}

Specialists with "scaffold" status have empty playbooks. The Hiring Loop will fill them on first use.
`
      : "";

  const constraintsSection = opts.constraints
    ? `
## Constraints

See \`context/constraints.md\` for the full list of things the agent should NOT do.
`
    : "";

  return `# ${opts.projectName} — Project Copilot

${opts.description}

This project uses [leo-core](~/Github/leo-core/).
Leo, rules, and Managers load from ~/.claude/ (symlinked from core).

## Team

| Role | Core Manager | Extension |
|------|-------------|-----------|
${managerRows}
${specialistSection}
## Context

- Project overview: .claude/context/project.md
- Stack: .claude/context/stack.md
- Decisions: .claude/context/decisions/
${opts.constraints ? "- Constraints: .claude/context/constraints.md" : ""}
${constraintsSection}
## Flows

| Scope | Flow |
|-------|------|
| Large feature | PRD (PM) → RDD (Engineer) → Execution → Review |
| UI feature | Design (Designer) → Implementation (Engineer) → Review |
| Bug fix | Triage (Engineer) → Fix → Review |
| Content/copy | Draft (Marketing) → Review → Publish |
`;
}
