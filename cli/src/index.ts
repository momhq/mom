#!/usr/bin/env node

import { Command } from "commander";
import { setup } from "./commands/setup.js";
import { init } from "./commands/init.js";
import { update } from "./commands/update.js";
import { status } from "./commands/status.js";
import { migrateKb } from "./commands/migrate-kb.js";

const program = new Command();

program
  .name("leo")
  .description("LEO — Living Ecosystem Orchestrator. Setup, onboard projects, and manage your AI agent system.")
  .version("0.2.0");

program
  .command("setup")
  .description("Install leo-core (symlink agents, rules, skills, KB to ~/.claude/)")
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

program
  .command("migrate-kb")
  .description("Migrate current project to KB architecture (JSON knowledge base)")
  .action(migrateKb);

program.parse();
