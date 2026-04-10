# copilot-core — the project

This repo **is** the core of the multi-agent system that manages all the owner's other projects. It's a meta project: the code here isn't a product, it's the definition of the working method that the products (Logbook, Saintfy, future ones) inherit.

## What lives here

- **`agents/leo.md`** — the Manager of Managers. Conversational entry point in any session of any project.
- **`agents/managers/`** — the 4 universal tech leads (Engineer, Designer, Marketing, PM). Projects extend via `extends:` to add local context without rewriting the base.
- **`rules/`** — 11 universal rules that govern behavior of all agents. Propagation, anti-hallucination, peer-review, escalation, etc.
- **`skills/`** — model-invoked skills that agents use on demand. Today only `session-wrap-up`.
- **`docs/conventions/`** — operational conventions shared across projects (e.g., GitHub project management). Templates live in `docs/conventions/templates/`.
- **`docs/rdds/`** — Research & Design Docs. Each big architectural decision becomes an RDD with `rdd.md` + append-only `refinements.md`.
- **`cli/`** — Node.js CLI (v0.1.0) for setup, project onboarding, updates, and status. Replaces `sync.sh` as the primary interface. Commands: `setup`, `init`, `update`, `status`.
- **`scripts/sync.sh`** — legacy symlink installer (still works, but CLI is preferred).

## Stack

Markdown + YAML frontmatter for agents/rules. Node.js + TypeScript for the CLI (`cli/`). The CLI uses `@clack/prompts` for interactive UX, `commander` for command parsing, and `tsup` for builds.

## Core philosophy

**Copilot-style, not Paperclip-style.** Owner talks to Leo, Leo delegates to the Managers, Managers delegate to the specialists. Review is automatic via adversarial sub-instances. Owner decides the *what* and *why*, Leo decides the *who* (routing and delegation), Managers decide the *how*, comes back to the owner on anything irreversible or structural.

**Extend, never override.** Projects inherit the core via `extends:` in the Managers' frontmatter. They can add rules, self-QA items, specifics to the local stack. They can never remove universal behavior.

## How to evolve the core

The core itself follows the method it defines:

1. **Big architectural decision** → becomes a new RDD in `docs/rdds/YYYY-MM-DD-slug/`
2. **Post-implementation refinement** → becomes an append-only entry in the original RDD's `docs/rdds/.../refinements.md`
3. **New rule or change to existing rule** → mandatory escalation trigger; only applies with explicit R2 from the owner
4. **New Manager** → strategic decision, goes through R2 and becomes an RDD
5. **New skill** → escalation trigger; only added when there's a clear reason (don't create speculative skills)

## Dogfooding

The expectation is that every session on copilot-core is itself an exercise of the method: PRD if it's a big system feature, RDD if it's a technical design, adversarial peer review, evidence-over-claim, propagation at the end. If the method makes sense in self-hosting, it makes sense in downstream products.

## Relevant history

The core was born from an initial RDD (`docs/rdds/2026-04-08-copilot-core-architecture/`) written from the accumulated learning of the Saintfy project that preceded it. Logbook is the first real pilot (Pilot Phase 1, started 2026-04-08).

The first post-implementation refinement (ebf685b) was about wrap-up-driven propagation — it came from a real failure observed in the pilot, not from speculation. That's the expected evolution pattern: field failures → documented refinements → cross-project propagation.
