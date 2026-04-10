import { existsSync, mkdirSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";

export type ManagerType = "engineer" | "designer" | "pm" | "marketing";

interface ManagerConfig {
  name: string;
  description: string;
  extendsFile: string;
}

const MANAGER_CONFIGS: Record<ManagerType, ManagerConfig> = {
  engineer: {
    name: "Engineer Manager",
    description: "Engineering tech lead, extending the core Engineer Manager",
    extendsFile: "engineer.md",
  },
  designer: {
    name: "Designer Manager",
    description: "Design tech lead, extending the core Designer Manager",
    extendsFile: "designer.md",
  },
  pm: {
    name: "PM Manager",
    description: "Product tech lead, extending the core PM Manager",
    extendsFile: "pm.md",
  },
  marketing: {
    name: "Marketing Manager",
    description: "Marketing tech lead, extending the core Marketing Manager",
    extendsFile: "marketing.md",
  },
};

export function generateManagerFile(
  type: ManagerType,
  projectName: string
): string {
  const config = MANAGER_CONFIGS[type];

  return `---
name: ${config.name} (${projectName})
description: ${projectName}'s ${config.description.toLowerCase()}
extends: ../../../../.claude/agents/managers/${config.extendsFile}
tools: Read, Edit, Write, Glob, Grep, Bash, Task
model: sonnet
skills: []
---

## Project-specific additions

<!-- Add project-specific principles, self-QA items, and escalation triggers below.
     These extend the core — they don't replace it. -->
`;
}

export function writeManagerFiles(
  projectDir: string,
  managers: ManagerType[],
  projectName: string
): string[] {
  const managersDir = resolve(projectDir, ".claude", "agents", "managers");
  mkdirSync(managersDir, { recursive: true });

  const created: string[] = [];
  for (const type of managers) {
    const config = MANAGER_CONFIGS[type];
    const filePath = resolve(managersDir, config.extendsFile);

    if (!existsSync(filePath)) {
      writeFileSync(filePath, generateManagerFile(type, projectName));
      created.push(`.claude/agents/managers/${config.extendsFile}`);
    }
  }

  return created;
}
