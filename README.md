# copilot-core

A replicable working method for Claude Code agents. One conversational manager (Leo) coordinates a team of discipline-specific Managers, universal rules, and on-demand specialists — across all your projects.

## How it works

You talk to **Leo** (the Manager of Managers). Leo delegates to the right **Manager** based on the domain — Engineer Manager for code, Designer Manager for design, PM Manager for product, Marketing Manager for content. Managers decompose tasks, delegate to **specialists** (hired on-demand via the Hiring Loop), review the work adversarially, and synthesize back to you.

You decide the **what** and **why**. Leo decides the **who**. Managers decide the **how**. Anything irreversible or structural comes back to you for approval.

```
You (founder)
  └─ Leo (routing, synthesis, propagation)
       ├─ Engineer Manager → specialists (frontend, backend, infra, ...)
       ├─ Designer Manager → specialists (UI, assets, ...)
       ├─ PM Manager → research, domain experts
       └─ Marketing Manager → content, growth specialists
```

## Getting started

### 1. Clone and install

```bash
git clone git@github.com:vmarinogg/copilot-core.git ~/Github/copilot-core
cd ~/Github/copilot-core && ./install.sh
```

This builds the CLI and registers `copilot-core` as a global command.

### 2. Onboard a project

```bash
cd ~/Github/your-project
copilot-core init
```

This automatically syncs the core's agents, rules, and skills to `~/.claude/` and then runs the interactive onboarding.

The interactive onboarding:
- Scans your codebase and detects the stack (20+ frameworks/tools supported)
- Infers project type and asks for confirmation
- Lets you pick which Managers you need
- Suggests specialists based on your stack (scaffolded with empty playbooks — filled on first use)
- Asks for negative constraints (things the agent should NOT do)
- Generates `.claude/` structure, `CLAUDE.md`, and adds `.claude/` to `.gitignore`

If interrupted, run `copilot-core init` again — it picks up where you left off.

### 3. Start working

```bash
claude
```

Leo is ready. Describe what you need, and the system handles routing, delegation, review, and delivery.

## CLI commands

| Command | What it does |
|---------|-------------|
| `copilot-core init` | Interactive project onboarding (auto-syncs core + scan, configure, generate) |
| `copilot-core setup` | Re-sync agents/rules/skills to `~/.claude/` (also runs automatically inside `init`) |
| `copilot-core update` | Pull latest core + re-sync + migrate projects |
| `copilot-core status` | Show core and current project state |

## Updating

```bash
copilot-core update
```

This fetches the latest core, re-syncs symlinks, and automatically migrates any projects under `~/Github/` that need updates (e.g., renamed files, changed references).

Content updates to existing agents/rules propagate instantly via symlinks — no re-run needed. Only run `update` when the core's structure changes (files added/removed/renamed).

## Structure

```
copilot-core/
├── cli/                               ← Node.js CLI (setup, init, update, status)
│   └── src/
│       ├── commands/                  ← CLI command implementations
│       ├── scanners/                  ← Stack detection (20+ frameworks)
│       ├── generators/               ← File generators (CLAUDE.md, managers, context)
│       └── utils/                     ← Paths, UI helpers, state management
├── agents/
│   ├── leo.md                         ← Manager of Managers (model: opus)
│   └── managers/
│       ├── engineer.md                ← engineering tech lead
│       ├── designer.md               ← design tech lead
│       ├── pm.md                      ← product tech lead
│       └── marketing.md              ← marketing tech lead
├── rules/                             ← 11 universal rules
│   ├── think-before-execute.md        ← direct mode vs alignment mode
│   ├── propagation.md                 ← decisions must reach context files
│   ├── anti-hallucination.md          ← mark origin, verify before asserting
│   ├── evidence-over-claim.md         ← deliveries need proof
│   ├── peer-review-automatic.md       ← adversarial review via sub-instances
│   ├── escalation-triggers.md         ← when to stop and ask
│   ├── hiring-loop.md                 ← managers report gaps, Leo hires
│   ├── know-what-you-dont-know.md     ← pre-execution checks + trust gradient
│   ├── inheritance.md                 ← projects extend core via `extends:`
│   ├── state-vs-learning.md           ← memories age differently
│   └── metrics-collection.md          ← 8 metrics per task
├── skills/
│   └── session-wrap-up/               ← end-of-session propagation protocol
├── docs/
│   ├── conventions/                   ← GitHub project management, templates
│   └── rdds/                          ← architectural decision records
├── scripts/
│   └── sync.sh                        ← legacy installer (CLI preferred)
└── install.sh                         ← one-command setup (build + link CLI)
```

## Key concepts

### Managers are tech leads, not executors

Managers receive tasks, decompose them, delegate to specialists, review adversarially, and synthesize. They write code/design/copy only as an exception (micro-tasks, emergency).

### Specialists live in the project, not the core

The core has universal Managers and rules. Each project builds its own specialist team via the Hiring Loop as needed. Specialists are created in `<project>/.claude/specialists/`.

### Projects extend the core, never override

A project's Manager file uses `extends:` to inherit the core Manager and add project-specific context (stack, conventions, extra self-QA items). Core behavior can never be removed — only extended.

```yaml
---
name: Engineer Manager (my-project)
extends: ../../../../.claude/agents/managers/engineer.md
---

## Project-specific additions
- shadcn/ui first — always check component library before creating custom components
```

### Rules in 2 scopes

- **Universal** (`rules/`) — always loaded, govern all agents. "How the team works."
- **Domain** — embedded in Manager files, loaded when the Manager is invoked. "How that discipline works."

### Review is automatic

Every piece of work goes through adversarial peer review by another instance of the same Manager — with isolated context and no access to the original reasoning. The cost is seconds and tokens, not human hours.

### 8 operational metrics

Every task produces a JSONL entry tracking: peer review pass rate, founder acceptance, self-QA honesty, rework cycles, hiring loop hit rate, delegation quality, internal iterations, and Leo's own errors.

## What it is not

- Not an autonomous agent framework — you stay in the loop at every inflection point
- Not a replacement for your project's `CLAUDE.md` — it complements it
- Not a generic CLI for Claude Code — it's a working method with opinions
