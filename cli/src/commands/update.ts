import { execSync } from "node:child_process";
import {
  existsSync,
  readdirSync,
  readFileSync,
  writeFileSync,
  renameSync,
} from "node:fs";
import { resolve, join } from "node:path";
import { homedir } from "node:os";
import { getCoreDir } from "../utils/paths.js";
import { header, success, warn, info, p, color } from "../utils/ui.js";
import { setup } from "./setup.js";

// --- Migration rules --------------------------------------------------------
// Each rule describes a rename/refactor that happened in the core.
// `update` scans all projects under ~/Github/ and applies fixes automatically.

interface MigrationRule {
  id: string;
  description: string;
  apply: (projectDir: string) => string[]; // returns list of changes made
}

const MIGRATIONS: MigrationRule[] = [
  {
    id: "dev-to-engineer",
    description: "Rename dev.md → engineer.md and Dev Manager → Engineer Manager",
    apply(projectDir: string): string[] {
      const changes: string[] = [];
      const managersDir = resolve(projectDir, ".claude", "agents", "managers");

      if (!existsSync(managersDir)) return changes;

      // Rename file if exists
      const oldFile = resolve(managersDir, "dev.md");
      const newFile = resolve(managersDir, "engineer.md");
      if (existsSync(oldFile) && !existsSync(newFile)) {
        renameSync(oldFile, newFile);
        changes.push("Renamed .claude/agents/managers/dev.md → engineer.md");
      }

      // Fix extends references in all .md files under .claude/
      const claudeDir = resolve(projectDir, ".claude");
      const mdFiles = findAllMd(claudeDir);
      for (const file of mdFiles) {
        let content = readFileSync(file, "utf-8");
        const original = content;

        content = content.replace(
          /extends:(.*)\/dev\.md/g,
          "extends:$1/engineer.md"
        );
        content = content.replace(/Dev Manager/g, "Engineer Manager");
        content = content.replace(
          /name: Dev Manager/g,
          "name: Engineer Manager"
        );

        if (content !== original) {
          writeFileSync(file, content);
          const relPath = file.replace(projectDir + "/", "");
          changes.push(`Updated references in ${relPath}`);
        }
      }

      // Fix CLAUDE.md at project root
      const claudeMd = resolve(projectDir, "CLAUDE.md");
      if (existsSync(claudeMd)) {
        let content = readFileSync(claudeMd, "utf-8");
        const original = content;

        content = content.replace(/Dev Manager/g, "Engineer Manager");
        content = content.replace(/dev\.md/g, "engineer.md");

        if (content !== original) {
          writeFileSync(claudeMd, content);
          changes.push("Updated references in CLAUDE.md");
        }
      }

      return changes;
    },
  },
];

function findAllMd(dir: string): string[] {
  const results: string[] = [];
  if (!existsSync(dir)) return results;

  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const fullPath = join(dir, entry.name);
    if (entry.isDirectory() && entry.name !== "node_modules") {
      results.push(...findAllMd(fullPath));
    } else if (entry.name.endsWith(".md")) {
      results.push(fullPath);
    }
  }
  return results;
}

function findProjects(): string[] {
  const githubDir = resolve(homedir(), "Github");
  if (!existsSync(githubDir)) return [];

  return readdirSync(githubDir, { withFileTypes: true })
    .filter((e) => e.isDirectory())
    .map((e) => resolve(githubDir, e.name))
    .filter((dir) => existsSync(resolve(dir, ".claude")));
}

// --- Main command -----------------------------------------------------------

export async function update() {
  header("leo update");

  const coreDir = getCoreDir();

  // --- Step 1: Check for core updates ---
  const spinner = p.spinner();
  spinner.start("Checking for updates...");

  try {
    execSync("git fetch origin", { cwd: coreDir, stdio: "pipe" });

    const status = execSync("git status -uno --porcelain -b", {
      cwd: coreDir,
      encoding: "utf-8",
    });

    const behind = status.includes("behind");
    const ahead = status.includes("ahead");

    if (behind) {
      spinner.stop("Updates available");

      const log = execSync("git log HEAD..origin/main --oneline", {
        cwd: coreDir,
        encoding: "utf-8",
      }).trim();

      if (log) {
        info("New commits:");
        for (const line of log.split("\n").slice(0, 10)) {
          info(`  ${line}`);
        }
      }

      const shouldPull = await p.confirm({
        message: "Pull updates?",
        initialValue: true,
      });

      if (!p.isCancel(shouldPull) && shouldPull) {
        const pullSpinner = p.spinner();
        pullSpinner.start("Pulling...");
        execSync("git pull origin main", { cwd: coreDir, stdio: "pipe" });
        pullSpinner.stop("Pulled latest changes");
      }
    } else {
      spinner.stop("Already up to date");
    }

    if (ahead) {
      warn("Local commits not yet pushed to remote");
    }
  } catch {
    spinner.stop("Could not check for updates");
    warn("Git fetch failed — are you online?");
  }

  // --- Step 2: Re-sync symlinks ---
  info("");
  info("Re-syncing symlinks...");
  await setup();

  // --- Step 3: Migrate projects ---
  info("");
  const migrateSpinner = p.spinner();
  migrateSpinner.start("Scanning projects for needed migrations...");

  const projects = findProjects();
  const allChanges: { project: string; changes: string[] }[] = [];

  for (const projectDir of projects) {
    const projectChanges: string[] = [];
    for (const migration of MIGRATIONS) {
      const changes = migration.apply(projectDir);
      projectChanges.push(...changes);
    }
    if (projectChanges.length > 0) {
      allChanges.push({
        project: projectDir.split("/").pop()!,
        changes: projectChanges,
      });
    }
  }

  migrateSpinner.stop("Migration scan complete");

  if (allChanges.length > 0) {
    info("");
    info(color.cyan("Migrations applied:"));
    for (const { project, changes } of allChanges) {
      info(`  ${color.bold(project)}:`);
      for (const change of changes) {
        success(`    ${change}`);
      }
    }
  } else {
    info("All projects up to date — no migrations needed");
  }

  p.outro("Update complete.");
}
