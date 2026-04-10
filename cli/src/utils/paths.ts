import { resolve, dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { homedir } from "node:os";

const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

/** Resolve the copilot-core root (three levels up from cli/src/utils/) */
export function getCoreDir(): string {
  return resolve(__dirname, "..", "..", "..");
}

/** ~/.claude/ */
export function getClaudeDir(): string {
  return resolve(homedir(), ".claude");
}

/** Current working directory (the project being onboarded) */
export function getProjectDir(): string {
  return process.cwd();
}
