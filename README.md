# LEO — Living Ecosystem Orchestrator

A replicable working method for Claude Code agents. One conversational manager (Leo) coordinates a team of discipline-specific Managers, universal rules, and on-demand specialists — across all your projects. Knowledge lives in a JSON Knowledge Base with neural tag connections.

## How it works

You talk to **Leo** (the Manager of Managers). Leo delegates to the right **Manager** based on the domain — Engineer Manager for code, Designer Manager for design, PM Manager for product, Marketing Manager for content. Managers decompose tasks, delegate to **specialists** (hired on-demand via the Hiring Loop), review the work adversarially, and synthesize back to you.

You decide the **what** and **why**. Leo decides the **who**. Managers decide the **how**. Anything irreversible or structural comes back to you for approval.

```
You (owner)
  └─ Leo (routing, synthesis, propagation)
       ├─ Engineer Manager → specialists (frontend, backend, infra, ...)
       ├─ Designer Manager → specialists (UI, assets, ...)
       ├─ PM Manager → research, domain experts
       └─ Marketing Manager → content, growth specialists
```

## Getting started

### 1. Clone and install

```bash
git clone git@github.com:vmarinogg/leo-core.git ~/Github/leo-core
cd ~/Github/leo-core && ./install.sh
```

This builds the CLI and registers `leo` as a global command.

### 2. Onboard a project

```bash
cd ~/Github/your-project
leo init
```

This automatically syncs the core's agents, rules, skills, and KB to `~/.claude/` and runs the interactive onboarding:
- Scans your codebase and detects the stack (20+ frameworks/tools supported)
- Infers project type and asks for confirmation
- Lets you pick which Managers you need
- Suggests specialists based on your stack
- Generates `.claude/` structure, `CLAUDE.md` bootloader, and KB foundation

If interrupted, run `leo init` again — it picks up where you left off.

### 3. Start working

```bash
claude
```

Leo boots from the KB, loads rules and identity, and is ready. Describe what you need.

## CLI commands

| Command | What it does |
|---------|-------------|
| `leo init` | Interactive project onboarding (auto-syncs core + scan, configure, generate) |
| `leo setup` | Re-sync agents/rules/skills/KB to `~/.claude/` |
| `leo update` | Pull latest core + re-sync + migrate projects |
| `leo status` | Show core and current project state (including KB health) |
| `leo migrate-kb` | Migrate existing project to KB architecture (JSON knowledge base) |

## Knowledge Base (KB)

The KB is the core innovation — a neural network of JSON documents connected by tags. AI thinks, scripts execute.

```
.claude/kb/
├── schema.json       ← JSON Schema for all doc types
├── index.json        ← Neural map (by_tag, by_type, by_scope, by_lifecycle)
├── docs/             ← Flat document store (type lives inside, not in filename)
│   ├── think-before-execute.json    ← type: rule
│   ├── project-identity.json        ← type: identity
│   ├── session-wrap-up.json         ← type: skill
│   └── ...
└── scripts/
    ├── validate.sh   ← Schema validation (zero tokens)
    ├── build-index.sh ← Rebuild neural map (zero tokens)
    └── check-stale.sh ← Detect expired docs (zero tokens)
```

### Token Economy

- **AI spends tokens** on thinking, judgment, content creation
- **Scripts spend zero tokens** on validation, indexing, stale detection
- **Hooks automate** KB maintenance (validate on write, rebuild index on stop)

### Doc types

| Type | Lifecycle | What it stores |
|------|-----------|---------------|
| `rule` | permanent | Operational rules governing agent behavior |
| `skill` | permanent | Executable workflows agents can invoke |
| `identity` | permanent | What the project IS — stack, philosophy, constraints |
| `decision` | learning | Decisions with context, alternatives, impact |
| `pattern` | learning | Reusable conventions and templates |
| `fact` | state | Temporary info that ages fast |
| `feedback` | learning | Owner corrections to agent behavior |
| `reference` | state | Pointers to external resources |
| `metric` | state | Task execution metrics |

## Structure

```
leo-core/
├── CLAUDE.md                          ← Bootloader (~30 lines, teaches agent to self-load)
├── cli/                               ← Node.js CLI (setup, init, update, status, migrate-kb)
├── agents/
│   ├── leo.md                         ← Manager of Managers (model: opus)
│   └── managers/                      ← 4 universal tech leads
├── rules/                             ← 11 universal rules (MD — legacy, migrating to KB)
├── .claude/
│   ├── kb/                            ← Knowledge Base
│   │   ├── schema.json               ← Document schema
│   │   ├── index.json                ← Neural map (auto-generated)
│   │   ├── docs/                     ← 15 JSON documents
│   │   └── scripts/                  ← Zero-token maintenance scripts
│   ├── hooks/                         ← Claude Code hooks (validate, rebuild-index)
│   └── settings.json                 ← Hooks configuration
├── skills/
│   └── session-wrap-up/              ← End-of-session propagation protocol
├── docs/                              ← Design docs, conventions, RDDs
└── install.sh                         ← One-command setup
```

## Key concepts

### Managers are tech leads, not executors

Managers receive tasks, decompose them, delegate to specialists, review adversarially, and synthesize. They write code/design/copy only as an exception.

### Projects extend the core, never override

A project's Manager file uses `extends:` to inherit the core Manager. Core behavior can never be removed — only extended. KB docs use `scope: core` vs `scope: project` for the same principle.

### Review is automatic

Every piece of work goes through adversarial peer review by another instance of the same Manager — with isolated context and no access to the original reasoning.

### Knowledge is alive

The KB grows during sessions (wrap-up creates JSON docs), maintains itself (scripts validate and index), and ages naturally (lifecycle field + stale detection).

## What it is not

- Not an autonomous agent framework — you stay in the loop at every inflection point
- Not a replacement for thinking — it's a structure for how AI agents collaborate
- Not vendor-locked — JSON KB is AI-agnostic by design
