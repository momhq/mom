import { existsSync, mkdirSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import type { DetectedStack, SuggestedSpecialist } from "../scanners/stack.js";

export function generateProjectMd(
  projectName: string,
  description: string,
  projectType: string
): string {
  return `# ${projectName} — the project

${description}

## Type

${projectType}

## What lives in .claude/

- **agents/managers/** — project-specific Manager extensions (inherit from core via \`extends:\`)
- **specialists/** — domain specialists created via Hiring Loop
- **context/** — project state, brand, stack, decisions
- **metrics/** — operational metrics (gitignored)
`;
}

export function generateStackMd(
  stacks: DetectedStack[],
  packageManager: string
): string {
  const lines = stacks.map(
    (s) => `- **${s.name}** (detected via ${s.detectedBy})`
  );

  return `# Stack

Auto-detected by leo-core CLI on ${new Date().toISOString().split("T")[0]}.

## Detected technologies

${lines.join("\n")}

## Package manager

${packageManager}

## Notes

<!-- Add architecture notes, important patterns, or conventions here -->
`;
}

export function generateConstraintsMd(constraints: string): string {
  return `# Negative constraints

Things the agent should **NOT** do in this project. These close the ambiguity gap that positive instructions leave open.

${constraints}

<!-- Add more constraints as you discover agent behaviors you want to prevent -->
`;
}

export function generateSpecialistScaffold(
  specialist: SuggestedSpecialist
): string {
  return `---
name: ${specialist.name}
description: ${specialist.description}
domain: ${specialist.domain}
---

## Domain

${specialist.description}

## Playbook

<!-- TODO: to be filled by Hiring Loop on first use.
     When Leo or the Engineer Manager needs this specialist,
     they will trigger the Hiring Loop to flesh out this playbook
     with actionable steps, gotchas, and anti-patterns. -->

## References

<!-- Official docs, relevant PRs, prior learnings -->

## Self-check

<!-- What this specialist must verify before reporting done -->
`;
}

export interface ContextFiles {
  projectMd: string;
  stackMd: string;
  constraintsMd?: string;
}

export function writeContextFiles(
  projectDir: string,
  files: ContextFiles
): string[] {
  const contextDir = resolve(projectDir, ".claude", "context");
  mkdirSync(contextDir, { recursive: true });

  const created: string[] = [];

  const projectPath = resolve(contextDir, "project.md");
  if (!existsSync(projectPath)) {
    writeFileSync(projectPath, files.projectMd);
    created.push(".claude/context/project.md");
  }

  const stackPath = resolve(contextDir, "stack.md");
  if (!existsSync(stackPath)) {
    writeFileSync(stackPath, files.stackMd);
    created.push(".claude/context/stack.md");
  }

  if (files.constraintsMd) {
    const constraintsPath = resolve(contextDir, "constraints.md");
    if (!existsSync(constraintsPath)) {
      writeFileSync(constraintsPath, files.constraintsMd);
      created.push(".claude/context/constraints.md");
    }
  }

  // Create decisions directory
  const decisionsDir = resolve(contextDir, "decisions");
  mkdirSync(decisionsDir, { recursive: true });

  // Create metrics directory
  const metricsDir = resolve(projectDir, ".claude", "metrics");
  mkdirSync(metricsDir, { recursive: true });

  return created;
}

export function writeSpecialistScaffolds(
  projectDir: string,
  specialists: SuggestedSpecialist[]
): string[] {
  const created: string[] = [];

  for (const specialist of specialists) {
    const specDir = resolve(
      projectDir,
      ".claude",
      "specialists",
      specialist.domain
    );
    mkdirSync(specDir, { recursive: true });

    const filePath = resolve(specDir, `${specialist.name}.md`);
    if (!existsSync(filePath)) {
      writeFileSync(filePath, generateSpecialistScaffold(specialist));
      created.push(
        `.claude/specialists/${specialist.domain}/${specialist.name}.md`
      );
    }
  }

  return created;
}
