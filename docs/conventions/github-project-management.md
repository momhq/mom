# GitHub project management conventions

Canonical reference for how repos managed by copilot-core organize issues, PRs, labels, milestones, and Projects v2 boards. This doc is the source of truth; each project copies the templates into its own `.github/` and is expected to follow the same rules unless it has an explicit, documented override in `context/`.

The goal is that any Manager (Leo, Engineer, Designer, Marketing, PM) operating across any project can predict the shape of an issue or PR without learning a new convention per repo. **Standard beats clever.**

---

## 1. Labels

The label taxonomy is deliberately small. Labels are for **cross-cutting filters** that need to work outside the Project (search, `gh` CLI, notifications, issue list views in the repo). Anything that is purely kanban-view concern belongs in a Project field instead, not in labels.

### 1.1 Universal labels (every repo has these)

**None.** GitHub's default labels (`bug`, `enhancement`, `documentation`, `duplicate`, `good first issue`, `help wanted`, `invalid`, `question`, `wontfix`) are **removed** from every repo on setup. They are redundant with the Type field and clutter the label list.

### 1.2 Optional labels (per-project, created on demand)

Project-specific areas use an `area:*` prefix. Examples:

- `area:push` — notifications subsystem
- `area:auth` — authentication
- `area:charts` — progress charts
- `area:cloudkit` — CloudKit sync

Areas are **not** pre-seeded. Create when the second issue in the same subsystem shows up. Before then, it's premature.

No priority labels (`p0`, `p1`, etc.). Priority is expressed by kanban column position and milestone, not label.

---

## 2. Project v2 fields

These are the **required** fields on every project's kanban. They live in the Project, not in labels, because enforcement matters more than cross-tool filterability for classification data.

### 2.1 `Status` (single-select, colored)

Default column flow:

- `Backlog` — not yet prioritized
- `Ready` — prioritized, ready to pick up
- `In Progress` — actively being worked on
- `In Review` — PR open, awaiting review
- `Done` — merged and closed

Projects that run the graduated PRD/RDD workflow add two upstream columns:

- `PRD` — product requirements doc being drafted
- `RDD` — technical design doc being drafted

Put PRD and RDD **before** Backlog if used. Keep the rest in the same order.

### 2.2 `Agent` (single-select, colored)

Which Manager owns this item. Options:

- `engineer` — Engineer Manager
- `design` — Designer Manager
- `marketing` — Marketing Manager
- `pm` — PM Manager

The field is named `Agent` (not `Manager`) because it reads more naturally on kanban cards and is the convention already established in the pilot. Options point to the Manager roles; the legacy persona names (`tomé`, `nico`, `davi`, etc. from pre-copilot-core Saintfy) are deprecated and replaced during migration.

Single-select enforces **one owner per issue**. If work crosses domains, the Manager that owns the *primary* outcome is the owner, and cross-manager coordination is handled via the body / sub-issues / comments — not by multi-selecting.

### 2.3 `Type` (single-select, colored)

What kind of work this is. Options:

- `feature` — new user-facing functionality
- `bug` — broken behavior
- `tech-debt` — refactor, cleanup, infra, housekeeping (no user-facing change)
- `design` — UI/UX work that isn't a feature or bug (polish, systemization)
- `pm` — PRD, roadmap, prioritization artifact
- `release` — version bump, release ceremony, changelog

Single-select because the taxonomy only works if items land in exactly one bucket. Labels (which are free-form) would let people drift into `type:ux-polish`, `type:minor-fix`, etc. and lose the taxonomy. The field locks the vocabulary.

### 2.4 `Platform` (single-select, colored) — **optional**

Only use when the project has more than one platform (e.g. Saintfy has iOS + web + backend). Logbook is iOS-only and omits this field entirely.

Options (when used): `iOS`, `Android`, `Web`, `Backend`, `LP` (landing page), `Both`, etc. — defined per project.

### 2.5 Standard GitHub fields (always present)

`Title`, `Assignees`, `Labels`, `Linked pull requests`, `Milestone`, `Repository`, `Reviewers`, `Parent issue`, `Sub-issues progress`. These come for free with every Project v2; include them all.

### 2.6 Fields we explicitly **don't** use

