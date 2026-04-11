import { resolve, dirname } from "node:path";
import { mkdirSync, writeFileSync } from "node:fs";
import { homedir } from "node:os";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const coreDir = resolve(__dirname, "..", "..");
const claudeDir = resolve(homedir(), ".claude");

mkdirSync(claudeDir, { recursive: true });
writeFileSync(resolve(claudeDir, ".leo-core-path"), coreDir, "utf-8");

console.log(`Saved core path: ${coreDir}`);
