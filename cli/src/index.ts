#!/usr/bin/env node

import { Command } from "commander";
import { setup } from "./commands/setup.js";
import { init } from "./commands/init.js";
import { update } from "./commands/update.js";
import { status } from "./commands/status.js";

const program = new Command();

program
  .name("copilot-core")
  .description("Setup, onboard projects, and manage your AI agent system")
  .version("0.1.0");

program
  .command("setup")
  .description("Install copilot-core (symlink agents, rules, skills to ~/.claude/)")
  .action(setup);

program
  .command("init")
  .description("Onboard the current project with interactive setup")
  .action(init);

program
  .command("update")
  .description("Check for updates and re-sync core files")
  .action(update);

program
  .command("status")
  .description("Show current core and project status")
  .action(status);

program.parse();