- **`Released in`** — redundant with `Milestone`. Filter/group by milestone instead. Dropped.
- **Priority** — expressed via column position and milestone, not a dedicated field.
- **Estimate / Size** — not used until a project demonstrates a real need for capacity planning.

---

## 3. Milestones

**Rule:** one milestone = one released version. Never use milestones for themes, epics, or phases.

### Naming

`vX.Y.Z` (semver). Examples: `v1.0.1`, `v1.1`, `v2.0`.

### Lifecycle

- Open a milestone when planning starts for that version
- Assign issues as they're scoped into the version
- Close the milestone the moment the App Store / production release ships
- Never reopen a closed milestone — bumped work moves to the next milestone

### Filtering

Native GitHub supports filtering and grouping by milestone in Projects v2 (`group by: Milestone` in view settings) and on the CLI (`gh issue list --milestone "v1.1"`). No need for a redundant `Released in` field.

---

## 4. Issue naming

### 4.1 Format

```
{Prefix}: {imperative action} [(optional context)]
```

- **Prefix** is one of: area, action verb, or artifact type (see 4.2)
- **Imperative action** starts with a verb in imperative present (`add`, `fix`, `refactor`, `audit`, `migrate`) — not descriptive (`addition of`, `fixing`, `refactoring`)
- **Optional context** in parentheses, for secondary info that doesn't fit in the main title
- Max ~70 characters (fits in GitHub's list view without truncation)

### 4.2 Prefix priority (use the first one that applies)

1. **Area-first** — when the work lives in a known subsystem
   - `Push: handle permission denied state`
   - `Charts: convert Y axis to user weight unit`
   - `Navigation: migrate destination views to .navigationTitle`
2. **Action-first** — when the work is cross-area and the verb dominates
   - `Fix: RLS blocks upsert on push_subscriptions`
   - `Cleanup: remove SignUp.tsx dead code`
   - `Refactor: extract FormatterRegistry`
3. **Artifact-first** — when the issue tracks a non-code deliverable
   - `PRD: Google Sign-In native iOS`
   - `RDD: copilot-core architecture`
   - `Decision: weight unit is display-only`

### 4.3 Language

Follows `locales.project_files` from the project's `.claude/project-config.yml`. If the project config says `en`, issues and PRs are in English. If `pt`, they're in Portuguese. **Never mix languages within a project.** Mixed-language projects must pick one in their config before any issues are written under these conventions.

### 4.4 Examples — good vs bad

✅ `Profile: add delete account flow`
✅ `Push: automate APNs dev→prod switch for release builds`
✅ `Fix: hide splash screen explicitly`
✅ `PRD: launch strategy Apple-first then Google Play`

❌ `Delete Account in Profile` — no prefix, passive form
❌ `bug with notifications` — lowercase, no area, describes not prescribes
❌ `Atualizar cores do tema` — wrong language for an `en` project

---

## 5. PR naming

### 5.1 Format

```
{type}: {imperative action}
```

PR titles use **Conventional Commits** prefixes, not area-first. This is a deliberate divergence from issue naming. Rationale: an issue describes *work to do* (area is the most useful mental filter); a PR describes *what changed* (type of change is the most useful mental filter, and enables changelog tooling later).

### 5.2 Allowed types

- `feat:` — new user-facing feature
- `fix:` — bug fix
- `chore:` — tech debt, infra, housekeeping, dependencies
- `docs:` — documentation only
- `refactor:` — code restructuring with no behavior change
- `test:` — test-only changes
- `style:` — formatting, whitespace (almost never used alone)
- `perf:` — performance improvement with no other change

### 5.3 Examples

✅ `feat: native Apple Sign-In + Push Notifications for iOS`
✅ `fix: hide splash screen explicitly`
✅ `chore: migrate historical PRDs to docs/prds/`
✅ `refactor: extract NavigationCoordinator from RootView`

❌ `Update auth flow` — no type prefix, vague verb
❌ `feat: Added some fixes to push notifications` — past tense, vague
❌ `Fix the bug` — no type prefix (wrong format), no specifics

---

## 6. PR body

Every PR body follows this template. Sections can be shortened or omitted only when genuinely not applicable — never omitted for laziness.

```markdown
## Summary

<One paragraph. What changes and why, at the level a reviewer needs to
understand the intent. Do not describe file by file.>

## Why

<Optional. Use only when the rationale is not obvious from the Summary —
e.g. non-trivial tradeoff, unusual constraint, failed alternative.>

## Changes

<Bulleted high-level changes. Cite file:line when a specific location matters.
Do not list every file touched — the diff is right there.>

-
-

## Test plan

<What you ran to validate. Paste output when relevant (build, test, lint,
type check). For UI changes: screenshot or screen recording link.>

- [ ]
- [ ]

## Related

<`Closes #N` for PRs that close an issue (auto-closes on merge).
`Part of #N` for PRs that are one slice of a larger issue.
`Relates to #N` for context-only references.>
```

### 6.1 Evidence discipline

The Test plan section is where `rules/evidence-over-claim.md` lands in the PR workflow. "Rodei e passou" without output is not acceptable. If the PR touches code, the reviewer should see concrete evidence — build output, test run, lint output, screenshot. This is non-negotiable per the rule.

### 6.2 Series PRs

When a PR is one of several slices of a larger issue (e.g. `#9 PR 1`, `#9 PR 2`, `#9 PR 3`), each PR body must:

- Open with "This is PR N of M for #9" in the Summary
- Use `Part of #9`, not `Closes #9`, except for the final PR which uses `Closes #9`
- State what the preceding PRs established (1 sentence) and what the next PR will do (1 sentence)

This prevents reviewers from losing the narrative between PRs.

---

## 7. Issue templates

Each repo has `.github/ISSUE_TEMPLATE/` with four templates: `bug.md`, `feature.md`, `tech-debt.md`, `prd.md`. Source templates live in `docs/conventions/templates/issue/` in copilot-core and are copied into each repo's `.github/` during project setup. Template bodies define the minimum structure an issue needs; actual title is written by the human following section 4.

Templates intentionally do **not** auto-set the `Type` field — fields can't be pre-set from issue templates, only labels can. The Type is set when the issue is added to the Project board. Templates also don't auto-set `agent:*` because ownership is set on the kanban too.

See `docs/conventions/templates/issue/` for the actual template files.

---

## 8. PR template

Each repo has `.github/pull_request_template.md` — a copy of `docs/conventions/templates/pull_request_template.md` from copilot-core. GitHub shows this template when opening a new PR, pre-filling the body with the structure from section 6.

---

## 9. Project setup checklist

When setting up a new repo under copilot-core, run this checklist:

1. Delete GitHub default labels (`bug`, `enhancement`, `documentation`, `duplicate`, `good first issue`, `help wanted`, `invalid`, `question`, `wontfix`)
2. Copy `docs/conventions/templates/issue/*.md` → `.github/ISSUE_TEMPLATE/`
3. Copy `docs/conventions/templates/pull_request_template.md` → `.github/pull_request_template.md`
4. Create Project v2 with fields from section 2 (Status, Agent, Type, Platform if multi-platform)
5. Link the repo to the Project
6. Enable "Auto-add" workflow: issues and PRs from this repo automatically enter the Project on creation
7. Open milestone `vX.Y` for the current in-flight version if one exists

For a project that spans multiple repos (e.g. Logbook = `logbook` main + `logbook-legal` landing), all repos share the same Project v2 but may have different milestone lifecycles (the landing page usually inherits milestones from the main app rather than having its own).

---

## 10. Overrides

Projects can diverge from this convention **only** when they have a documented reason in `.claude/context/conventions-overrides.md` — a short file listing what the project does differently and why. Overrides are audited by Leo periodically; if an override drifts from "genuine reason" to "we forgot to align", it gets removed.

Examples of legitimate overrides:

- A project uses a different `Status` flow because of a regulatory review step
- A project adds extra types (`type:security` in a healthtech app)

Examples of illegitimate overrides that would be rejected:

- "We don't like the area-first format" — opinion, not reason
- "The team is used to something else" — discipline problem, not a convention problem

Illegitimate overrides are the path to convention rot. When in doubt, follow the core and propose a change to the core via R2 instead.

---

*Lives in the core because the whole point is shared language across projects. Last updated at initial authoring (pilot phase 1).*
