---
name: session-wrap-up
description: Use this skill when the founder signals end of session or asks to wrap up, consolidate, or persist the learnings from the current work block. Recognize the intent across languages — triggers include "wrap up", "let's consolidate", "close the session", "save this", "done for today", "we're good", "finalize", or the Portuguese equivalents "fecha a sessão", "consolida aí", "tá bom assim", "pronto por hoje", "salva isso". Any natural-language signal that the current session or block of work is ending and the decisions/learnings should be materialized. This skill orchestrates the full propagation checklist from propagation.md — it lists what changed in the session, classifies each item, presents a propagation plan for founder R2, executes the edits, commits with a clear message, and reports what was done. Invoked by Leo (model-invoked), never by the founder directly.
---

# Session wrap-up

## When this applies

Invoke when the founder signals that the current session or block of work is ending and the learnings/decisions should be persisted. Recognition is based on **intent**, not exact phrasing — Leo identifies wrap-up intent in any language. See `rules/propagation.md` section "Quando disparar o checklist completo" for the full semantics.

If the signal is ambiguous (e.g. founder says "ok" after a task finishes — could be "ok, done for today" or just "ok, continue"), **ask once** before invoking:

> *"Want me to close the session now (run wrap-up) or is this just a pause?"*

Do **not** invoke this skill:

- Mid-task, after a single decision (use opportunistic propagation from `propagation.md` §"Quando disparar o checklist completo" — it allows propagating a single locked decision without running this full protocol)
- Without any founder signal (the safety-net question counts as a signal only if the founder answers "yes, wrap up")
- For a session that already ran this skill (check: is there a recent commit with "session wrap-up" or similar in the message? If yes, the wrap-up already happened — ask the founder before running again)
- Just to end your turn cleanly (this is not a "goodbye" ceremony)

## The protocol (execute in order)

### Step 1 — Inventory

List **what actually changed** in the current session's context. Be specific. Go through mentally:

- **Decisions made** — strategic, architectural, product, design, or tech decisions. Capture the rationale, not just the conclusion.
- **State changes** — project status, pending items closed, new pending items opened, scope adjustments, milestone updates, issue lists
- **New artifacts** — specialists hired, new managers, new rules proposed, new docs created, new files added to the repo
- **Learnings / failures** — something that broke or surprised in a way that should inform future work

Skip:

- Implementation details that live in code (code is source of truth — `git log`/`git blame` have them)
- Temporary task state (in-progress work that was not finalized)
- Conversation turns that did not produce a durable decision or artifact
- Things already committed or already in auto-memory

**Be honest about scope.** If the session produced only one decision and no artifacts, say so — do not manufacture inventory to justify running the protocol.

### Step 2 — Classify each inventory item

For each item, determine **where** it should propagate, using the checklist in `rules/propagation.md`:

| Item type | Target |
|---|---|
| Strategic decision / pattern / rationale | `<project>/.claude/context/decisions/<topic>.md` |
| Project state change (status, scope, pendencies) | `<project>/.claude/context/project.md` |
| Stack change (new lib, migration, architecture shift) | `<project>/.claude/context/stack.md` (or `project.md` if that is where stack lives) |
| Manager-level rule or learning specific to the project | Agent file of the relevant manager (project extension, not core) |
| Learning useful across future sessions of this project | Auto-memory (`~/.claude/projects/<project>/memory/`) |
| Rule refinement (universal, not project-specific) | **ESCALATION** — propose to founder for core change. Do NOT edit core from inside the skill. |
| External roadmap artifact (issues, PRs, milestones) | The external system (GitHub Issues, etc). Local files should *reference* these, not duplicate them. |

Mark items that require no propagation as "no-op — reason: X" so the founder can see you considered them.

### Step 3 — Present the propagation plan

Show the founder a concise plan. Suggested format:

```
## Session wrap-up — propagation plan

**Inventory (N items)**
1. [item description] → [target file/action]
2. [item description] → [target file/action]
...

**Skipped (M items)**
- [item] — reason: [no-op / already persisted / out of scope]

**Commit strategy**
- [single commit / multiple commits] with message: "[draft message]"
- Files to stage: [explicit list]
- Files deliberately NOT staged: [list + reason]

**Escalations needed**
- [any items that need founder R2 beyond this plan itself — e.g. core
  changes, new specialists, external actions]

Waiting for approval to execute.
```

Wait for explicit founder approval ("yes", "go", "execute", "approved", or Portuguese equivalents) before Step 4. If the founder asks for changes, revise and re-present. **Never execute without approval.**

### Step 4 — Execute

- Edit/create files exactly as planned — no drift from the approved plan
- Run grep sanity checks for staleness (e.g. `grep -r "old claim" .claude/`) to confirm no stale references remain that contradict the new state
- Stage the exact files planned (no `git add .` or `git add -A` — explicit file list)
- Commit with the drafted message, ending with the standard `Co-Authored-By` footer
- If this is in a project that pushes automatically on commit: push. Otherwise: leave uncommitted and report the commit SHA to the founder, asking whether to push now or later.
- If there are items for the auto-memory system, write those as separate memory files (not inside the git commit). Auto-memory lives outside the project repo.

### Step 5 — Report

Brief report back to the founder:

```
## Wrap-up complete

**Committed:** [SHA] — N files, +X/-Y lines
**Memory written:** [list, or "none"]
**Pushed:** [yes / no / not applicable]
**Deferred or still pending:** [list, or "nothing"]

**Session-level learning:** [one-line summary, if any]
```

Mention briefly any items that were in the inventory but deliberately not propagated, so the founder knows they were seen and skipped on purpose (not forgotten).

### Step 6 — Session-level learning capture (optional)

If this session produced a **learning about the copilot-core system itself** (a rule gap, a failure mode, a pattern that should be codified), ask the founder once if they want to capture it as:

- A feedback memory (quick, cheap — just a note for future sessions)
- A rule refinement proposal (heavier — requires R2 and eventually a core change)

Do **not** force this step. Skip if the session was routine. Skip if the founder already discussed the learning explicitly during the session.

## What this skill does NOT do

- **Does not decide *whether* to propagate** — that is in `rules/propagation.md`. This skill is the *how*, triggered by the *when*.
- **Does not edit `copilot-core` itself.** Refinements to the core are escalations handled separately with explicit R2 outside this skill.
- **Does not run on its own.** It is model-invoked by Leo after recognizing a wrap-up signal.
- **Does not skip R2.** Every execution of the edits requires explicit founder approval. The skill exists to make propagation *deterministic and transparent*, not *automatic*.
- **Does not handle session termination** (context limit hit, client crash). Those are recovery scenarios, not wrap-up scenarios. Separate problem, separate solution.

## Anti-patterns

- **Invoking on every "ok"** — "ok" is not a wrap-up signal. Only invoke when the intent is clearly "we are done, persist the learnings".
- **Skipping inventory and going straight to "here is my commit plan"** — the inventory is what reveals the real scope. Without it, items get forgotten.
- **Invoking mid-task "just in case"** — the safety-net rule allows asking ONCE in a long session, not auto-invoking. If the answer is "keep going", do not ask again for a while.
- **Executing without R2** — even if the plan is obvious, R2 is not optional. The founder is authorizing persistence, not just the content.
- **Treating this as a session-end ceremony even when nothing changed** — if the inventory is empty, say so briefly and stop. Do not manufacture propagation.
- **Bundling unrelated work in one commit "because we are wrapping up anyway"** — if the session touched two unrelated areas, commit them separately with clear scopes.
