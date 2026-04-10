import { existsSync, readFileSync, appendFileSync } from "node:fs";
import { resolve, basename } from "node:path";
import { getProjectDir, getCoreDir } from "../utils/paths.js";
import { header, success, warn, error, info, p, color } from "../utils/ui.js";
import {
  detectStack,
  suggestSpecialists,
  detectPackageManager,
  isMonorepo,
} from "../scanners/stack.js";
import type { SuggestedSpecialist } from "../scanners/stack.js";
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
import { writeFileSync } from "node:fs";

export async function init() {
  header("copilot-core · project onboarding");

  const projectDir = getProjectDir();
  const coreDir = getCoreDir();
  const projectName = basename(projectDir);

  // Check if already initialized
  if (existsSync(resolve(projectDir, ".claude", "agents", "managers"))) {
    const shouldContinue = await p.confirm({
      message: "This project already has .claude/agents/managers/. Continue and overwrite?",
      initialValue: false,
    });
    if (p.isCancel(shouldContinue) || !shouldContinue) {
      p.cancel("Aborted.");
      process.exit(0);
    }
  }

  // --- Step 1: Scan codebase ---
  const spinner = p.spinner();
  spinner.start("Scanning codebase...");

  const stacks = detectStack(projectDir);
  const packageManager = detectPackageManager(projectDir);
  const mono = isMonorepo(projectDir);

  spinner.stop("Codebase scanned");

  if (stacks.length > 0) {
    const stackNames = stacks.map((s) => s.name).join(", ");
    info(`Detected: ${color.cyan(stackNames)}`);
    info(`Package manager: ${color.cyan(packageManager)}`);
    if (mono) info(`Monorepo: ${color.cyan("yes")}`);
  } else {
    warn("No known stack detected. You can configure manually later.");
  }

  // --- Step 2: Project type ---
  const projectType = await p.select({
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

  if (p.isCancel(projectType)) {
    p.cancel("Aborted.");
    process.exit(0);
  }

  // --- Step 3: Project description ---
  const description = await p.text({
    message: "Project description (1-2 sentences):",
    placeholder: "What does this project do?",
    validate: (value) => {
      if (!value.trim()) return "Description is required";
    },
  });

  if (p.isCancel(description)) {
    p.cancel("Aborted.");
    process.exit(0);
  }

  // --- Step 4: Select managers ---
  const defaultManagers: ManagerType[] = ["engineer"];
  if (
    projectType === "web-app" ||
    projectType === "mobile-app" ||
    projectType === "fullstack"
  ) {
    defaultManagers.push("designer");
  }
  defaultManagers.push("pm");

  const selectedManagers = await p.multiselect({
    message: "Which managers do you need?",
    options: [
      { value: "engineer" as ManagerType, label: "Engineer Manager", hint: "development tech lead" },
      { value: "designer" as ManagerType, label: "Designer Manager", hint: "design tech lead" },
      { value: "pm" as ManagerType, label: "PM Manager", hint: "product tech lead" },
      { value: "marketing" as ManagerType, label: "Marketing Manager", hint: "marketing tech lead" },
    ],
    initialValues: defaultManagers,
    required: true,
  });

  if (p.isCancel(selectedManagers)) {
    p.cancel("Aborted.");
    process.exit(0);
  }

  // --- Step 5: Suggest specialists ---
  const allSuggestions = suggestSpecialists(projectDir);
  let selectedSpecialists: SuggestedSpecialist[] = [];

  if (allSuggestions.length > 0) {
    const specialistChoices = await p.multiselect({
      message: "Based on your stack, I suggest these specialists:",
      options: allSuggestions.map((s) => ({
        value: s.name,
        label: s.name,
        hint: s.reason,
      })),
      initialValues: allSuggestions.map((s) => s.name),
      required: false,
    });

    if (p.isCancel(specialistChoices)) {
      p.cancel("Aborted.");
      process.exit(0);
    }

    selectedSpecialists = allSuggestions.filter((s) =>
      (specialistChoices as string[]).includes(s.name)
    );
  } else {
    info("No specialists suggested based on detected stack. You can add them later via Hiring Loop.");
  }

  // --- Step 6: Negative constraints ---
  const constraints = await p.text({
    message: "Any negative constraints? (things the agent should NOT do)",
    placeholder: "e.g. Don't use CSS modules, don't add Redux, don't create API routes without RLS",
  });

  if (p.isCancel(constraints)) {
    p.cancel("Aborted.");
    process.exit(0);
  }

  // --- Step 7: Generate files ---
  p.log.step(color.cyan("Creating project structure..."));

  // Manager files
  const managerFiles = writeManagerFiles(
    projectDir,
    selectedManagers as ManagerType[],
    projectName
  );
  for (const f of managerFiles) {
    success(f);
  }

  // Context files
  const contextFiles = writeContextFiles(projectDir, {
    projectMd: generateProjectMd(
      projectName,
      description as string,
      projectType as string
    ),
    stackMd: generateStackMd(stacks, packageManager),
    constraintsMd: constraints
      ? generateConstraintsMd(constraints as string)
      : undefined,
  });
  for (const f of contextFiles) {
    success(f);
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
    description: description as string,
    managers: selectedManagers as ManagerType[],
    specialists: selectedSpecialists,
    constraints: constraints as string | undefined,
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
      warn("CLAUDE.md skipped (already exists)");
    }
  } else {
    writeFileSync(claudeMdPath, claudeContent);
    success("CLAUDE.md");
  }

  // --- Step 8: Add .claude/ to .gitignore ---
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

  // --- Done ---
  p.outro(
    `Done! Your project is ready for Claude Code. Run ${color.cyan("claude")} to start.`
  );
}
