import {
  existsSync,
  mkdirSync,
  writeFileSync,
  readFileSync,
  copyFileSync,
  readdirSync,
  chmodSync,
} from "node:fs";
import { execSync } from "node:child_process";
import { resolve, basename } from "node:path";
import { getCoreDir } from "../utils/paths.js";

// ─── Types ───────────────────────────────────────────────────────��───────────

export interface KbDoc {
  id: string;
  type: string;
  lifecycle: string;
  scope: string;
  tags: string[];
  created: string;
  created_by: string;
  updated: string;
  updated_by: string;
  content: Record<string, unknown>;
}

// ─── KB Structure ────────────────────────────────��───────────────────────────

/** Create the .claude/kb/ directory structure in a project */
export function createKbStructure(projectDir: string): string[] {
  const created: string[] = [];
  const kbDir = resolve(projectDir, ".claude", "kb");
  const docsDir = resolve(kbDir, "docs");
  const scriptsDir = resolve(kbDir, "scripts");

  for (const dir of [kbDir, docsDir, scriptsDir]) {
    mkdirSync(dir, { recursive: true });
  }

  // Copy schema.json from core
  const coreDir = getCoreDir();
  const coreSchema = resolve(coreDir, ".claude", "kb", "schema.json");
  const targetSchema = resolve(kbDir, "schema.json");

  if (existsSync(coreSchema)) {
    copyFileSync(coreSchema, targetSchema);
    created.push(".claude/kb/schema.json");
  }

  // Copy scripts from core
  const coreScripts = resolve(coreDir, ".claude", "kb", "scripts");
  if (existsSync(coreScripts)) {
    for (const file of readdirSync(coreScripts)) {
      if (file.endsWith(".sh")) {
        const src = resolve(coreScripts, file);
        const dest = resolve(scriptsDir, file);
        copyFileSync(src, dest);
        chmodSync(dest, 0o755);
        created.push(`.claude/kb/scripts/${file}`);
      }
    }
  }

  return created;
}

/** Copy hooks from core to project */
export function setupKbHooks(projectDir: string): string[] {
  const created: string[] = [];
  const coreDir = getCoreDir();
  const coreHooks = resolve(coreDir, ".claude", "hooks");
  const targetHooks = resolve(projectDir, ".claude", "hooks");

  mkdirSync(targetHooks, { recursive: true });

  if (existsSync(coreHooks)) {
    for (const file of readdirSync(coreHooks)) {
      if (file.endsWith(".sh")) {
        const src = resolve(coreHooks, file);
        const dest = resolve(targetHooks, file);
        copyFileSync(src, dest);
        chmodSync(dest, 0o755);
        created.push(`.claude/hooks/${file}`);
      }
    }
  }

  // Create or merge settings.json with hooks config
  const settingsPath = resolve(projectDir, ".claude", "settings.json");
  let settings: Record<string, unknown> = {};

  if (existsSync(settingsPath)) {
    try {
      settings = JSON.parse(readFileSync(settingsPath, "utf-8"));
    } catch {
      // Invalid JSON, start fresh
    }
  }

  settings.hooks = {
    PostToolUse: [
      {
        matcher: "Write",
        hooks: [
          {
            type: "command",
            command: ".claude/hooks/validate-kb-doc.sh",
            timeout: 15,
          },
        ],
      },
    ],
  };

  writeFileSync(settingsPath, JSON.stringify(settings, null, 2) + "\n");
  created.push(".claude/settings.json (hooks configured)");

  return created;
}

// ─── Doc Generators ──────────────────────────────────────────────────────���───

const now = () => new Date().toISOString().replace(/\.\d{3}Z$/, "Z");

/** Generate a project identity doc from project info */
export function generateIdentityDoc(opts: {
  projectName: string;
  description: string;
  projectType: string;
  stacks: string[];
  constraints?: string;
}): KbDoc {
  const constraintsList = opts.constraints
    ? opts.constraints.split("\n").map((l) => l.trim()).filter(Boolean)
    : [];

  return {
    id: "project-identity",
    type: "identity",
    lifecycle: "permanent",
    scope: "project",
    tags: ["identity", "project"],
    created: now(),
    created_by: "cli",
    updated: now(),
    updated_by: "cli",
    content: {
      what: `${opts.projectName} — ${opts.description}`,
      stack: opts.stacks,
      philosophy: "",
      constraints: constraintsList,
    },
  };
}

