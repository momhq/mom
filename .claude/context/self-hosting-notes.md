# Self-hosting notes — specific care when running Leo inside the core

Working on copilot-core using copilot-core itself is useful dogfooding, but it has particularities that don't apply in a downstream project. This file lists what needs extra attention.

## 1. Propagation is instant

`~/.claude/agents/`, `~/.claude/rules/`, `~/.claude/skills/` are **symlinks** to this repo (created via `scripts/sync.sh`). When Leo edits `rules/peer-review-automatic.md` here, the next session of any project — Logbook, Saintfy, whatever — picks up the new version.

**Consequence:** there is no "staging" between editing a rule and it taking effect. Every change in `rules/`, `agents/`, `skills/` is a cross-project commit by construction.

**Mitigation:** the `escalation-triggers.md` rule already lists "Adding or modifying a rule in the core" as a mandatory R2 trigger. That applies here too — Leo never edits a rule without your approval. Even here, where "there's nobody else to block".

## 2. Changes in a Manager propagate to the projects that extend it

If Leo edits `agents/managers/engineer.md` here, every project that has `.claude/agents/managers/engineer.md` with `extends: ../../../../.claude/agents/managers/engineer.md` (via inheritance) inherits the change immediately in the next session.

**Consequence:** changing the Engineer Manager's Self-QA in the core affects Logbook now and Saintfy when it migrates. Propagation is active, not optional.

**Mitigation:** before changing a Manager in the core, answer mentally "does any project that inherits this Manager depend on the current behavior?". If yes, the change requires a migration note in the commit and verification in the affected project's next work. Leo writes that note as part of the diff, not separately.

## 3. Editing Leo inside a Leo session

If you modify `agents/leo.md` in the middle of a session, the **next** Leo invocation (including a sub-agent inside the same session) loads the new version. The current session keeps the old version in memory.

**Consequence:** there's a one-invocation lag between editing Leo and Leo behaving differently. Normally invisible, but can cause confusion if you expect an immediate change on the next turn of the same session.

**Mitigation:** after editing `agents/leo.md`, open a new session to validate that the behavior changed. Don't trust the next turn of the current session to test.

## 4. No other projects to escalate to

In a downstream project (Logbook, Saintfy), the "escalation target" is the founder. Here it's also the founder, but without a project layer between you and the architectural decision. That increases the temptation for Leo to "decide alone" because it's "in the core, it's meta-work".

**Mitigation:** the `think-before-execute` and `escalation-triggers` rules still apply without exception. Meta-work is not a license for direct mode. If something looks like a structural decision, it is — and it needs R2.

## 5. Artifact languages — core is locked to EN

As of 2026-04-09, all core artifacts are standardized on English:

- `rules/*.md` — EN
- `agents/leo.md` — EN
- `agents/managers/*.md` — EN
- `docs/rdds/2026-04-08-copilot-core-architecture/rdd.md` — EN
- `docs/rdds/2026-04-08-copilot-core-architecture/refinements.md` — EN
- `docs/conventions/github-project-management.md` — EN
- `docs/conventions/templates/*` — EN
- `skills/session-wrap-up/SKILL.md` — EN
- `README.md` — EN

This is enforced at config level: `.claude/project-config.yml` has `locales.project_files: en`. Any new artifact in the core must be written in English. The rationale is neutral ground — every contributor (human or AI) can read and extend the system without a language barrier.

Interaction language (Leo ↔ founder) is a personal choice and lives in `.claude/project-config.local.yml` (gitignored). Each contributor sets their own without touching the committed config.

## 6. Known loose ends

Things that are messy in the core and deserve a dedicated cleanup session:

- **No `context/` in the core itself beyond this scaffold** — when the core gains state that needs to be tracked (e.g., an evolution roadmap), we need to decide whether it lives in `context/` (like projects) or in `docs/` (like RDDs).
- **No templates for new Managers or new rules** — if someone needs to create an `agents/managers/legal.md` or `rules/retention-policy.md`, there's no skeleton. Would be useful, but speculative right now.

## 7. Recommended session flow in the core

1. Open Claude Code in `~/Github/copilot-core/`
2. Leo identifies it's a core session via `self_hosting: true` in `project-config.yml`
3. Founder describes the intent
4. Leo applies `think-before-execute`: a structural decision is almost always alignment mode
5. If it's a rule/Manager/skill, Leo writes a proposal (prose, not code) for R2
6. R2 approved → Leo edits
7. Evidence at the end (diff, sync.sh output, whatever is verifiable)
8. Wrap-up on the founder's signal → `session-wrap-up` skill runs the protocol, commit with a clear message referencing the relevant RDD or refinement
9. Push to `origin/main` (private repo, no escalation trigger; if it goes public, escalation applies)

Sessions on the core tend to be shorter and more "documentary" than project sessions. That's not a flaw — it's the nature of the work here.
