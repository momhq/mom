# copilot-core

A replicable working method for Claude Code agents. A conversational manager (Leo) + a team of Managers per discipline + universal rules. Extends per project without rewriting the base.

**Status:** early-stage, no pilot yet. Private repo while the design matures.

## Philosophy in 2 sentences

**Copilot-style, not Paperclip-style.** You talk to Leo, he delegates to the Managers, they delegate to the specialists (hired via Hiring Loop), review is automatic and transparent, you validate at inflection points. Founder decides the *what*, Leo decides the *how*, comes back to the founder on anything irreversible or structural.

## Structure

```
copilot-core/
├── agents/
│   ├── leo.md                      ← Manager of Managers (model: opus)
│   └── managers/
│       ├── dev.md                  ← development tech lead
│       ├── designer.md             ← design tech lead
│       ├── pm.md                   ← product tech lead
│       └── marketing.md            ← marketing tech lead
├── rules/                          ← 11 universal rules
│   ├── propagation.md
│   ├── anti-hallucination.md
│   ├── think-before-execute.md
│   ├── evidence-over-claim.md
│   ├── peer-review-automatic.md
│   ├── state-vs-learning.md
│   ├── hiring-loop.md
│   ├── know-what-you-dont-know.md
│   ├── escalation-triggers.md
│   ├── inheritance.md
│   └── metrics-collection.md
├── scripts/
│   └── sync.sh                     ← symlinks core → ~/.claude/ (to be written — D8)
└── docs/
    └── rdds/                       ← versioned architectural decisions
```

## How to use (once the pilot validates it)

**First time on a machine:**
```bash
git clone git@github.com:vmarinogg/copilot-core.git ~/Github/copilot-core
bash ~/Github/copilot-core/scripts/sync.sh
```

**Updates:**
```bash
cd ~/Github/copilot-core && git pull
# Symlinks point to the repo — content already updated.
# Run sync.sh only if topology changed (files added/removed).
```

**Per project** (e.g. Saintfy, logbook):
- Active project gets `.claude/agents/managers/<manager>.md` with `extends: ../../../.claude/agents/managers/<manager>.md` in the frontmatter
- Project-specific rules and specialists live under the project's `.claude/`, never here in the core

## Conventions

**Managers are tech leads, not executors.** They receive, decompose, delegate to the specialist team, review, synthesize. They execute code/design/copy directly only as an exception (micro-tasks, emergency).

**Specialists live in the project, never in the core.** Core keeps only universal Managers + universal Rules. Each project builds its specialist team via Hiring Loop as needed.

**Rules in 2 scopes:**
- **Universal** (here in the core, under `rules/`) — "how the company works". Always loaded.
- **Domain** — "how that team works". Embedded in the Manager files, loaded only when the Manager is invoked.

**Manager style: minimalist.** Identity + principles + checklist. Zero long prose. Fixed internal structure: Role → Principles → Hiring loop → Self-QA → Escalation.

**Tone: casual.** Second person, zero corporate-speak. Interaction language is configurable per project.

## Architectural reference

Foundational decisions live in:
- `docs/rdds/2026-04-08-copilot-core-architecture/` — main RDD (will be copied once the repo stabilizes)
- Saintfy-Copilot/docs/rdds/2026-04-08-copilot-core-architecture/rdd.md (origin, pre-copy)

## Current state

- ✅ Leo + 4 Managers (Dev, Designer, PM, Marketing) written
- ✅ 11 universal rules written (10 from the RDD + metrics-collection)
- ✅ `sync.sh` implemented, tested idempotent, active in `~/.claude/`
- ✅ Logbook pilot configured (Phase 1 — hiring loop and extends active, peer review and metrics deferred to Phase 2)
- ⏳ First real task in logbook (next)
- ⏳ Saintfy migration — Q8 (after pilot is stable)

## What it is not

- Not an autonomous agent framework (Paperclip-style)
- Not a generic CLI for Claude Code
- Not a replacement for a project's CLAUDE.md
- Not an open-source product yet (might become one — see the RDD parking lot)
