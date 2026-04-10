import { existsSync, readFileSync, appendFileSync, writeFileSync } from "node:fs";
import { resolve, basename } from "node:path";
import { getProjectDir } from "../utils/paths.js";
import { header, success, warn, info, p, color } from "../utils/ui.js";
import {
  detectStack,
  suggestSpecialists,
  detectPackageManager,
  isMonorepo,
  inferProjectType,
  detectExistingState,
  detectTestFramework,
  detectDesignSystem,
  detectCI,
} from "../scanners/stack.js";
import type { SuggestedSpecialist, DetectedStack } from "../scanners/stack.js";
import { writeManagerFiles } from "../generators/managers.js";
import type { ManagerType } from "../generators/managers.js";
import {
  generateProjectMd,
  generateStackMd,
  generateConstraintsMd,
  writeContextFiles,
  writeSpecialistScaffolds,
} from "../generators/context.js";
import { generateClaudeMd } from "../generators/claude-md.js";
import { saveInitState, loadInitState, clearInitState } from "../utils/state.js";
import type { InitState } from "../utils/state.js";

// ─── Helpers ────────────────────────────────────────────────────────────────

function statusIcon(done: boolean): string {
  return done ? color.green("✅") : color.dim("◻️");
}

function showExistingStatus(
  projectDir: string,
  stacks: DetectedStack[]
): void {
  const state = detectExistingState(projectDir);
  const testFramework = detectTestFramework(stacks);
  const designSystem = detectDesignSystem(stacks);
  const ci = detectCI(stacks);

  p.log.step(color.cyan("Current project state:"));

  // Stack
  if (stacks.length > 0) {
    const stackNames = stacks.map((s) => s.name).join(", ");
    info(`${statusIcon(true)} Stack detected: ${color.cyan(stackNames)}`);
  } else {
    info(`${statusIcon(false)} Stack: not detected`);
  }

  // Existing copilot-core structures
  if (state.hasClaudeDir) {
    info(
      `${statusIcon(state.managers.length > 0)} Managers: ${
        state.managers.length > 0
          ? state.managers.join(", ")
          : "none"
      }`
    );
    info(
      `${statusIcon(state.specialists.length > 0)} Specialists: ${
        state.specialists.length > 0
          ? `${state.specialists.length} found`
          : "none"
      }`
    );
    info(`${statusIcon(state.hasProjectMd)} context/project.md`);
    info(`${statusIcon(state.hasStackMd)} context/stack.md`);
    info(`${statusIcon(state.hasConstraintsMd)} context/constraints.md`);
  } else {
    info(`${statusIcon(false)} .claude/ directory: not created yet`);
  }

  info(`${statusIcon(state.hasClaudeMd)} CLAUDE.md`);
  info(`${statusIcon(state.hasGitignoreEntry)} .gitignore (.claude/ entry)`);

  // Extra detections
  if (testFramework) info(`${statusIcon(true)} Test framework: ${color.cyan(testFramework)}`);
  if (designSystem) info(`${statusIcon(true)} Design system: ${color.cyan(designSystem)}`);
  if (ci) info(`${statusIcon(true)} CI/CD: ${color.cyan(ci)}`);

  info("");
}

// ─── Main ───────────────────────────────────────────────────────────────────

