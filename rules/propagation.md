---
name: propagation
description: Every decision or context change must propagate to the impacted files before closing the task
---

## Rule

No task is complete until the system's context reflects what changed. If you made a decision, changed a stack, defined a new pattern, or learned something worth remembering for next time — it must be materialized before reporting "done".

**Outdated information is worse than missing information.** It induces silent error.

## When to apply

Propagate whenever the task includes:

- **Decision** in product, design, tech, business, or marketing
- **Stack change** (new lib, migration, altered architecture)
- **New pattern** (convention, template, domain rule)
- **Context change** (project status, persona, new competitor, reverted decision)
- **Asset or template created** that other agents can reuse
- **Lesson** from a failure that would have been avoided with a rule or checklist

## When to fire the full checklist

Conversational sessions don't have explicit endings. Every turn is a potential stopping point. For that reason the propagation checklist **does not run after every tool call nor after every individual decision** — that would be noisy, expensive, and would risk persisting intermediate state that's still going to change.

The checklist runs in **three situations**, with different weights:

### 1. Primary — explicit owner end-of-session signal

When the owner signals that the session (or the current block of work) has ended, Leo **invokes the `session-wrap-up` skill**, which runs the full protocol deterministically.

Natural-language signal examples (the list is not exhaustive — Leo recognizes the *intent* in any language, not the exact phrase):

- "fecha a sessão" / "fecha aí" / "pode fechar"
- "vamos consolidar" / "consolida aí" / "salva isso"
- "acho que tá bom por hoje" / "tá bom assim" / "pronto por hoje"
- "próxima vez continua daqui" / "daqui a pouco volto"
- "manda bala, fecha" / "finaliza"
- "wrap up", "let's consolidate", "save this", "done for today", "we're good"

If the signal is ambiguous (e.g., the owner says "ok" after a task ends — it could be "ok, done for today" or "ok, keep going"), Leo **asks once** before invoking the skill: *"Do you want me to close the session now (run wrap-up) or is this just a pause?"*.

### 2. Secondary — opportunistic single-decision propagation

When a decision is **clearly locked** mid-session — e.g., the owner says "iOS 17 is the floor, final" or "decision: TabBar is out, drawer stays" — Leo can propagate **just that decision** immediately, creating or editing the relevant file (e.g., `context/decisions/...`). This **does not** fire the full checklist nor invoke the skill; it's only capturing an isolated fact that won't change.

Criteria for "clearly locked":

- The owner used an explicit closing word ("final", "locked", "decided", "not reopening")
- OR the decision was explicitly contrasted with alternatives and one was chosen with a rationale
- **When in doubt, wait for wrap-up.** Propagating too incrementally recreates the problem this rule solves.

### 3. Safety net — single question in a long session

If Leo notices the session is growing long (many accumulated decisions, expanding context, multiple turns without any closing signal), he asks **once**: *"There's a lot piling up. Do you want me to close the session now or keep going?"*. That's it — don't insist. The safety net exists to cover the "owner in flow, forgot to wrap up" case, not to break rhythm.

**Anti-pattern:** Leo **never** runs the full checklist without one of the three triggers above. Propagating mid-task on his own initiative is like committing on every line of code — it defeats the point of the rule.

## How to apply

When closing a task that falls into the above:

1. **Identify** what changed (decision, fact, pattern)
2. **Consult** the project's `context/propagation-map.md` (if it exists) or use the mental checklist below to map impacted files
3. **Update** each impacted file
4. **Verify** with grep that no stale references remain
5. **Report** to the owner: "Propagation done — I updated X, Y, Z"

When the trigger is the **explicit end-of-session signal** (situation 1 above), Leo invokes the `session-wrap-up` skill, which orchestrates these steps deterministically (inventory → classify → plan → R2 → execute → report). See `~/.claude/skills/session-wrap-up/SKILL.md` (source in `leo-core/skills/session-wrap-up/`).

## Mental checklist (Leo runs mentally when closing any task)

- [ ] Was any decision made? → `context/decisions/{domain}.md`
- [ ] Did what the project **is** change? → `context/project.md`, `context/brand.md`
- [ ] Did the stack change? → `context/stack.md`
- [ ] Does any specialist playbook need to know this? → the relevant playbook (or memory)
- [ ] Does any workflow reference what changed?
- [ ] Does any project rule need an update?
- [ ] Is any memory now stale? → update or remove
- [ ] Does any canonical doc (PRD, RDD) reference something that changed?

## What NOT to propagate

- Implementation details that live in code (code is the source of truth)
- Temporary state of an in-progress task
- Information that's already in git history
- Session outputs (those go to `outputs/`)

## Final responsibility

**Leo is ultimately responsible for propagation.** Specialists can flag propagation needs within their scope (domain rule, specific memory) when reporting back to Leo, but Leo is the one who ensures nothing slipped through. If the owner complains about missing propagation, Leo answers — not the specialist.
