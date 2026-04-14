---
name: metrics-collection
description: Collect 8 operational metrics per task to inform future system refinement.
---

## Rule

Every executed task leaves **one metric entry** in a project-specific JSONL file. This creates the dataset that will later allow refining agents and rules based on real data — not guesswork.

**Where:** `<project>/.claude/metrics/<YYYY-MM>.jsonl` (one file per month, one line per task)

**Responsibility:** Leo writes the entry when closing the task and synthesizing for the owner. Specialists pass the raw data to Leo as part of their final report.

## Why

Inspired by Karpathy's autoresearch paradigm: any system that intends to be refinable needs a **measurable fitness function**. Without metrics, refining the core becomes guesswork — "I think the engineering specialist playbook needs this extra rule". With metrics, we can look at the worst numbers and go straight to the pain.

Two immediate uses:

1. **Deciding where to refine.** After ~20-30 tasks, owner and Leo review the numbers. Which metric is worst? That's the area that needs adjustment in the core or rules.
2. **Input for the offline auto-refinement loop** (horizon 2 of the RDD §8.10). Once there's enough volume, we can build a benchmark from the logged tasks and run deliberate refinement of specialist playbooks and rules against real history.

## The 8 metrics

### 1. Peer review pass rate
**What it measures:** % of tasks that pass peer review **on the first attempt** (no rework).
**Collection:** the review instance records "approved" or "rejected" + iteration count.
**Field:** `review.first_pass` (bool), `review.iterations` (int)

### 2. Owner rejection rate
**What it measures:** % of final deliveries where the owner rejects or requests a substantial change after Leo reports "done".
**Collection:** Leo records whether, after reporting, the owner needed to request an adjustment in the same turn or the next.
**Field:** `owner.accepted_on_delivery` (bool)

### 3. Self-QA honesty rate
**What it measures:** % of tasks where the executing agent's self-QA was honest — i.e., it said it passed AND it actually passed review.
**Collection:** compare the self-QA output (all checks marked ✅) against the peer review result. Dishonest = self-QA said "all ok" but review found a problem.
**Field:** `self_qa.honest` (bool)

### 4. Rework cycles
**What it measures:** the number of back-and-forth iterations between executor and reviewer (or between Leo and owner) until the task closes.
**Collection:** Leo counts.
**Field:** `rework_cycles` (int)

### 5. Hiring loop hit rate
**What it measures:** % of tasks where Leo (or the specialist) correctly recognized a gap (and fired the Hiring Loop) **vs** % where execution proceeded without a specialist playbook and failed because of it.
**Collection:** when peer review or the owner identifies that a failure was due to a missing specialist playbook, that task counts as a "hit rate miss". Tasks that fired the Hiring Loop count as a "hit".
**Field:** `hiring_loop.outcome` (string: "triggered" | "missed" | "na")

### 6. Delegation quality
**What it measures:** whether Leo delegated to the correct specialist on the first attempt, or tried to solve it himself / spun up the wrong specialist first.
**Why it exists:** the Logbook v1.1 pilot revealed that Leo sometimes attempts to provide solutions directly (e.g., design decisions) instead of delegating to the appropriate specialist. The original 5 metrics didn't capture this — the task would still show as "delivered, owner accepted" even though the process was wrong.
**Collection:** Leo self-reports honestly. If Leo catches himself having worked on something before delegating, or if the owner points it out, record it.
**Field:** `delegation.quality` (string: "correct_first" | "self_attempted_then_delegated" | "wrong_agent_then_corrected")

### 7. Internal iterations
**What it measures:** the number of significant back-and-forth iterations that happened **inside** the execution — between Leo and the specialist, or between Leo and the owner on clarifications — **before** the formal delivery.
**Why it exists:** `rework_cycles` only counts post-delivery rejections. The Logbook pilot had a design analysis that went through 3 rounds of factual corrections before delivery, but `rework_cycles` showed 0 because the final delivery was accepted. The real friction was invisible.
**Collection:** the specialist reports how many significant revision rounds happened during execution. A "round" means the agent produced output, received correction (from self-QA, from Leo, or from the owner mid-task), and revised. Minor typo fixes don't count; substantive corrections do.
**Field:** `internal_iterations` (int)

