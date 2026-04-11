import { existsSync, readdirSync, readFileSync, writeFileSync } from "node:fs";
import { resolve, basename } from "node:path";
import { homedir } from "node:os";
import { getProjectDir } from "../utils/paths.js";
import { header, success, warn, info, p, color } from "../utils/ui.js";
import {
  createKbStructure,
  setupKbHooks,
  generateIdentityDoc,
  convertDecisionMd,
  parseProjectMd,
  parseStackMd,
  writeKbDoc,
  rebuildIndex,
} from "../generators/kb.js";
import { generateBootloaderClaudeMd } from "../generators/claude-md.js";

// ─── Helpers ─────────────────────────────────────────────────────────────────

function detectKbState(projectDir: string): {
  hasKb: boolean;
  hasOldContext: boolean;
  hasOldDecisions: boolean;
  hasOldProjectMd: boolean;
  hasOldStackMd: boolean;
  hasOldConstraintsMd: boolean;
  hasClaudeMd: boolean;
  kbDocCount: number;
  decisionFiles: string[];
} {
  const kbDir = resolve(projectDir, ".claude", "kb");
  const docsDir = resolve(kbDir, "docs");
  const contextDir = resolve(projectDir, ".claude", "context");
  const decisionsDir = resolve(contextDir, "decisions");

  let kbDocCount = 0;
  if (existsSync(docsDir)) {
    kbDocCount = readdirSync(docsDir).filter((f) => f.endsWith(".json")).length;
  }

  let decisionFiles: string[] = [];
  if (existsSync(decisionsDir)) {
    decisionFiles = readdirSync(decisionsDir)
      .filter((f) => f.endsWith(".md"))
      .map((f) => resolve(decisionsDir, f));
  }

  return {
    hasKb: existsSync(kbDir) && existsSync(resolve(kbDir, "schema.json")),
    hasOldContext: existsSync(contextDir),
    hasOldDecisions: decisionFiles.length > 0,
    hasOldProjectMd: existsSync(resolve(contextDir, "project.md")),
    hasOldStackMd: existsSync(resolve(contextDir, "stack.md")),
    hasOldConstraintsMd: existsSync(resolve(contextDir, "constraints.md")),
    hasClaudeMd: existsSync(resolve(projectDir, "CLAUDE.md")),
    kbDocCount,
    decisionFiles,
  };
}

// ─── Main ────────────────────────────────────────────────────────────────────

