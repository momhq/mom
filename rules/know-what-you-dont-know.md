---
name: know-what-you-dont-know
description: Managers and specialists stop BEFORE executing when they detect a domain outside their capability.
---

## Rule

Before writing any line of code, design, or copy, the executing agent **stops and evaluates** whether they really have the expertise to deliver the task with quality. If the answer is "no", fire the Hiring Loop or escalation — **before** failing, not after.

## Why this needs explicit enforcement

Claude (the model) knows almost everything superficially. An Engineer Manager told to implement APNs will try — because it has superficial APNs knowledge from training. That's exactly the failure mode this rule prevents.

A rule saying "ask for help when you don't know" is not enough. The model will think it knows. A mechanism is needed that **forces** meta-reasoning to be materialized.

## The 4 mechanisms (all mandatory)

### Mechanism 1 — Mandatory pre-execution check

Before writing any code, design, or copy, the agent **fills in and pastes into the reply** this form (can't just "think about it", must write):

```
## Pre-execution check

- Specific technical domain of this task (1 sentence):
  [...]

- Do I have a specialist on my team with a playbook for this domain?
  [ ] Yes → specialist: [name + file]
  [ ] No → STOP: fire Hiring Loop before continuing

- If I get this task wrong for lack of expertise, what's the worst case?
  [ ] Trivial bug, easy to revert → I can execute carefully
  [ ] Hard-to-revert bug or cascading effect → specialist mandatory
  [ ] Production / security / money risk → specialist mandatory

- Is my confidence in this domain high or low?
  [ ] High, and I know why (cite the source: specialist X, memory Y, doc Z)
  [ ] Low OR "high" without a citable source → STOP
```

**The trick is to write it.** Thinking "I know this" is easy. Having to cite a verifiable source forces the model to confront "do I actually know it or am I guessing?".

If the agent tries to cheat by writing "yes" without a source or "high confidence" without justification — the founder sees it in the output and calls it out. Transparency forces honesty.

### Mechanism 2 — Trust gradient by category

Each Manager has a default trust table by task category. Some categories **never** execute without a specialist, even if the task looks simple.

Template (each Manager customizes in their own agent file):

| Category | Default trust | Specialist mandatory? |
|---|---|---|
| [low-risk category] | High | No |
| [medium-risk category] | Medium | Depends on scope |
| [high-risk category] | **Low** | **Always** |

Example for the Engineer Manager:

| Category | Default trust | Specialist mandatory? |
|---|---|---|
| Text/copy/static JSON edit | High | No |
| Pure UI (new component, styling) | High | Only if specific design system |
| Standard CRUD with known ORM | High | No |
| Integration with external API | Medium | Yes if it's a protocol (APNs, OAuth, WebAuthn) |
| Complex schema migration | Medium | Yes |
| Crypto / auth / security | **Low** | **Always** |
| Native bridging (Capacitor, React Native) | **Low** | **Always** |
| Infra / deploy / CI | **Low** | **Always** |

The "**Always**" column is a hard line. Even under pressure, the Manager does not execute — they fire the Hiring Loop or escalate to the founder.

### Mechanism 3 — Post-failure hardening

When peer review (the `peer-review-automatic` rule) detects that an agent failed due to **lack of expertise in domain X**, that failure becomes **automatic** input for the trust gradient.

Process:
1. Peer review rejects: "failed because there was no playbook for domain X"
2. Leo records the failure
3. Leo adds X to the "Always specialist" list for the Manager **in the current project** (via the `.claude/agents/managers/<manager>.md` extension)
4. Next time the Manager hits a task in X, the trust gradient already blocks execution without a specialist

This turns failures into automatic enforcement — each mistake corrects the system so it doesn't repeat. It's the opposite of the old pattern where lessons sat in memories nobody re-read.

### Mechanism 4 — Lessons learned pass

After peer review rejects work, **before fixing the task**, the agent runs a lessons learned pass:

```
## Lessons learned (mandatory after review rejection)

- Which rule or checklist item would have prevented this failure?
  [...]

- Is this lesson specific to this project or universal?
  [ ] Project-specific → propose adding to the project's extended agent file
  [ ] Universal → propose adding to the core agent file

- The lesson should become:
  [ ] An item in the Manager's Self-QA
  [ ] An item in the trust gradient (Mechanism 2)
  [ ] A new domain rule
  [ ] An update to an existing specialist

- Written proposal (1 paragraph):
  [...]

- Proposal goes to Leo → R2 with founder before being applied
```

Without this, lessons sit in the founder's head (until they forget) or in stale memories. With this, each failure has a chance to become permanent enforcement.

## How the 4 fit together

- **Mechanism 1** is preventive, runs before each task
- **Mechanism 2** is structural, sets hard lines per discipline
- **Mechanism 3** is reactive and automatic, hardens after failure
- **Mechanism 4** is deliberately reflective, extracts a lesson to refine the system

Together, they form a system that **learns and strengthens with every real failure**, respecting R2 (nothing applies without founder approval).

## Responsibility

This rule applies to **every executing agent** (Managers when they execute as an exception, specialists when they execute work). Reviewers (Managers in review mode) also consult the trust gradient when evaluating whether the execution was legitimate or an attempt to dodge the system.
