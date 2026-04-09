---
name: Large feature (PRD)
about: Big feature that needs a Product Requirements Doc before any code
title: 'PRD: '
labels: ''
assignees: ''
---

<!--
Title format: PRD: {feature name}
Examples:
  PRD: Google Sign-In native iOS
  PRD: launch strategy Apple-first then Google Play
-->

## One-line goal

<!-- The single sentence that captures what this feature is trying to achieve. -->

## Status

- [ ] PRD draft (PR: #?)
- [ ] PRD approved
- [ ] RDD draft (PR: #?)
- [ ] RDD approved
- [ ] Execution issues created

## Documents

- **PRD:** `docs/prds/YYYY-MM-DD-slug/prd.md` _(to be created)_
- **RDD:** `docs/rdds/YYYY-MM-DD-slug/rdd.md` _(to be created)_

## How this issue works

This issue is a **tracker** for a large feature. Actual content lives in the canonical docs above, not in this body.

1. **PM Manager** writes the PRD as a PR adding `docs/prds/YYYY-MM-DD-slug/prd.md`. Founder reviews on the PR. When merged, this issue moves `PRD` → `RDD` on the board.
2. **Dev Manager** (or relevant technical Manager) writes the RDD as a PR adding `docs/rdds/YYYY-MM-DD-slug/rdd.md`. Founder reviews. When merged, issue moves to `Backlog`.
3. The tracker is split into 1+ execution issues that flow `Backlog → Ready → In Progress → In Review → Done`.
4. When all execution issues close, this tracker closes too.

## Initial context (for the PM Manager to expand into the PRD)

<!-- Founder's raw thoughts, constraints, references, links. The PM pulls from here when writing the PRD doc. -->

## Refs

- Existing memories / decisions / docs that inform this:
- Related issues:
