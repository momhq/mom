import { existsSync, readFileSync, writeFileSync, mkdirSync, unlinkSync } from "node:fs";
import { resolve } from "node:path";
import type { ManagerType } from "../generators/managers.js";
import type { SuggestedSpecialist } from "../scanners/stack.js";

export interface InitState {
  phase: "scan" | "type" | "description" | "managers" | "specialists" | "constraints" | "generate" | "done";
  projectDir: string;
  projectName: string;
  projectType?: string;
  description?: string;
  managers?: ManagerType[];
  specialists?: SuggestedSpecialist[];
  constraints?: string;
  timestamp: string;
}

function getStatePath(projectDir: string): string {
  return resolve(projectDir, ".claude", ".init-state.json");
}

export function saveInitState(state: InitState): void {
  const statePath = getStatePath(state.projectDir);
  mkdirSync(resolve(state.projectDir, ".claude"), { recursive: true });
  writeFileSync(statePath, JSON.stringify(state, null, 2));
}

export function loadInitState(projectDir: string): InitState | null {
  const statePath = getStatePath(projectDir);
  if (!existsSync(statePath)) return null;

  try {
    const content = readFileSync(statePath, "utf-8");
    const state = JSON.parse(content) as InitState;

    // Check if state is stale (older than 24h)
    const age = Date.now() - new Date(state.timestamp).getTime();
    if (age > 24 * 60 * 60 * 1000) return null;

    return state;
  } catch {
    return null;
  }
}

export function clearInitState(projectDir: string): void {
  const statePath = getStatePath(projectDir);
  if (existsSync(statePath)) {
    try {
      unlinkSync(statePath);
    } catch { /* ignore */ }
  }
}
