---
name: metrics-collection
description: Collect 5 operational metrics per task to inform future system refinement.
---

## Rule

Every executed task leaves **one metric entry** in a project-specific JSONL file. This creates the dataset that will later allow refining agents and rules based on real data — not guesswork.

**Where:** `<project>/.claude/metrics/<YYYY-MM>.jsonl` (one file per month, one line per task)

**Responsibility:** Leo writes the entry when closing the task and synthesizing for the founder. Managers pass the raw data to Leo as part of the final report.

## Why

Inspired by Karpathy's autoresearch paradigm: any system that intends to be refinable needs a **measurable fitness function**. Without metrics, refining the core becomes guesswork — "I think the Dev Manager needs this extra rule". With metrics, we can look at the worst numbers and go straight to the pain.

Two immediate uses:

1. **Deciding where to refine.** After ~20-30 tasks, founder and Leo review the numbers. Which metric is worst? That's the area that needs adjustment in the core or rules.
2. **Input for the offline auto-refinement loop** (horizon 2 of the RDD §8.10). Once there's enough volume, we can build a benchmark from the logged tasks and run deliberate refinement of a Manager against real history.

## The 5 initial metrics

### 1. Peer review pass rate
**What it measures:** % of tasks that pass peer review **on the first attempt** (no rework).
**Collection:** the review instance records "approved" or "rejected" + iteration count.
**Field:** `review.first_pass` (bool), `review.iterations` (int)

### 2. Founder rejection rate
**What it measures:** % of final deliveries where the founder rejects or requests a substantial change after Leo reports "done".
**Collection:** Leo records whether, after reporting, the founder needed to request an adjustment in the same turn or the next.
**Field:** `founder.accepted_on_delivery` (bool)

### 3. Self-QA honesty rate
**What it measures:** % of tasks where the executing agent's self-QA was honest — i.e., it said it passed AND it actually passed review.
**Collection:** compare the self-QA output (all checks marked ✅) against the peer review result. Dishonest = self-QA said "all ok" but review found a problem.
**Field:** `self_qa.honest` (bool)

### 4. Rework cycles
**What it measures:** the number of back-and-forth iterations between executor and reviewer (or between Leo and founder) until the task closes.
**Collection:** Leo counts.
**Field:** `rework_cycles` (int)

### 5. Hiring loop hit rate
**What it measures:** % of tasks where the Manager correctly recognized a gap (and fired the Hiring Loop) **vs** % where they tried without a specialist and failed because of it.
**Collection:** when peer review or the founder identifies that a failure was due to a missing specialist, that task counts as a "hit rate miss". Tasks that fired the Hiring Loop count as a "hit".
**Field:** `hiring_loop.outcome` (string: "triggered" | "missed" | "na")

## JSONL entry format

Each line in the file is a self-contained valid JSON:

```json
{
  "task_id": "2026-04-08-001",
  "timestamp": "2026-04-08T15:30:00Z",
  "founder_prompt_summary": "Add a settings screen with a dark mode toggle",
  "manager": "dev",
  "specialist_used": "frontend-react-specialist",
  "domain_category": "ui_only",
  "review": {
    "first_pass": true,
    "iterations": 1
  },
  "self_qa": {
    "honest": true
  },
  "founder": {
    "accepted_on_delivery": true
  },
  "rework_cycles": 0,
  "hiring_loop": {
    "outcome": "na"
  },
  "duration_minutes_approximate": 12,
  "notes": "Clear task, executed on the first pass."
}
```

**Required fields:** `task_id`, `timestamp`, `manager`, `review`, `founder`, `rework_cycles`, `hiring_loop`
**Optional fields:** `specialist_used`, `domain_category`, `self_qa`, `duration_minutes_approximate`, `notes`

## How to apply

### End of task (Leo)

Before reporting done to the founder:

1. Collect raw data from each participant in the task (Manager, specialists, reviewer)
2. Compile a JSONL entry following the format above
3. Write (append) to `<project>/.claude/metrics/<YYYY-MM>.jsonl`
   - Create the file if it doesn't exist
   - Create the `.claude/metrics/` directory if it doesn't exist
4. Report to the founder normally (metric collection is transparent)

### Periodic review

Every 20-30 tasks (or once a month), the founder can ask Leo for a **metrics report**:

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

- **Real execution time** — Claude Code is already non-deterministic in time, and the founder doesn't care whether a task takes 5 or 15 minutes. `duration_minutes_approximate` is optional and coarse.
- **Subjective quality** — "did the code come out pretty?" is not a metric. Metrics are binary or countable.
- **Number of files changed** — weak correlation with quality. A big, well-done task is better than a small, badly-done one.
- **Tokens consumed** — it'll vary too much, and the refinement decision should not be to optimize cost at the expense of quality.

## Privacy

Metric logs live inside the project's `.claude/metrics/`, **gitignored**. They are not committed, they don't go to the repo. They are the founder's local data and serve the founder's analysis.

## Responsibility

This rule depends on Leo being disciplined about collecting and writing. If Leo forgets to log tasks, the dataset becomes skewed and refinement decisions will be based on a biased sample. The founder can periodically audit: "Leo, show me the last 10 entries in metrics.jsonl" — if something's missing, that's a signal to reinforce.
