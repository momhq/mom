import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { existsSync, readFileSync, mkdirSync, writeFileSync } from "node:fs";
import { homedir } from "node:os";

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

const CORE_PATH_FILE = resolve(homedir(), ".claude", ".leo-core-path");
// Legacy path file from pre-rename era — kept for backwards compatibility
const LEGACY_PATH_FILE = resolve(homedir(), ".claude", ".copilot-core-path");

/** Save the core directory path to ~/.claude/.leo-core-path */
export function saveCoreDir(dir: string): void {
  const claudeDir = resolve(homedir(), ".claude");
  mkdirSync(claudeDir, { recursive: true });
  writeFileSync(CORE_PATH_FILE, resolve(dir), "utf-8");
}

/** Resolve the leo-core root */
export function getCoreDir(): string {
  // 1. Read from saved config (set at npm link / prepare time)
  for (const pathFile of [CORE_PATH_FILE, LEGACY_PATH_FILE]) {
    if (existsSync(pathFile)) {
      const saved = readFileSync(pathFile, "utf-8").trim();
      if (existsSync(resolve(saved, "agents"))) {
        return saved;
      }
    }
  }

  // 2. Fallback: walk up from binary location (works when running from source)
  let dir = __dirname;
  for (let i = 0; i < 5; i++) {
    const parent = resolve(dir, "..");
    if (
      existsSync(resolve(parent, "agents")) &&
      existsSync(resolve(parent, "rules"))
    ) {
      return parent;
    }
    dir = parent;
  }

  throw new Error(
    "Could not find leo-core directory. Run 'npm link' again from leo-core/cli/."
  );
}

/** ~/.claude/ */
export function getClaudeDir(): string {
  return resolve(homedir(), ".claude");
}

/** Current working directory (the project being onboarded) */
export function getProjectDir(): string {
  return process.cwd();
}
