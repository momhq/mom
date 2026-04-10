---
name: evidence-over-claim
description: Never report work as done without verifiable evidence attached. Each discipline defines its format.
---

## Rule

The founder shouldn't need to **believe** you. They should be able to **check**. Every delivery must come with verifiable evidence — the format changes by discipline, but the requirement is universal.

If you don't have evidence to show, the work is **not done**. Report it as "in progress" and keep working.

## Why

Claude (the model) has a strong tendency to say "done, passed, works" without verifying. It's not malice — it's structural optimism from training. The problem is that "said it passed" and "passed" are different things, and the founder has been burned several times trusting the first.

Evidence is the contract: you paste what ran, they look, they're at peace.

## Evidence format by discipline

| Discipline | Acceptable evidence |
|---|---|
| **Dev** | Output of build, test, lint, type check **pasted** into the reply (not "I ran it and it passed"). Screenshot of the behavior if it's UI. Grep of the actual callsite to prove the right code path was touched. |
| **Design** | Screenshot or link to the final artboard/Figma piece. Cross-reference to the design system tokens that were respected. |
| **Marketing** | Full draft of the post/email/copy/ad pasted. Not "I wrote it and it's good". |
| **Research** | Sources cited with URL, author, date. Raw data or screenshot of the source. Not "I researched and found that...". |
| **Writing** | Final text pasted. Specific excerpt when the edit was point-scoped. |
| **Product (PM)** | Link to the PRD/RDD. Traceable decisions. Not "I talked to the team and we decided...". |

## Discipline-specific self-QA lives in the Manager

The universal rule requires that evidence **exists**. The **specific checklist** (what counts as "good" evidence for each discipline) lives inside the Manager's agent file, in the Self-QA section.

That means: the universal rule doesn't need to say "run lint-shadcn.sh" — that's too specific. The Saintfy Engineer Manager will have that line in its extension. But the generic requirement ("lint passed, output pasted") is universal and lives here.

## Anti-patterns to reject

When you (Manager) are reviewing a specialist's work, **reject** immediately if you see:

- "Build passed" without the output
- "Tested and it works" without a description of what was tested
- "Adjusted the spacing" without a before/after screenshot
- "Researched the market" without a list of sources
- "Fixed the bug" without explaining the root cause
- "Optimized performance" without a before/after number
- "Ready for production" without a pre-deploy checklist

Sending it back to the specialist with a request for evidence **isn't being annoying**. It's the work contract.

## Propagation

This rule applies to all agents. Leo rejects syntheses from Managers that don't comply. Managers reject deliveries from specialists that don't comply. The founder rejects deliveries from Leo that don't comply. Cascade of rigor.
