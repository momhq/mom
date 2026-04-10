import type { ManagerType } from "./managers.js";
import type { SuggestedSpecialist } from "../scanners/stack.js";

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

This project uses [copilot-core](~/Github/copilot-core/).
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