/** Convert a context/decisions/*.md file to a KB decision doc */
export function convertDecisionMd(filePath: string): KbDoc | null {
  if (!existsSync(filePath)) return null;

  const content = readFileSync(filePath, "utf-8");
  const filename = basename(filePath, ".md");
  const id = toKebabCase(filename);

  // Extract title from first heading
  const titleMatch = content.match(/^#\s+(.+)$/m);
  const title = titleMatch ? titleMatch[1] : filename;

  // Extract sections
  const sections = parseMdSections(content);

  return {
    id,
    type: "decision",
    lifecycle: "learning",
    scope: "project",
    tags: extractTags(content, filename),
    created: now(),
    created_by: "cli-migration",
    updated: now(),
    updated_by: "cli-migration",
    content: {
      decision: title,
      context: sections.context || sections.why || sections.background || content.slice(0, 500),
      alternatives_considered: sections.alternatives
        ? sections.alternatives.split("\n").filter((l: string) => l.trim().startsWith("-")).map((l: string) => l.replace(/^-\s*/, "").trim())
        : [],
      impact: sections.impact
        ? sections.impact.split("\n").filter((l: string) => l.trim().startsWith("-")).map((l: string) => l.replace(/^-\s*/, "").trim())
        : [],
      reversible: true,
    },
  };
}

/** Convert context/project.md to identity fields */
export function parseProjectMd(filePath: string): {
  description: string;
  projectType: string;
} {
  if (!existsSync(filePath)) {
    return { description: "", projectType: "" };
  }

  const content = readFileSync(filePath, "utf-8");
  const sections = parseMdSections(content);

  // Extract description from first non-heading paragraph
  const lines = content.split("\n");
  let description = "";
  for (const line of lines) {
    if (line.startsWith("#")) continue;
    if (line.trim() === "") continue;
    description = line.trim();
    break;
  }

  return {
    description,
    projectType: sections.type?.trim() || "",
  };
}

/** Convert context/stack.md to a list of stack items */
export function parseStackMd(filePath: string): string[] {
  if (!existsSync(filePath)) return [];

  const content = readFileSync(filePath, "utf-8");
  const stacks: string[] = [];

  for (const line of content.split("\n")) {
    const match = line.match(/^-\s+\*\*(.+?)\*\*/);
    if (match) {
      stacks.push(match[1]);
    }
  }

  return stacks;
}

// ─── Write helpers ──────────────��────────────────────────────────────────────

/** Write a KB doc to .claude/kb/docs/{id}.json */
export function writeKbDoc(projectDir: string, doc: KbDoc): string {
  const docsDir = resolve(projectDir, ".claude", "kb", "docs");
  mkdirSync(docsDir, { recursive: true });

  const filePath = resolve(docsDir, `${doc.id}.json`);
  writeFileSync(filePath, JSON.stringify(doc, null, 2) + "\n");
  return `.claude/kb/docs/${doc.id}.json`;
}

/** Run build-index.sh in a project */
export function rebuildIndex(projectDir: string): boolean {
  const script = resolve(projectDir, ".claude", "kb", "scripts", "build-index.sh");
  if (!existsSync(script)) return false;

  try {
    execSync(script, { cwd: projectDir, stdio: "pipe" });
    return true;
  } catch {
    return false;
  }
}

// ─── Internal helpers ────────────────────────────────────────────────────────

function toKebabCase(str: string): string {
  return str
    .replace(/[A-Z]/g, (m) => `-${m.toLowerCase()}`)
    .replace(/[^a-z0-9-]/g, "-")
    .replace(/--+/g, "-")
    .replace(/^-|-$/g, "")
    .toLowerCase();
}

function parseMdSections(content: string): Record<string, string> {
  const sections: Record<string, string> = {};
  let currentKey = "";
  let currentContent: string[] = [];

  for (const line of content.split("\n")) {
    const headingMatch = line.match(/^##\s+(.+)$/);
    if (headingMatch) {
      if (currentKey) {
        sections[currentKey] = currentContent.join("\n").trim();
      }
      currentKey = headingMatch[1].toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
      currentContent = [];
    } else if (currentKey) {
      currentContent.push(line);
    }
  }

  if (currentKey) {
    sections[currentKey] = currentContent.join("\n").trim();
  }

  return sections;
}

function extractTags(content: string, filename: string): string[] {
  const tags = new Set<string>();

  // Add filename-derived tag
  const parts = filename.split("-");
  for (const part of parts) {
    if (part.length > 2) tags.add(part.toLowerCase());
  }

  // Look for common domain keywords
  const keywords = [
    "auth", "api", "database", "ui", "design", "deploy", "testing",
    "security", "performance", "migration", "architecture", "product",
    "marketing", "infrastructure", "mobile", "web", "backend", "frontend",
  ];

  const lower = content.toLowerCase();
  for (const kw of keywords) {
    if (lower.includes(kw)) tags.add(kw);
  }

  // Ensure at least one tag
  if (tags.size === 0) tags.add("general");

  return [...tags].slice(0, 8);
}