export async function migrateKb() {
  header("leo · KB migration");

  const projectDir = getProjectDir();
  const projectName = basename(projectDir);

  // ── Step 1: Assess current state ─────────────────────────────────────────

  const state = detectKbState(projectDir);

  info(color.cyan("Current project state:"));
  info(`  KB structure: ${state.hasKb ? color.green("exists") : color.dim("not created")}`);
  if (state.hasKb) {
    info(`  KB docs: ${color.cyan(String(state.kbDocCount))}`);
  }
  info(`  Old context/: ${state.hasOldContext ? color.yellow("exists") : color.dim("none")}`);
  info(`  Old decisions: ${state.hasOldDecisions ? color.yellow(`${state.decisionFiles.length} file(s)`) : color.dim("none")}`);
  info(`  CLAUDE.md: ${state.hasClaudeMd ? color.green("exists") : color.dim("not created")}`);
  info("");

  if (state.hasKb && state.kbDocCount > 0) {
    const choice = await p.select({
      message: "KB already exists. What would you like to do?",
      options: [
        { value: "update", label: "Update schema/scripts from core (keep existing docs)" },
        { value: "full", label: "Full migration (may overwrite existing docs)" },
        { value: "abort", label: "Cancel" },
      ],
    });

    if (p.isCancel(choice) || choice === "abort") {
      p.cancel("Aborted.");
      process.exit(0);
    }

    if (choice === "update") {
      // Just update schema and scripts
      const spinner = p.spinner();
      spinner.start("Updating schema and scripts from core...");
      const structureFiles = createKbStructure(projectDir);
      const hookFiles = setupKbHooks(projectDir);
      spinner.stop("Updated");

      for (const f of [...structureFiles, ...hookFiles]) {
        success(f);
      }

      // Rebuild index
      info("");
      info("Rebuilding index...");
      if (rebuildIndex(projectDir)) {
        success("index.json rebuilt");
      }

      p.outro("Update complete.");
      return;
    }
  }

  // ── Step 2: Confirm migration ────────────────────────────────────────────

  const proceed = await p.confirm({
    message: `Migrate ${color.cyan(projectName)} to KB architecture?`,
    initialValue: true,
  });

  if (p.isCancel(proceed) || !proceed) {
    p.cancel("Aborted.");
    process.exit(0);
  }

  // ── Step 3: Create KB structure ──────────────────────────────────────────

  const spinner = p.spinner();
  spinner.start("Creating KB structure...");

  const structureFiles = createKbStructure(projectDir);
  spinner.stop("KB structure created");

  for (const f of structureFiles) {
    success(f);
  }

  // ── Step 4: Setup hooks ──────────────────────────────────────────────────

  info("");
  const hookSpinner = p.spinner();
  hookSpinner.start("Setting up hooks...");

  const hookFiles = setupKbHooks(projectDir);
  hookSpinner.stop("Hooks configured");

  for (const f of hookFiles) {
    success(f);
  }

  // ── Step 5: Convert existing context to KB docs ──────────────────────────

  info("");
  const docsCreated: string[] = [];
  const contextDir = resolve(projectDir, ".claude", "context");

  // Identity doc from project.md + stack.md
  if (state.hasOldProjectMd || state.hasOldStackMd) {
    info(color.cyan("Converting context files to KB docs..."));

    const projectMdPath = resolve(contextDir, "project.md");
    const stackMdPath = resolve(contextDir, "stack.md");
    const constraintsMdPath = resolve(contextDir, "constraints.md");

    const projectInfo = parseProjectMd(projectMdPath);
    const stacks = parseStackMd(stackMdPath);

    let constraints: string | undefined;
    if (state.hasOldConstraintsMd) {
      constraints = readFileSync(constraintsMdPath, "utf-8");
    }

    const identityDoc = generateIdentityDoc({
      projectName,
      description: projectInfo.description || projectName,
      projectType: projectInfo.projectType || "unknown",
      stacks,
      constraints,
    });

    const path = writeKbDoc(projectDir, identityDoc);
    docsCreated.push(path);
    success(`${path} (from project.md + stack.md)`);
  }

  // Decision docs from context/decisions/*.md
  if (state.hasOldDecisions) {
    info(color.cyan("Converting decision files..."));

    for (const decisionFile of state.decisionFiles) {
      const doc = convertDecisionMd(decisionFile);
      if (doc) {
        const path = writeKbDoc(projectDir, doc);
        docsCreated.push(path);
        success(`${path} (from ${basename(decisionFile)})`);
      } else {
        warn(`Could not convert ${basename(decisionFile)}`);
      }
    }
  }

  if (docsCreated.length === 0) {
    info("No existing context files to convert — KB starts empty.");
    info("Leo will populate it during session wrap-ups.");
  }

  // ── Step 6: Generate bootloader CLAUDE.md ────────────────────────────────

  info("");

  const claudeMdPath = resolve(projectDir, "CLAUDE.md");
  const bootloaderContent = generateBootloaderClaudeMd(projectName);

  if (state.hasClaudeMd) {
    const overwrite = await p.confirm({
      message: "CLAUDE.md already exists. Replace with KB bootloader?",
      initialValue: true,
    });

    if (!p.isCancel(overwrite) && overwrite) {
      writeFileSync(claudeMdPath, bootloaderContent);
      success("CLAUDE.md (replaced with KB bootloader)");
    } else {
      info("CLAUDE.md unchanged.");
    }
  } else {
    writeFileSync(claudeMdPath, bootloaderContent);
    success("CLAUDE.md (KB bootloader created)");
  }

  // ── Step 7: Rebuild index ────────────────────────────────────────────────

  info("");
  const indexSpinner = p.spinner();
  indexSpinner.start("Building index...");

  if (rebuildIndex(projectDir)) {
    indexSpinner.stop("Index built");

    // Show index stats
    const indexPath = resolve(projectDir, ".claude", "kb", "index.json");
    if (existsSync(indexPath)) {
      try {
        const index = JSON.parse(readFileSync(indexPath, "utf-8"));
        info(`  ${color.cyan(String(index.stats.total_docs))} docs, ${color.cyan(String(index.stats.total_tags))} tags`);
      } catch { /* ignore */ }
    }
  } else {
    indexSpinner.stop("Index build failed");
    warn("Could not build index — run .claude/kb/scripts/build-index.sh manually");
  }

  // ── Summary ──────────────────────────────────────────────────────────────

  info("");
  info(color.cyan("Migration summary:"));
  info(`  KB structure: ${color.green("created")}`);
  info(`  Schema + scripts: ${color.green("copied from core")}`);
  info(`  Hooks: ${color.green("configured")}`);
  info(`  KB docs created: ${color.green(String(docsCreated.length))}`);
  info(`  CLAUDE.md: ${color.green("bootloader")}`);
  info("");
  info(color.dim("Old context/ files are preserved as fallback."));
  info(color.dim("Remove them manually once you've validated the KB works."));

  p.outro(`${color.cyan(projectName)} migrated to KB architecture. Run ${color.cyan("claude")} to test.`);
}