### 8. Leo operational errors
**What it measures:** errors made by Leo himself that generated unnecessary work, delays, or required recovery — regardless of whether the task ultimately succeeded.
**Why it exists:** Leo's own mistakes (e.g., deleting a base branch in stacked PRs, asserting a rule is missing without checking config) were invisible in the original metrics. They'd show up as successful "recovery" tasks or not at all. This metric ensures Leo's operational quality is tracked with the same rigor as the specialists'.
**Collection:** Leo self-reports. When Leo causes an incident that requires recovery work, or the owner points out a Leo-level mistake, log it. Be honest — this metric exists precisely because the system was hiding its own errors.
**Field:** `leo_errors` (array of strings, each a brief description of the error; empty array `[]` if none)

## JSONL entry format

Each line in the file is a self-contained valid JSON:

```json
{
  "task_id": "2026-04-08-001",
  "timestamp": "2026-04-08T15:30:00Z",
  "owner_prompt_summary": "Add a settings screen with a dark mode toggle",
  "domain": "engineering",
  "specialist_used": "frontend-react-specialist",
  "domain_category": "ui_only",
  "review": {
    "first_pass": true,
    "iterations": 1
  },
  "self_qa": {
    "honest": true
  },
  "owner": {
    "accepted_on_delivery": true
  },
  "rework_cycles": 0,
  "hiring_loop": {
    "outcome": "na"
  },
  "delegation": {
    "quality": "correct_first"
  },
  "internal_iterations": 0,
  "leo_errors": [],
  "duration_minutes_approximate": 12,
  "notes": "Clear task, executed on the first pass."
}
```

**Required fields:** `task_id`, `timestamp`, `domain`, `review`, `owner`, `rework_cycles`, `hiring_loop`, `delegation`, `internal_iterations`, `leo_errors`
**Optional fields:** `specialist_used`, `domain_category`, `self_qa`, `duration_minutes_approximate`, `notes`

## How to apply

### End of task (Leo)

Before reporting done to the owner:

1. Collect raw data from each participant in the task (specialists, reviewer)
2. Compile a JSONL entry following the format above
3. Write (append) to `<project>/.claude/metrics/<YYYY-MM>.jsonl`
   - Create the file if it doesn't exist
   - Create the `.claude/metrics/` directory if it doesn't exist
4. Report to the owner normally (metric collection is transparent)

### Periodic review

Every 20-30 tasks (or once a month), the owner can ask Leo for a **metrics report**:

```
Leo, show me this month's metric numbers.
```

Leo reads the month's JSONL, aggregates, presents:
- Total task count
- Rate of each metric
- Worst cases (tasks with lots of rework, rejected reviews, hiring loop missed)
- Patterns (does any domain category concentrate failures?)

That's the input for the next core refinement conversation.

## What NOT to measure

- **Real execution time** — Claude Code is already non-deterministic in time, and the owner doesn't care whether a task takes 5 or 15 minutes. `duration_minutes_approximate` is optional and coarse.
- **Subjective quality** — "did the code come out pretty?" is not a metric. Metrics are binary or countable.
- **Number of files changed** — weak correlation with quality. A big, well-done task is better than a small, badly-done one.
- **Tokens consumed** — it'll vary too much, and the refinement decision should not be to optimize cost at the expense of quality.

## Privacy

Metric logs live inside the project's `.claude/metrics/`, **gitignored**. They are not committed, they don't go to the repo. They are the owner's local data and serve the owner's analysis.

## Responsibility

This rule depends on Leo being disciplined about collecting and writing. If Leo forgets to log tasks, the dataset becomes skewed and refinement decisions will be based on a biased sample. The owner can periodically audit: "Leo, show me the last 10 entries in metrics.jsonl" — if something's missing, that's a signal to reinforce.

**Leo must record his own errors with the same rigor he records the specialists'.** The Logbook v1.1 pilot showed that without `delegation.quality`, `internal_iterations`, and `leo_errors`, the metrics painted a falsely positive picture — zero rework, all first-pass approved — while the owner experienced real friction (3 rounds of design corrections, Leo attempting design solutions instead of delegating to a specialist, branch deletion causing PR cascade). Metrics that only measure output quality without measuring process quality are structurally biased toward positive. These 3 fields exist to close that gap. If Leo finds himself wanting to skip `leo_errors` or mark `delegation.quality` as `"correct_first"` when it wasn't — that's exactly the honesty problem `self_qa.honest` was designed to catch, applied to Leo himself.
