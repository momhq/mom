# Refinements log

Living document. Each entry is a post-hoc refinement of decisions made in `rdd.md`, captured after a real session reveals something that needs to evolve. Entries are dated, append-only, and never rewrite previous ones.

The `rdd.md` stays as a point-in-time architectural record. This file tracks how it was refined after implementation started.

---

## 2026-04-09 — Wrap-up trigger for propagation

### The gap the RDD left open

`rules/propagation.md` (shipped with the initial core) said: *"Nenhuma task está completa até que o contexto do sistema reflita o que mudou... Ao fechar uma task que se enquadra acima, Leo roda o checklist."*

It defined **what** to propagate and **how**, but not **when**. The rule assumed a task has clear boundaries. In conversational sessions, it doesn't — every turn is a potential stopping point, and there's no hard signal that "we're done for now".

### What the first real session revealed

A Logbook v1.1 roadmap session on 2026-04-09 (morning) produced 5 GitHub issues, a new specialist (`swiftui-navigation-specialist`), and ~5 strategic decisions. The issues went to GitHub (external delivery worked). The specialist file was created locally. But the local `.claude/context/` files in the Logbook repo were never updated to reflect any of this — `project.md` still said "v1.1.0 scope not yet defined" and the specialist was left untracked.

When a *new* session opened later that same day to ask "what was decided?", it had to reconstruct everything from git diffs and GitHub API calls.

**Initial reading:** Leo of that session forgot to run the propagation checklist. Failure of discipline.

**Correct reading (after discussion with owner):** Leo had no way to know the task had "ended". The session was still open and productive when the decisions were made. Running the checklist after every decision would have been noisy and risked persisting intermediate state that was still changing. Running it at the "end" required knowing where the end was — which only the owner knows.

The rule was missing a **trigger semantic**, not a discipline enforcement. Adding more urgency to the existing rule would not have fixed it. Defining *when* to fire the checklist did.

### The refinement

Three-part trigger model added to `rules/propagation.md` as a new section "Quando disparar o checklist completo":

1. **Principal — explicit owner signal.** When owner says something like "fecha a sessão", "consolida aí", "tá bom assim", Leo invokes the `session-wrap-up` skill, which runs the full checklist deterministically. Natural-language recognition — not a slash command, not a keyword.

2. **Secondary — opportunistic single-decision propagation.** When a decision is clearly locked mid-session (owner explicitly uses "ponto final", "locked", "decidido"), Leo can propagate *just that decision* immediately without running the full checklist. Conservative criterion: in doubt, wait for wrap-up.

3. **Safety net — single question in long sessions.** If the session is accumulating decisions without a fechamento signal, Leo asks **once**: *"Tá juntando bastante coisa. Quer que eu feche a sessão agora ou seguimos?"*. Not repeated. Exists for the "owner in flow, forgot to wrap up" case.

The `session-wrap-up` skill was created to codify the wrap-up protocol (inventory → classify → plan → R2 → execute → report → optional learning capture), so Leo doesn't re-derive the steps each time and the owner sees a predictable shape.

### Why a skill and not a slash command

Owner feedback: *"Ter uma skill pra eu usar fere a ideia do copilot-core, que visa os agentes operarem sozinhos sabendo o que precisam ou não usar e quando. Então eu vejo mais como uma skill que seja usada pelo Leo lá e não eu ter que chamar."*

A slash command would have required the owner to remember a specific invocation. A model-invoked skill lets the owner signal in natural language and trusts Leo to recognize intent. This is more aligned with the core's original philosophy (agents autonomous in their tooling choices) and lower cognitive load on the owner.

### Why EN body with multi-language triggers

Convention: core documentation is in EN by default. But Leo recognizes wrap-up intent in any language, so the skill's `description` frontmatter lists both EN and PT trigger examples. Future non-PT owners/collaborators would add their language's trigger examples to the description without changing the protocol body.

### Why R2 inside the skill even though the signal was already given

