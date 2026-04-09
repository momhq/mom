---
name: state-vs-learning
description: State memories age fast. Learning memories endure. Treat each differently.
---

## Rule

Memories have two different natures. Treating them the same is like indexing history books and technical manuals by the same rule — you lose the nuance that matters.

- **State memories** describe the factual state of the project at a point in time (what's been done, what's pending, who owns what, what the current blocker is). **They age fast.** They need to be revalidated before being cited as fact.
- **Learning memories** describe lessons, rules of conduct, work decisions, patterns that worked or didn't. **They age slowly.** They're nearly timeless.

## How to distinguish

**State memory** (ages fast):
- Describes what's done vs pending
- Contains a list of open issues
- Mentions the current app version, release cadence
- Names who is working on what
- Cites a metric at a specific moment ("X users", "Y% conversion")
- Records a decision that may be reverted or evolve
- Has a "Pending" section

**Learning memory** (ages slowly):
- Describes a rule of conduct ("always grep the real callsite before refactor")
- Explains an observed pattern ("iCloud Drive + cap sync = `* 2` duplicates")
- Captures a lesson from failure ("JWT without `typ: 'JWT'` breaks APNs")
- Documents a founder preference ("no toasts for form errors, use inline")
- Defines project vocabulary or convention

## How to apply

### When writing a memory

Before saving, classify mentally: **state or learning**? If it's state, mark the date explicitly in the body and assume it will age. If it's learning, write it in a timeless way when possible.

### When reading a memory

- **State memory**: **always verify against the current state** before acting on it. If the memory says "pending: feature X" and you're about to implement, first confirm it's still pending. If it says "issue #12 is in progress", confirm via `gh issue view 12` before citing.
- **Learning memory**: you can cite with more confidence, but still be careful with lessons that have been superseded by more recent learning.

### When propagation affects memories

When you update a state memory because it aged, apply the `propagation` rule — update, don't create a new one. When a learning memory is refined by a new failure (via `know-what-you-dont-know` Mechanism 4), you can create a new one that supersedes the previous — but mark the old one as superseded.

## Anti-pattern: memory as "last snapshot"

If you're tempted to write a memory that says "current project status: X, Y, Z, pending A, B", stop. That will be garbage in 1 week. Think about whether there's a genuine **lesson** in what you want to save. If there isn't, it probably shouldn't be a memory — it should be a one-shot report that lives in `outputs/` for that session.

## Responsibility

Leo is responsible for periodically auditing memories, identifying stale state memories, updating or removing them. Managers can propose this work when they notice they're carrying outdated memory that conflicts with observed state.

**Practical rule:** if a memory has a "Pending" or "Next steps" section, it's almost certainly state and deserves review every 1-2 weeks.
