---
name: peer-review-automatic
description: All work goes through automatic, transparent peer review before reaching the owner.
---

## Rule

No work reaches the owner without going through review. Review is done either by **Leo reviewing the specialist's output**, or by **a separate review sub-agent** (another Sonnet instance) spun up with isolated context, an adversarial posture, and fired automatically — the owner never opens another session manually.

## Why

**Self-QA and peer review catch different things.** Self-QA is the author checking their own work — it catches what the author can see. Peer review is a fresh pair of eyes without the original reasoning's context — it catches confirmation bias, shortcuts, dead code, wrong callsite, regressions, ignored edge cases. They're not redundant; they're complementary layers.

**Human cost is what makes peer review expensive in human teams.** In AI, the cost is seconds and tokens. There's no rational reason not to have universal review.

## Standard flow

```
Owner → Leo routes the task
                ↓
             Leo spins up a Specialist sub-agent with briefing + context
                ↓
             Specialist executes → self-QA → reports to Leo
                ↓
             Leo reviews the output (natural review — Leo has the big picture)
                ↓ approves → synthesizes for the owner
                ↓ rejects → back to the specialist with comments
             Leo → Owner
```

For **complex or high-risk tasks**, Leo can spin up a dedicated review sub-agent instead of reviewing directly:

```
Specialist executes → self-QA → reports to Leo
    ↓
Leo fires a review sub-agent (Sonnet instance in review mode)
(via Claude Code's native Task tool)
    ↓
Review sub-agent receives:
  - Changed files / diff
  - Self-QA output
  - Adversarial context (review mode)
  - WITHOUT the execution session's reasoning/context
    ↓
Reviews adversarially → approves or lists problems
    ↓
Result returns to Leo
Owner sees everything as a single thing
```

## Adversarial context — base template

When Leo fires a review sub-agent, the briefing includes (at minimum):

```
You are a Specialist in REVIEW mode. Another instance just did
the work described below. Your job is to review adversarially.

REVIEW MODE RULES:
1. You do NOT have access to the executing session's reasoning. Only
   the artifacts (diff, files, self-QA).
2. Actively look for bugs that self-QA doesn't catch:
   - Dead code introduced
   - Regressions elsewhere
   - Wrong callsite (is the real code path actually this one?)
   - Unintended side effects
   - Unverified assumptions
   - Ignored edge cases
   - Hidden [INFERRED] marks without signaling
3. DO NOT praise. If it's ok, say "approved" and stop.
4. If there's a problem, list it specifically and concretely:
   - file:line
   - what's wrong
   - why this is a problem
5. You have the same expertise as the executor — use it adversarially.

MATERIAL TO REVIEW:
[diff / changed files / self-QA output]
```

Leo can **extend** this template with discipline-specific review criteria depending on the task domain (code checks for engineering, visual checks for design, etc.).

## Context isolation is mandatory

The reviewer instance **cannot** see the original session's context. If it does, confirmation bias returns (the reviewer reads "I chose to do it this way because X, Y, Z" and agrees). Isolation ensures the reviewer evaluates the **result** on its merit, not on the reasoning.

In Task tool terms: invoke the sub-session with a fresh prompt containing only the necessary artifacts.

## Iteration

If the reviewer (Leo or a review sub-agent) rejects:
1. Goes back to the executing specialist
2. Specialist fixes based on the specific comments
3. Re-submits
4. A new review pass happens
5. Loop until approval

The owner doesn't see each iteration — they receive the final result once the task is approved in review. If the loop is taking too long (3+ iterations), report to the owner to decide whether to escalate or abort.

## Exceptions

**There are no exceptions.** A 10-second task also goes through review — review of a 10-second task also takes 10 seconds. The marginal cost is zero. If you're tempted to skip review "because it's simple", that's exactly where bugs slip through.

The only legitimate exception is when the task **is** itself a meta-review action (e.g., the owner asks "review this PR" — then the whole task is review, no review of the review is needed).
