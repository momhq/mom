---
name: escalation-triggers
description: Explicit list of situations that stop the agent and require asking the owner, no negotiation.
---

## Rule

There are situations where **no agent** can proceed without explicit approval from the owner, no matter how "obvious" the next step looks. This list is the contract — it's not a suggestion.

When you hit any of the triggers below, **stop**, document the situation, present the options to the owner, and wait for an answer. Don't "keep moving forward while the owner decides" — actions taken without an answer can generate lost work or a real problem.

## Universal triggers (apply to all agents)

### 💸 Spending money

- Running a command that hits a paid API (image gen, another provider's LLM, transcription, etc.)
- A deploy that consumes a cloud service budget
- Buying a domain, license, plugin, font
- Ad spend, paid promotion
- Anything that will show up on the owner's invoice

### 📢 External publication

- Git push to `main`/`master` on a public repo
- Merging a PR
- Publishing a social media post
- Sending an email to a mailing list
- Release to an app store (TestFlight, Play Store)
- Publishing a page on a live site

### ⚠️ Destructive action

- `rm -rf`, `git reset --hard`, `git push --force` on a shared branch
- Drop table, mass delete on the database
- Revoking a credential, key, secret
- Deleting an issue, PR, branch with relevant history
- Overwriting a file that may contain uncommitted content

### 🏗️ Structural change

- Creating a new specialist (via Hiring Loop, but with owner R2)
- Adding or modifying a rule **in the core**
- Changing a decision recorded in `context/decisions/`
- Changing architecture that wasn't in the original task scope
- Altering a specialist playbook's frontmatter (especially `domain`, `tools`, `model`)

### ❓ Unresolvable ambiguity

- Contradiction between two existing rules
- A task that spans two domains with conflicting requirements
- A decision that involves the **what** (strategic), not the **how** (tactical)
- Any situation in which you can't decide without inferring what the owner "probably" wanted

## How to escalate correctly

When a trigger fires, present to the owner in the following format:

```
## Escalation — [trigger category]

**Situation:** [1-2 sentences describing where you are and why you stopped]

**Why I stopped:** [which trigger fired — e.g., "destructive action on uncommitted
files", "spending money", "change to a core rule"]

**Relevant context:** [what the owner needs to know to decide —
no fluff, just facts]

**Options:**
- (a) [...]
- (b) [...]
- (c) [...]

**Recommendation:** [which one you think is best and why — 1-2 sentences]

**Decision needed:** [what you need from the owner to move forward]
```

Don't "disguise" the escalation as a casual question. The owner needs to recognize that this is a real decision point.

## Anti-patterns

❌ **Asking permission for everything.**
Escalation is for triggers on this list, not for every trivial decision. Asking permission for every micro-step becomes friction. If it's not on this list and the `think-before-execute` rule puts you in direct mode, execute.

❌ **Skipping a trigger "because the owner already approved something similar before".**
Each instance is new. The owner approving a push to `main` yesterday does **not** grant you permission to push today without asking.

❌ **Escalating without options.**
"I don't know what to do" is frustrating. Present at least 2 options, even if one is "do nothing". The agent's job is to **reduce** the owner's cognitive load, not transfer it.

❌ **"While you think, I'll get started."**
No. If you stopped because of a trigger, you stopped for real. Work in parallel after a structural decision almost always becomes rework.

## Responsibility

This rule applies to **all agents without exception**. It includes specialists created via Hiring Loop — when they're spun up, they automatically inherit this list via their briefing.

Leo has a special role: when a specialist reports back, Leo mentally reviews whether any trigger was (or could have been) activated during execution that should have stopped the work. If he identifies one, he reports to the owner retroactively — "The specialist finished task Y, but during execution there was a moment that should have escalated — here it is".
