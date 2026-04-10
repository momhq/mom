import { existsSync, mkdirSync, readdirSync, lstatSync, readlinkSync, symlinkSync, unlinkSync, statSync } from "node:fs";
import { resolve, basename, join } from "node:path";
import { getCoreDir, getClaudeDir } from "../utils/paths.js";
import { header, success, warn, error, info, p } from "../utils/ui.js";

function findMdFiles(dir: string): string[] {
  const results: string[] = [];
  if (!existsSync(dir)) return results;

  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const fullPath = join(dir, entry.name);
    if (entry.isDirectory()) {
      results.push(...findMdFiles(fullPath));
    } else if (entry.name.endsWith(".md")) {
      results.push(fullPath);
    }
  }
  return results;
}

function findTopLevelDirs(dir: string): string[] {
  if (!existsSync(dir)) return [];
  return readdirSync(dir, { withFileTypes: true })
    .filter((e) => e.isDirectory())
    .map((e) => join(dir, e.name));
}

function syncSymlink(source: string, target: string): void {
  if (existsSync(target) || lstatSync(target).isSymbolicLink?.()) {
    unlinkSync(target);
  }
  symlinkSync(source, target);
}

function safeSync(source: string, target: string): boolean {
  try {
    // Remove existing symlink or file at target
    try {
      const stat = lstatSync(target);
      if (stat) unlinkSync(target);
    } catch {
      // Target doesn't exist, that's fine
    }
    symlinkSync(source, target);
    return true;
  } catch (err) {
    return false;
  }
}

function cleanDanglingSymlinks(dir: string): number {
  let count = 0;
  if (!existsSync(dir)) return count;

  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const fullPath = join(dir, entry.name);
    try {
      const lstats = lstatSync(fullPath);
      if (lstats.isSymbolicLink()) {
        try {
          statSync(fullPath); // follows symlink — throws if dangling
        } catch {
          unlinkSync(fullPath);
          count++;
        }
      }
    } catch {
      // skip
    }
  }
  return count;
}

export async function setup() {
  header("copilot-core setup");

  const coreDir = getCoreDir();
  const claudeDir = getClaudeDir();

  // Sanity checks
  const agentsDir = resolve(coreDir, "agents");
  const rulesDir = resolve(coreDir, "rules");
  const skillsDir = resolve(coreDir, "skills");

  if (!existsSync(agentsDir) || !existsSync(rulesDir)) {
    error(`Not a valid copilot-core directory: ${coreDir}`);
    error("Expected agents/ and rules/ directories");
    process.exit(1);
  }

  success(`Core directory found at ${coreDir}`);

  // Create target dirs
  const targetAgents = resolve(claudeDir, "agents");
  const targetRules = resolve(claudeDir, "rules");
  const targetSkills = resolve(claudeDir, "skills");

  mkdirSync(targetAgents, { recursive: true });
  mkdirSync(targetRules, { recursive: true });
  mkdirSync(targetSkills, { recursive: true });

  // Sync agents (recursive find, flat destination)
  const agentFiles = findMdFiles(agentsDir);
  let agentCount = 0;
  for (const src of agentFiles) {
    const name = basename(src);
    if (safeSync(src, resolve(targetAgents, name))) {
      agentCount++;
    }
  }
  success(`Linked ${agentCount} agents`);

  // Sync rules
  const ruleFiles = findMdFiles(rulesDir);
  let ruleCount = 0;
  for (const src of ruleFiles) {
    const name = basename(src);
    if (safeSync(src, resolve(targetRules, name))) {
      ruleCount++;
    }
  }
  success(`Linked ${ruleCount} rules`);

  // Sync skills (symlink directories)
  const skillDirs = findTopLevelDirs(skillsDir);
  let skillCount = 0;
  for (const src of skillDirs) {
    const name = basename(src);
    if (safeSync(src, resolve(targetSkills, name))) {
      skillCount++;
    }
  }
  success(`Linked ${skillCount} skills`);

  // Clean dangling symlinks
  let danglingCount = 0;
  danglingCount += cleanDanglingSymlinks(targetAgents);
  danglingCount += cleanDanglingSymlinks(targetRules);
  danglingCount += cleanDanglingSymlinks(targetSkills);

  if (danglingCount > 0) {
    warn(`Cleaned ${danglingCount} dangling symlinks`);
  }

  info("");
  info(`Source:  ${coreDir}`);
  info(`Target:  ${claudeDir}`);

  p.outro("Setup complete. Run 'copilot-core init' in a project to onboard it.");
}