Deliberate. "Wrap-up signal" authorizes *starting the protocol*, not *executing edits*. The skill forces Leo to present the propagation plan (Step 3) and wait for explicit approval before editing files (Step 4). This prevents the failure mode where owner says "fecha aí" meaning "wrap up whatever made sense" and Leo interprets it as carte blanche to commit anything.

The owner reviews the plan *before* anything is written. R2 is not optional, even on the happy path.

### Files shipped with this refinement

Copilot-core commit `ebf685b` (pushed to `origin/main` on 2026-04-09):

- `rules/propagation.md` — new section "Quando disparar o checklist completo"
- `skills/session-wrap-up/SKILL.md` — first skill in the core
- `agents/leo.md` — `skills: [session-wrap-up]` in frontmatter + new principle
- `scripts/sync.sh` — symlinks `skills/*` directories from core into `~/.claude/skills/`

### Open questions for field testing

These weren't decided — they're bets that the next session will validate or refine:

- **Does Leo reliably recognize wrap-up intent across phrasings?** The `description` is verbose but can't anticipate every owner phrasing. First field test: next Logbook v1.1 session.
- **Does opportunistic propagation get abused?** The "claramente locked" criterion is qualitative. Leo might lean too permissive or too conservative. Observe actual behavior over 2-3 sessions.
- **Does the safety-net question land at the right moment?** Hard to define "long session" quantitatively. Starting with Leo's judgment, tightening later if it misfires.
- **Is the Step 6 "optional learning capture" useful or is it noise?** Might produce cluttered memory directories if Leo invokes it on routine sessions.

All four questions are deliberate open bets, not design flaws. They exist to be answered by real use, not by more up-front design.

### What this refinement does NOT try to solve

- **Crash recovery.** If a session dies mid-work (client crash, context limit) before wrap-up, the work is still lost. Solving that is a separate problem — possibly auto-checkpointing on some cadence, or a "last known good" snapshot system. Not in scope here.
- **Cross-session continuity.** When owner opens a new session to continue the work of a previous one, Leo still has to reconstruct context from git and filesystem. This refinement helps by making sure the filesystem reflects reality at wrap-up time, but doesn't add any "resume from previous session" primitive.
- **Multi-owner scenarios.** The "owner signal" assumes a single owner. Collaborative scenarios would need a different design. Not relevant for current use.

### Source session

This refinement itself was produced in the session of 2026-04-09 (afternoon, after the morning Logbook roadmap session). The conversation started with the owner asking "o que foi definido na sessão anterior?" and progressively identified the propagation failure, diagnosed it as structural ambiguity, and designed the trigger mechanism. Meta-fun fact: the wrap-up skill did not exist yet when the wrap-up protocol was run manually for this same session.

---

## 2026-04-09 (evening) — Shared GitHub conventions + core self-hosting scaffold

### The gap the RDD left open

The original RDD described how projects inherit Managers and rules from the core via `extends:`, and `rules/propagation.md` governed what should be written down. But nothing in the core said how project-management artifacts (GitHub labels, milestones, issue titles, PR titles, Project v2 boards, issue templates, PR templates) should be organized. Each project was free to invent its own taxonomy.

In practice that meant Saintfy had a taxonomy born before copilot-core (persona-named `agent:*` labels referring to fictional characters Nico/Tomé/Davi/etc.), Logbook had zero taxonomy beyond GitHub defaults plus ad-hoc version labels, and there was no shared vocabulary for a Manager operating across both projects.

### What this session revealed

A Logbook session opened to execute the v1.1 roadmap asked Leo to also set up a GitHub Project v2 board mirroring the Saintfy setup. That tripped two deeper questions:

1. If Logbook is copying Saintfy's conventions, they should come from somewhere canonical, not by osmosis from reading another repo's config.
2. If core refinements keep happening as side-effects of project sessions (as this very refinement is doing), then the context-switching cost between "working on the project" and "working on the system that runs the project" becomes a real friction point for the owner.

### The two refinements

Two separate, related outputs landed in the same session:

