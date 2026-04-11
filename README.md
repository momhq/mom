# L.E.O.
### Living Ecosystem Orchestrator

> Knowledge isn't documented. It's operated.

A living knowledge infrastructure where humans and agents think, decide, and evolve together.

---

## What it is

L.E.O. is a multi-agent system core that manages all your projects. Not a product — the definition of a working method that downstream projects inherit, extend, and operate.

The knowledge base is a neural network of JSON documents connected by tags, consumed by agents, maintained by scripts. Knowledge lives as structured data. Human-readable outputs are generated on demand.

This repo is the core. Projects are the edges.

---

## How it works

You talk to L.E.O. L.E.O. routes to the right Manager. Managers decompose, delegate to specialists, review adversarially, and synthesize back to you.

```
You (owner)
  └─ L.E.O. (routing, synthesis, propagation)
       ├─ Engineer Manager → specialists (frontend, backend, infra, ...)
       ├─ Designer Manager → specialists (UI, assets, ...)
       ├─ PM Manager → research, domain experts
       └─ Marketing Manager → content, growth specialists
```

**You decide the what and why.**
**L.E.O. decides the who.**
**Managers decide the how.**

Anything irreversible or structural comes back to you for approval.

---

## Core principles

**Operate, don't document.**
The KB is not a wiki. Every node is a structured JSON file consumed by agents at runtime. Knowledge that isn't used gets flagged as stale and removed.

**Extend, never override.**
Projects inherit the core via `extends:`. Core behavior cannot be weakened — only extended. No rule or Manager changes without explicit owner approval.

**Tokens for judgment, scripts for everything else.**
If it's deterministic, it's a bash script. Zero tokens. AI spends cycles on reasoning and judgment, not on work that a `grep` can do better.

**Review is automatic.**
Every deliverable goes through adversarial peer review by a separate agent instance — isolated context, no access to the original reasoning. The cost is seconds and tokens, not human hours.

---

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

Scans your codebase, detects the stack, generates `.claude/` structure, `CLAUDE.md` bootloader, and KB foundation. If interrupted, run `leo init` again — it picks up where you left off.

### 3. Start working

```bash
claude
```

L.E.O. boots from the KB, loads rules and identity, and is ready. Describe what you need.

---

## Knowledge Base

The KB is the core — a neural network of JSON documents connected by tags.

```
.claude/kb/
├── schema.json       ← JSON Schema for all doc types
├── index.json        ← Neural map (by_tag, by_type, by_scope, by_lifecycle)
├── docs/             ← Flat document store
│   ├── think-before-execute.json    ← type: rule
│   ├── project-identity.json        ← type: identity
│   ├── session-wrap-up.json         ← type: skill
│   └── ...
└── scripts/
    ├── validate.sh   ← Schema validation (zero tokens)
    ├── build-index.sh ← Rebuild neural map (zero tokens)
    └── check-stale.sh ← Detect expired docs (zero tokens)
```

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

---

## CLI

| Command | What it does |
|---------|-------------|
| `leo init` | Interactive project onboarding (scan, configure, generate) |
| `leo setup` | Re-sync agents/rules/skills/KB to `~/.claude/` |
| `leo update` | Pull latest core + re-sync + migrate projects |
| `leo status` | Show core and current project state |
| `leo migrate-kb` | Migrate existing project to KB architecture |

---

## What it is not

- Not an autonomous agent framework — you stay in the loop at every inflection point
- Not a replacement for your project's `CLAUDE.md` — it complements it
- Not a generic Claude Code config — it is a working method with strong opinions
- Not a documentation system — knowledge is operated, not archived

---

## Status

Active development. Core architecture stable. CLI and onboarding tooling in progress.

---

*L.E.O. is the core. Your projects are the ecosystem.*
