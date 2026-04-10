---
name: hiring-loop
description: Manager reports a gap → Leo hires the specialist → hands it back to the Manager to execute.
---

## Rule

**Recognizing a gap** and **filling a gap** are separate responsibilities, assigned to different roles.

- Manager identifies that they need a specialist that doesn't exist → **reports** to Leo (does not try to create it alone)
- Leo, with big-picture and cross-project context, **hires** the specialist by formatting the correct playbook → hands it back to the Manager
- Manager uses the specialist and **executes** the task

This mirrors real headhunting: the engineering manager says "I need a senior iOS engineer with push experience", HR/CTO writes the JD, sources, interviews, hires, delivers. The manager executes the work with the new hire. Separation of concerns.

## Why Leo hires and not the Manager

1. **Leo sees duplication.** If another Manager already asked for a similar specialist in this session or another, Leo remembers. A Manager on their own doesn't have that cross-cutting view.
2. **Leo sees cross-project reuse.** If another project in `~/Github/*/` already has a similar specialist, Leo proposes adapting instead of creating from scratch.
3. **Leo enforces structural standards.** Frontmatter, format, level of detail. Prevents messy specialists created by different Managers with different styles.
4. **Manager stays focused.** They asked, they went back to execute the original task. They don't lose context on the meta-task of "writing a good specialist".

## When a Manager fires the hiring loop

**Two legitimate cases:**

### Case 1 — Assemble the initial team

In a Manager's first interactions with a new project, they identify which generalist specialists are needed based on the stack and expected tasks, and fire the Hiring Loop to assemble the basic team.

Examples:
- Engineer Manager on a React project: "I need `frontend-react-specialist`, `backend-supabase-specialist`, `deploy-vercel-specialist`"
- Designer Manager on a mobile project: "I need `mobile-ui-specialist`, `app-store-assets-specialist`"

### Case 2 — Fill a specific domain gap

During execution, the Manager hits a task that requires deep expertise the current team doesn't cover.

Examples:
- Task requires APNs integration → Engineer Manager asks for `apns-push-protocol-specialist`
- Task requires a complex Lottie animation → Designer Manager asks for `lottie-animation-specialist`

In both cases, **specialists live 100% in the project**, never in the core. The core keeps only universal managers.

## Step-by-step flow

```
1. Manager, executing a task:
   "This task involves [X technical domain]. I don't have a specialist
    on my team with a playbook for this. I need to fire the Hiring Loop."

2. Manager → Leo:
   "I need a specialist `[proposed-name]`. Scope: [what they know].
    For what: [why this task needs it]. Worst case without it:
    [consequence of not having it]."

3. Leo checks:
   - Has another Manager on this project asked for something similar recently?
   - Does another project have a reusable specialist? (checks ~/Github/*/.claude/specialists/)
   - Does the proposed scope make sense? Too broad or too narrow?

4. Leo formats a proposal and presents it to the founder (R2):
   "Manager [X] asked for a specialist `[name]` with scope [Y]. Proposal:
    [1-page playbook]. Approve?"

5. Founder approves, rejects, or requests an adjustment.

6. If approved: Leo creates the file at .claude/specialists/{domain}/{name}.md
   in the project. Hands it back to the Manager with the reference.

7. Manager reads the specialist as context and executes the task.
```

## Minimum specialist format

When Leo creates a specialist, they follow a format similar to Managers' but focused on actionable technical content:

```markdown
---
name: <specialist name>
description: <what they know in 1 line>
domain: <dev|design|marketing|pm|...>
---

## Domain
[What this specialist knows and does NOT know]

## Playbook
[Steps, checklist, gotchas, anti-patterns — real technical content]

## References
[Links, official docs, prior memories, relevant PRs]

## Self-check
[What the specialist must verify before reporting done]
```

## Anti-patterns

❌ **Manager creating a specialist without going through Leo.**
Without Leo's cross-cutting view, you generate duplicates and inconsistency.

❌ **Specialists that are too generic.**
A "Frontend generalist" trying to cover React + Vue + Svelte is useless. Be specific.

❌ **Specialists that replicate the Manager's knowledge.**
If the Manager already knows it, no specialist is needed. Specialists exist for **deep** or **specific** knowledge the Manager doesn't have.

❌ **Hiring loop for a 5-minute task.**
If the task is so small that creating a specialist costs more than executing carefully, the Manager can execute (with strict self-QA + peer review). The hiring loop is for tasks where the cost of being wrong justifies the cost of hiring.