#### A. Shared GitHub project management conventions

New canonical doc `docs/conventions/github-project-management.md` + templates in `docs/conventions/templates/`. It defines:

- **Labels** are limited to `agent:{dev,design,marketing,pm}` (owner) + optional `area:*` labels created on demand. GitHub defaults (`bug`, `enhancement`, etc.) are removed — they're redundant with the Type field.
- **Project v2 fields** carry the real classification: `Status` (Backlog/Ready/In Progress/In Review/Done), `Agent` (single-select, single owner), `Type` (single-select: feature/bug/tech-debt/design/pm/release). `Platform` only when multi-platform. Dropped `Released in` as redundant with Milestone.
- **Milestones** are strictly `vX.Y.Z` releases. Never themes or epics. Convention: "open a milestone when planning starts for that version".
- **Issue titles** use area-first `{Prefix}: {imperative action}` with prefix priority: area > action > artifact type. Language follows `locales.project_files` in `.claude/project-config.yml`.
- **PR titles** use Conventional Commits (`feat:`, `fix:`, `chore:`, etc.) — a deliberate divergence from issue naming, because issue and PR serve different scanning purposes.
- **PR bodies** follow a 5-section template (Summary, Why, Changes, Test plan, Related) where the Test plan section carries the `evidence-over-claim` discipline.
- **Issue templates** (`bug`, `feature`, `tech-debt`, `prd`) live in `docs/conventions/templates/issue/` and are copied into each project's `.github/ISSUE_TEMPLATE/` during setup. The PR template works the same way.
- **Project overrides** are allowed only when documented in `.claude/context/conventions-overrides.md` with a legitimate reason. Convention rot prevention.

Managers (`dev`, `designer`, `marketing`, `pm`) got a new Self-QA item: "issue title and PR title/body follow `docs/conventions/github-project-management.md`".

Applied immediately to Logbook: labels migrated, 10 issues renamed, `v1.0.1`/`v1.1` label-to-milestone promotion, Project #5 created with the full field set, both `logbook` and `logbook-legal` repos linked, templates installed. Saintfy migration deferred until Saintfy itself adopts copilot-core.

#### B. Core self-hosting scaffold

Owner's direct observation during the session: *"melhorias no core estão acontecendo já interando via os projetos, o que não tem problema nenhum, mas acaba nos tirando o foco do que precisa ser feito no projeto em si."*

The problem articulated: core refinements kept landing as side-effects of project sessions (this refinement is itself an instance). Each occurrence forced context-switching between product work and meta-work, mixed commits in confusing ways, and — most importantly — meant core changes weren't themselves passing through the fluxo PRD → RDD → execution that the core *prescribes*. Meta-incoherence.

The fix: add the minimum `.claude/` scaffold to `copilot-core` itself so that opening Claude Code in `~/Github/copilot-core/` gives Leo enough project context to behave as "the Leo of the core project". Three files:

- `.claude/project-config.yml` — name, stack (markdown/yaml/bash), `locales`, `self_hosting: true` flag
- `.claude/context/project.md` — what copilot-core is, what lives here, how it evolves, how it's dogfooded
- `.claude/context/self-hosting-notes.md` — the particularities that don't apply to downstream projects: propagation is instant (sync.sh symlinks), editing a Manager affects projects that extend it, editing Leo mid-session has a one-turn lag, language inconsistency (PT majority, EN drift), known loose ends (circular symlink bug in `skills/session-wrap-up/`, no templates for new Managers/rules)

No new Managers, no rule overrides, no specialists. The core Managers and rules already apply via `~/.claude/` symlinks. The scaffold just gives them project context when invoked from inside the core repo.

### Why both landed in the same refinement entry

They're the same root cause. The conventions needed a home that was clearly "the core, not one of the projects". The self-hosting scaffold is what makes the core a place where Leo can **work** on the core the same way he works on any other project — which is the dogfood loop the whole system depends on. Splitting them into two entries would hide that they were the same insight.

### Files shipped with this refinement

