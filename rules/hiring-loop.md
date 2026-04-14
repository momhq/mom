---
name: hiring-loop
description: Leo identifies a capability gap → spins up a specialist sub-agent → specialist executes the task.
---

## Rule

**Recognizing a gap** and **filling a gap** are separate responsibilities. Leo identifies the need and provisions the specialist; the specialist executes.

- Leo (or a running specialist) identifies that a task requires deep expertise that no existing specialist playbook covers → **reports** the gap to Leo
- Leo, with big-picture and cross-project context, **creates** the specialist playbook → spins up the specialist sub-agent with the correct briefing
- The specialist executes the task using the playbook

This mirrors real headhunting: the team says "we need a senior iOS engineer with push experience", the CTO writes the JD, sources, hires, delivers. Separation of concerns — but without a persistent middle layer.

## Why Leo provisions specialists directly

1. **Leo sees duplication.** If another specialist was already created for a similar domain in this session or another, Leo remembers. A specialist on its own doesn't have that cross-cutting view.
2. **Leo sees cross-project reuse.** If another project in `~/Github/*/` already has a similar specialist playbook, Leo proposes adapting instead of creating from scratch.
3. **Leo enforces structural standards.** Frontmatter, format, level of detail. Prevents messy playbooks created ad-hoc with inconsistent styles.
4. **Specialists stay focused.** They flagged the gap, Leo handles the meta-task of "writing a good playbook", and the specialist (or a new one) goes back to executing.

## When the hiring loop fires

**Two legitimate cases:**

### Case 1 — Assemble the initial team

When Leo begins working on a new project, it identifies which specialist playbooks are needed based on the stack and expected tasks, and provisions the basic team.

Examples:
- React project: "I need `frontend-react-specialist`, `backend-supabase-specialist`, `deploy-vercel-specialist`"
- Mobile project: "I need `mobile-ui-specialist`, `app-store-assets-specialist`"

### Case 2 — Fill a specific domain gap

During execution, a specialist (or Leo during routing) hits a task that requires deep expertise no current playbook covers.

Examples:
- Task requires APNs integration → Leo creates `apns-push-protocol-specialist`
- Task requires a complex Lottie animation → Leo creates `lottie-animation-specialist`

In both cases, **specialist playbooks live 100% in the project**, never in the core.

## Step-by-step flow

```
1. During task routing or execution, a gap is identified:
   "This task involves [X technical domain]. There's no specialist
    playbook for this. The Hiring Loop needs to fire."

2. Gap is reported to Leo (if identified by a running specialist):
   "I need a specialist `[proposed-name]`. Scope: [what they know].
    For what: [why this task needs it]. Worst case without it:
    [consequence of not having it]."

3. Leo checks:
   - Has a similar specialist already been created in this session or project?
   - Does another project have a reusable playbook? (checks ~/Github/*/.claude/specialists/)
   - Does the proposed scope make sense? Too broad or too narrow?

4. Leo formats a proposal and presents it to the owner (R2):
   "A specialist `[name]` with scope [Y] is needed. Proposal:
    [1-page playbook]. Approve?"

5. Owner approves, rejects, or requests an adjustment.

6. If approved: Leo creates the file at .claude/specialists/{domain}/{name}.md
   in the project. Spins up the specialist sub-agent with the playbook as context.

7. Specialist executes the task.
```

## Minimum specialist format

When Leo creates a specialist playbook, it follows this format:

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

❌ **A specialist creating another specialist without going through Leo.**
Without Leo's cross-cutting view, you generate duplicates and inconsistency.

❌ **Specialists that are too generic.**
A "Frontend generalist" trying to cover React + Vue + Svelte is useless. Be specific.

❌ **Specialists that replicate Leo's routing knowledge.**
If Leo already knows it well enough to route and brief, no specialist playbook is needed. Specialists exist for **deep** or **specific** execution knowledge.

❌ **Hiring loop for a 5-minute task.**
If the task is so small that creating a specialist costs more than executing carefully, the specialist can execute (with strict self-QA + review). The hiring loop is for tasks where the cost of being wrong justifies the cost of creating a playbook.
