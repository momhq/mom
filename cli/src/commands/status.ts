import { existsSync, readdirSync, lstatSync, statSync } from "node:fs";
import { resolve, basename } from "node:path";
import { getCoreDir, getClaudeDir, getProjectDir } from "../utils/paths.js";
import { header, success, warn, info, p, color } from "../utils/ui.js";

function countSymlinks(dir: string): { linked: number; dangling: number } {
  let linked = 0;
  let dangling = 0;

  if (!existsSync(dir)) return { linked, dangling };

  for (const entry of readdirSync(dir)) {
    const fullPath = resolve(dir, entry);
    try {
      const lstats = lstatSync(fullPath);
      if (lstats.isSymbolicLink()) {
        try {
          statSync(fullPath);
          linked++;
        } catch {
          dangling++;
        }
      }
    } catch {
      // skip
    }
  }

  return { linked, dangling };
}

function countFiles(dir: string, ext: string): number {
  if (!existsSync(dir)) return 0;
  let count = 0;
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    if (entry.isDirectory()) {
      count += countFiles(resolve(dir, entry.name), ext);
    } else if (entry.name.endsWith(ext)) {
      count++;
    }
  }
  return count;
}

export async function status() {
  header("copilot-core status");

  const coreDir = getCoreDir();
  const claudeDir = getClaudeDir();
  const projectDir = getProjectDir();
  const isCoreProject = projectDir === coreDir;

  // Core info
  let version = "unknown";
  try {
    const pkg = await import(resolve(coreDir, "cli", "package.json"), {
      with: { type: "json" },
    });
    version = pkg.default.version;
  } catch {
    // ignore
  }

  info(`Core: ${color.cyan(coreDir)} (v${version})`);

  // Sync status
  const agents = countSymlinks(resolve(claudeDir, "agents"));
  const rules = countSymlinks(resolve(claudeDir, "rules"));
  const skills = countSymlinks(resolve(claudeDir, "skills"));

  info(
    `Synced: ${agents.linked} agents, ${rules.linked} rules, ${skills.linked} skills`
  );

  const totalDangling = agents.dangling + rules.dangling + skills.dangling;
  if (totalDangling > 0) {
    warn(`${totalDangling} dangling symlinks found — run 'copilot-core update' to clean`);
  }

  // Project info (if not in core dir)
  if (!isCoreProject) {
    info("");
    info(`Current project: ${color.cyan(projectDir)}`);

    const managersDir = resolve(projectDir, ".claude", "agents", "managers");
    if (existsSync(managersDir)) {
      const managers = readdirSync(managersDir)
        .filter((f) => f.endsWith(".md"))
        .map((f) => f.replace(".md", ""));
      info(`Managers: ${managers.join(", ") || "none"}`);
    } else {
      warn("Not initialized — run 'copilot-core init' to onboard");
    }

    const specialistsDir = resolve(projectDir, ".claude", "specialists");
    if (existsSync(specialistsDir)) {
      const specialistCount = countFiles(specialistsDir, ".md");
      info(`Specialists: ${specialistCount}`);
    }

    const contextDir = resolve(projectDir, ".claude", "context");
    if (existsSync(contextDir)) {
      const hasProject = existsSync(resolve(contextDir, "project.md"));
      const hasStack = existsSync(resolve(contextDir, "stack.md"));
      const hasConstraints = existsSync(resolve(contextDir, "constraints.md"));
      const contextItems = [
        hasProject ? "project.md" : null,
        hasStack ? "stack.md" : null,
        hasConstraints ? "constraints.md" : null,
      ].filter(Boolean);
      info(`Context: ${contextItems.join(", ") || "none"}`);
    }
  }

  p.outro("");
}