export async function init() {
  header("copilot-core · project onboarding");

  const projectDir = getProjectDir();
  const projectName = basename(projectDir);

  // ── Phase 0: Recall ────────────────────────────────────────────────────

  const savedState = loadInitState(projectDir);

  if (savedState && savedState.phase !== "done") {
    info(`Found an interrupted init session from ${savedState.timestamp.split("T")[0]}.`);
    info(`Progress: reached the ${color.cyan(savedState.phase)} phase.`);
    info("");

    const resumeChoice = await p.select({
      message: "What would you like to do?",
      options: [
        { value: "resume", label: "Resume where I left off" },
        { value: "fresh", label: "Start fresh" },
        { value: "abort", label: "Cancel" },
      ],
    });

    if (p.isCancel(resumeChoice) || resumeChoice === "abort") {
      p.cancel("Aborted.");
      process.exit(0);
    }

    if (resumeChoice === "fresh") {
      clearInitState(projectDir);
    }
  }

  const state: InitState = (savedState?.phase !== "done" && savedState)
    ? savedState
    : {
        phase: "scan",
        projectDir,
        projectName,
        timestamp: new Date().toISOString(),
      };

  // ── Phase 1: Scan & Status ─────────────────────────────────────────────

  const spinner = p.spinner();
  spinner.start("Scanning codebase...");

  const stacks = detectStack(projectDir);
  const packageManager = detectPackageManager(projectDir);
  const mono = isMonorepo(projectDir);

  spinner.stop("Scan complete");

  // Status-first: show what exists before asking anything
  showExistingStatus(projectDir, stacks);

  if (mono) info(`Monorepo: ${color.cyan("yes")}`);
  info(`Package manager: ${color.cyan(packageManager)}`);
  info("");

  // Check if project is already fully set up
  const existing = detectExistingState(projectDir);
  if (
    existing.hasClaudeDir &&
    existing.managers.length > 0 &&
    existing.hasClaudeMd &&
    existing.hasProjectMd
  ) {
    const continueChoice = await p.select({
      message: "This project is already set up. What would you like to do?",
      options: [
        { value: "gaps", label: "Fill the gaps (only create what's missing)" },
        { value: "fresh", label: "Start fresh (overwrite everything)" },
        { value: "abort", label: "Cancel" },
      ],
    });

    if (p.isCancel(continueChoice) || continueChoice === "abort") {
      p.cancel("Aborted.");
      process.exit(0);
    }

    if (continueChoice === "fresh") {
      info("Starting fresh — existing files will be overwritten where needed.");
    }
    // "gaps" mode: the generators already skip existing files
  }

  // ── Phase 2: Project type (contextual confirmation) ────────────────────

  let projectType: string;

  if (state.phase !== "scan" && state.projectType) {
    projectType = state.projectType;
  } else {
    const inferred = inferProjectType(stacks);

    if (inferred) {
      const typeLabels: Record<string, string> = {
        "web-app": "web app",
        "mobile-app": "mobile app",
        "fullstack": "fullstack app (web + mobile)",
        "api": "API / backend service",
        "cli-tool": "CLI tool",
        "library": "library / package",
      };

      const confirm = await p.confirm({
        message: `Based on your code, this looks like a ${color.cyan(typeLabels[inferred] || inferred)}. Is that right?`,
        initialValue: true,
      });

      if (p.isCancel(confirm)) {
        saveInitState({ ...state, phase: "type" });
        p.cancel("Progress saved. Run 'copilot-core init' to resume.");
        process.exit(0);
      }

      if (confirm) {
        projectType = inferred;
      } else {
        const selected = await p.select({
          message: "What type of project is this?",
          options: [
            { value: "web-app", label: "Web app" },
            { value: "mobile-app", label: "Mobile app (Capacitor/React Native/Flutter)" },
            { value: "fullstack", label: "Fullstack (web + mobile)" },
            { value: "cli-tool", label: "CLI tool" },
            { value: "library", label: "Library / package" },
            { value: "api", label: "API / backend service" },
            { value: "other", label: "Other" },
          ],
        });
        if (p.isCancel(selected)) {
          saveInitState({ ...state, phase: "type" });
          p.cancel("Progress saved. Run 'copilot-core init' to resume.");
          process.exit(0);
        }
        projectType = selected as string;
      }
    } else {
      const selected = await p.select({
        message: "I couldn't infer the project type from the code. What is it?",
        options: [
          { value: "web-app", label: "Web app" },
          { value: "mobile-app", label: "Mobile app (Capacitor/React Native/Flutter)" },
          { value: "fullstack", label: "Fullstack (web + mobile)" },
          { value: "cli-tool", label: "CLI tool" },
          { value: "library", label: "Library / package" },
          { value: "api", label: "API / backend service" },
          { value: "other", label: "Other" },
        ],
      });
      if (p.isCancel(selected)) {
        saveInitState({ ...state, phase: "type" });
        p.cancel("Progress saved. Run 'copilot-core init' to resume.");
        process.exit(0);
      }
      projectType = selected as string;
    }

    state.projectType = projectType;
    state.phase = "description";
    saveInitState(state);
  }

  // ── Phase 3: Description ───────────────────────────────────────────────

  let description: string;

  if (state.description) {
    description = state.description;
  } else {
    // Try to read from existing README or package.json
    let placeholder = "What does this project do?";
    const pkgPath = resolve(projectDir, "package.json");
    if (existsSync(pkgPath)) {
      try {
        const pkg = JSON.parse(readFileSync(pkgPath, "utf-8"));
        if (pkg.description) placeholder = pkg.description;
      } catch { /* ignore */ }
    }

    const desc = await p.text({
      message: "Project description (1-2 sentences):",
      placeholder,
      initialValue: placeholder !== "What does this project do?" ? placeholder : undefined,
      validate: (value) => {
        if (!value.trim()) return "Description is required";
      },
    });

    if (p.isCancel(desc)) {
      saveInitState({ ...state, phase: "description" });
      p.cancel("Progress saved. Run 'copilot-core init' to resume.");
      process.exit(0);
    }

    description = desc as string;
    state.description = description;
    state.phase = "managers";
    saveInitState(state);
  }

  // ── Phase 4: Managers (conditional — skip if detected) ─────────────────

  let selectedManagers: ManagerType[];

  if (state.managers) {
    selectedManagers = state.managers;
  } else {
    // Smart defaults based on project type
    const defaults: ManagerType[] = ["engineer"];

    if (["web-app", "mobile-app", "fullstack"].includes(projectType)) {
      defaults.push("designer");
    }
    defaults.push("pm");

    // If project already has managers, pre-select those
    if (existing.managers.length > 0) {
      info(`You already have managers: ${color.cyan(existing.managers.join(", "))}`);

      const keepExisting = await p.confirm({
        message: "Keep existing managers and add any missing ones?",
        initialValue: true,
      });

      if (p.isCancel(keepExisting)) {
        saveInitState({ ...state, phase: "managers" });
        p.cancel("Progress saved. Run 'copilot-core init' to resume.");
        process.exit(0);
      }

      if (keepExisting) {
        // Only ask about managers not yet present
        const missing = (["engineer", "designer", "pm", "marketing"] as ManagerType[]).filter(
          (m) => !existing.managers.includes(m)
        );

        if (missing.length > 0) {
          const toAdd = await p.multiselect({
            message: "Add any of these missing managers?",
            options: missing.map((m) => ({
              value: m,
              label: `${m.charAt(0).toUpperCase() + m.slice(1)} Manager`,
              hint: m === "engineer" ? "engineering tech lead" : m === "designer" ? "design tech lead" : m === "pm" ? "product tech lead" : "marketing tech lead",
            })),
            required: false,
          });

          if (p.isCancel(toAdd)) {
            saveInitState({ ...state, phase: "managers" });
            p.cancel("Progress saved. Run 'copilot-core init' to resume.");
            process.exit(0);
          }

          selectedManagers = [...existing.managers, ...(toAdd as ManagerType[])] as ManagerType[];
        } else {
          selectedManagers = existing.managers as ManagerType[];
        }
      } else {
        // Fresh selection
        const selected = await p.multiselect({
          message: "Which managers do you need?",
          options: [
            { value: "engineer" as ManagerType, label: "Engineer Manager", hint: "engineering tech lead" },
            { value: "designer" as ManagerType, label: "Designer Manager", hint: "design tech lead" },
            { value: "pm" as ManagerType, label: "PM Manager", hint: "product tech lead" },
            { value: "marketing" as ManagerType, label: "Marketing Manager", hint: "marketing tech lead" },
          ],
          initialValues: defaults,
          required: true,
        });

        if (p.isCancel(selected)) {
          saveInitState({ ...state, phase: "managers" });
          p.cancel("Progress saved. Run 'copilot-core init' to resume.");
          process.exit(0);
        }

        selectedManagers = selected as ManagerType[];
      }
    } else {
      const selected = await p.multiselect({
        message: "Which managers do you need?",
        options: [
          { value: "engineer" as ManagerType, label: "Engineer Manager", hint: "engineering tech lead" },
          { value: "designer" as ManagerType, label: "Designer Manager", hint: "design tech lead" },
          { value: "pm" as ManagerType, label: "PM Manager", hint: "product tech lead" },
          { value: "marketing" as ManagerType, label: "Marketing Manager", hint: "marketing tech lead" },
        ],
        initialValues: defaults,
        required: true,
      });

      if (p.isCancel(selected)) {
        saveInitState({ ...state, phase: "managers" });
        p.cancel("Progress saved. Run 'copilot-core init' to resume.");
        process.exit(0);
      }

      selectedManagers = selected as ManagerType[];
    }

    state.managers = selectedManagers;
    state.phase = "specialists";
    saveInitState(state);
  }

  // ── Phase 5: Specialists (contextual — only suggest what's missing) ────

  let selectedSpecialists: SuggestedSpecialist[];

  if (state.specialists) {
    selectedSpecialists = state.specialists;
  } else {
    const allSuggestions = suggestSpecialists(projectDir);

    // Filter out specialists that already exist
    const existingNames = new Set(
      existing.specialists.map((s) => s.split("/").pop())
    );
    const newSuggestions = allSuggestions.filter(
      (s) => !existingNames.has(s.name)
    );

    if (existing.specialists.length > 0) {
      info(
        `You already have ${existing.specialists.length} specialist(s): ${color.cyan(existing.specialists.join(", "))}`
      );
    }

    if (newSuggestions.length > 0) {
      const specialistChoices = await p.multiselect({
        message:
          existing.specialists.length > 0
            ? "I found new specialists to suggest based on your stack:"
            : "Based on your stack, I suggest these specialists:",
        options: newSuggestions.map((s) => ({
          value: s.name,
          label: s.name,
          hint: s.reason,
        })),
        initialValues: newSuggestions.map((s) => s.name),
        required: false,
      });

      if (p.isCancel(specialistChoices)) {
        saveInitState({ ...state, phase: "specialists" });
        p.cancel("Progress saved. Run 'copilot-core init' to resume.");
        process.exit(0);
      }

      selectedSpecialists = newSuggestions.filter((s) =>
        (specialistChoices as string[]).includes(s.name)
      );
    } else if (allSuggestions.length === 0) {
      info("No specialists suggested based on detected stack. Add them later via Hiring Loop.");
      selectedSpecialists = [];
    } else {
      info("All suggested specialists already exist. No new ones to add.");
      selectedSpecialists = [];
    }

    state.specialists = selectedSpecialists;
    state.phase = "constraints";
    saveInitState(state);
  }

  // ── Phase 6: Negative constraints (only if not already present) ────────

  let constraints: string | undefined;

  if (state.constraints !== undefined) {
    constraints = state.constraints || undefined;
  } else if (existing.hasConstraintsMd) {
    info("constraints.md already exists — skipping.");
    constraints = undefined;
  } else {
    const input = await p.text({
      message: "Any negative constraints? (things the agent should NOT do)",
      placeholder:
        "e.g. Don't use CSS modules, don't add Redux, don't create API routes without RLS",
    });

    if (p.isCancel(input)) {
      saveInitState({ ...state, phase: "constraints" });
      p.cancel("Progress saved. Run 'copilot-core init' to resume.");
      process.exit(0);
    }

    constraints = (input as string) || undefined;
    state.constraints = constraints || "";
    state.phase = "generate";
    saveInitState(state);
  }

  // ── Phase 7: Generate files ────────────────────────────────────────────

  p.log.step(color.cyan("Creating project structure..."));

  // Manager files
  const managerFiles = writeManagerFiles(
    projectDir,
    selectedManagers,
    projectName
  );
  for (const f of managerFiles) {
    success(f);
  }
  if (managerFiles.length === 0 && selectedManagers.length > 0) {
    info("All manager files already exist — skipped.");
  }

  // Context files
  const contextFiles = writeContextFiles(projectDir, {
    projectMd: generateProjectMd(projectName, description, projectType),
    stackMd: generateStackMd(stacks, packageManager),
    constraintsMd: constraints
      ? generateConstraintsMd(constraints)
      : undefined,
  });
  for (const f of contextFiles) {
    success(f);
  }
  if (contextFiles.length === 0) {
    info("All context files already exist — skipped.");
  }

  // Specialist scaffolds
  if (selectedSpecialists.length > 0) {
    const specialistFiles = writeSpecialistScaffolds(
      projectDir,
      selectedSpecialists
    );
    for (const f of specialistFiles) {
      success(f);
    }
  }

  // CLAUDE.md
  const claudeMdPath = resolve(projectDir, "CLAUDE.md");
  const claudeContent = generateClaudeMd({
    projectName,
    description,
    managers: selectedManagers,
    specialists: selectedSpecialists,
    constraints,
  });

  if (existsSync(claudeMdPath)) {
    const overwrite = await p.confirm({
      message: "CLAUDE.md already exists. Overwrite?",
      initialValue: false,
    });
    if (!p.isCancel(overwrite) && overwrite) {
      writeFileSync(claudeMdPath, claudeContent);
      success("CLAUDE.md (overwritten)");
    } else {
      info("CLAUDE.md skipped (already exists).");
    }
  } else {
    writeFileSync(claudeMdPath, claudeContent);
    success("CLAUDE.md");
  }

  // .gitignore
  const gitignorePath = resolve(projectDir, ".gitignore");
  const copilotIgnoreBlock = `\n# copilot-core (local agent infrastructure)\n.claude/\n`;

  if (existsSync(gitignorePath)) {
    const content = readFileSync(gitignorePath, "utf-8");
    if (!content.includes(".claude/")) {
      appendFileSync(gitignorePath, copilotIgnoreBlock);
      success(".gitignore updated (added .claude/)");
    }
  } else {
    appendFileSync(gitignorePath, copilotIgnoreBlock);
    success(".gitignore created (with .claude/)");
  }

  // Clean up init state
  clearInitState(projectDir);
  state.phase = "done";

  // ── Summary ────────────────────────────────────────────────────────────

  info("");
  showExistingStatus(projectDir, stacks);

  p.outro(
    `Done! Your project is ready for Claude Code. Run ${color.cyan("claude")} to start.`
  );
}