Copilot-core commits (pushed to `origin/main`):

- `d8bdb6e` — `docs/conventions/github-project-management.md` + 4 issue templates + PR template + Manager Self-QA updates
- `4de6928` — `.claude/project-config.yml` + `.claude/context/project.md` + `.claude/context/self-hosting-notes.md`

Logbook commits:

- `b25e5e3` — `.github/` templates
- `036086d` — `.claude/context/` updates propagating the session's decisions

logbook-legal commit:

- `9f66243` — `.github/` templates (no milestones — inherits release lifecycle from `logbook`)

### Open questions for field testing

As with the previous refinement, these are bets the next sessions will validate:

- **Does opening Leo directly in `copilot-core` feel different enough to actually separate core work from project work?** Or does the scaffold still leave Leo "too generic" because there are no core-specific Managers? First field test: the next core refinement that would have historically landed as a side-effect.
- **Does the `agent:*` label + `Type` field taxonomy hold up across projects with different workflows?** Logbook is small and mostly dev. Saintfy when migrated will have more cross-discipline work. If the taxonomy breaks down there, it needs another iteration.
- **Is the issue-area-first vs PR-type-first divergence confusing in practice?** Rationale was clear on paper. Reality will tell.
- **Does the "convention overrides only with a documented reason" rule actually prevent drift, or does it just produce a bunch of overrides files?** Leo audits are the main defense; this needs to be checked during wrap-ups.

### What this refinement does NOT try to solve

- **Language inconsistency in the core.** Rules are PT, newer conventions are EN, and this session added more EN. Flagged in `self-hosting-notes.md` as a loose end to resolve in a dedicated core session. Not forcing a migration in either direction now would be worse than the current drift.
- **The circular symlink bug** in `skills/session-wrap-up/session-wrap-up`. Observed and documented in `self-hosting-notes.md`, deliberately not touched in this session (out of scope, preserves the rule about touching only what the task is about).
- **Saintfy migration.** The new conventions apply to Saintfy only after Saintfy itself adopts copilot-core architecture. Trying to apply them piecemeal now would create inconsistency during migration.
- **Auto-add workflow** for Project #5 (items added to the repo automatically enter the board). GitHub only exposes that via the web UI, not via the API/CLI. Logged as a 30-second follow-up, not a blocker.

### Meta observation — this session as a field test of prior refinements

Two prior refinements were directly validated in this session:

1. **The wrap-up trigger from commit `ebf685b`** fired for the first time in the wild. Owner signaled with *"faz um wrap-up aqui nessa sessão"* — Leo recognized the intent, invoked the `session-wrap-up` skill (which shipped in the same earlier commit), ran the 6-step protocol, got R2 on the plan, executed. The first open bet ("Does Leo reliably recognize wrap-up intent across phrasings?") now has its first data point: yes, for this phrasing.
2. **`rules/escalation-triggers.md`** produced its first full cycle: Leo stopped before `git push` on `logbook-legal` (public repo), presented the situation in the structured escalation format, waited for explicit approval. Worked as designed.

### One learning captured as a memory, not a rule refinement

During this session Leo asserted that renaming a GitHub issue would "break notification history and cross-references in PR bodies". Owner pushed back: GitHub references issues by immutable `#N`, not by title slug. Leo was wrong, and hadn't verified before asserting.

This is exactly the failure mode that `rules/anti-hallucination.md` exists to prevent. The rule itself didn't need refinement — Leo failed to *apply* it. Captured as a feedback memory in the Logbook project memory directory rather than as a rule change. If this failure pattern recurs (N > 1), it becomes a candidate for a rule refinement — perhaps a more specific bullet in `anti-hallucination.md` about tool behavior assumptions.

### Source session

Afternoon of 2026-04-09 (immediately following the wrap-up trigger refinement of the morning). The session started with the owner asking Leo to resume the v1.1 roadmap for Logbook and set up a GitHub Project board. Everything in this refinement entry emerged from that request — the owner did not set out to design conventions or scaffold the core.
