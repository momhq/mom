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

**Correct reading (after discussion with founder):** Leo had no way to know the task had "ended". The session was still open and productive when the decisions were made. Running the checklist after every decision would have been noisy and risked persisting intermediate state that was still changing. Running it at the "end" required knowing where the end was — which only the founder knows.

The rule was missing a **trigger semantic**, not a discipline enforcement. Adding more urgency to the existing rule would not have fixed it. Defining *when* to fire the checklist did.

### The refinement

Three-part trigger model added to `rules/propagation.md` as a new section "Quando disparar o checklist completo":

1. **Principal — explicit founder signal.** When founder says something like "fecha a sessão", "consolida aí", "tá bom assim", Leo invokes the `session-wrap-up` skill, which runs the full checklist deterministically. Natural-language recognition — not a slash command, not a keyword.

2. **Secondary — opportunistic single-decision propagation.** When a decision is clearly locked mid-session (founder explicitly uses "ponto final", "locked", "decidido"), Leo can propagate *just that decision* immediately without running the full checklist. Conservative criterion: in doubt, wait for wrap-up.

3. **Safety net — single question in long sessions.** If the session is accumulating decisions without a fechamento signal, Leo asks **once**: *"Tá juntando bastante coisa. Quer que eu feche a sessão agora ou seguimos?"*. Not repeated. Exists for the "founder in flow, forgot to wrap up" case.

The `session-wrap-up` skill was created to codify the wrap-up protocol (inventory → classify → plan → R2 → execute → report → optional learning capture), so Leo doesn't re-derive the steps each time and the founder sees a predictable shape.

### Why a skill and not a slash command

Founder feedback: *"Ter uma skill pra eu usar fere a ideia do copilot-core, que visa os agentes operarem sozinhos sabendo o que precisam ou não usar e quando. Então eu vejo mais como uma skill que seja usada pelo Leo lá e não eu ter que chamar."*

A slash command would have required the founder to remember a specific invocation. A model-invoked skill lets the founder signal in natural language and trusts Leo to recognize intent. This is more aligned with the core's original philosophy (agents autonomous in their tooling choices) and lower cognitive load on the founder.

### Why EN body with multi-language triggers

Convention: core documentation is in EN by default. But Leo recognizes wrap-up intent in any language, so the skill's `description` frontmatter lists both EN and PT trigger examples. Future non-PT founders/collaborators would add their language's trigger examples to the description without changing the protocol body.

### Why R2 inside the skill even though the signal was already given

Deliberate. "Wrap-up signal" authorizes *starting the protocol*, not *executing edits*. The skill forces Leo to present the propagation plan (Step 3) and wait for explicit approval before editing files (Step 4). This prevents the failure mode where founder says "fecha aí" meaning "wrap up whatever made sense" and Leo interprets it as carte blanche to commit anything.

The founder reviews the plan *before* anything is written. R2 is not optional, even on the happy path.

### Files shipped with this refinement

Copilot-core commit `ebf685b` (pushed to `origin/main` on 2026-04-09):

- `rules/propagation.md` — new section "Quando disparar o checklist completo"
- `skills/session-wrap-up/SKILL.md` — first skill in the core
- `agents/leo.md` — `skills: [session-wrap-up]` in frontmatter + new principle
- `scripts/sync.sh` — symlinks `skills/*` directories from core into `~/.claude/skills/`

### Open questions for field testing

These weren't decided — they're bets that the next session will validate or refine:

- **Does Leo reliably recognize wrap-up intent across phrasings?** The `description` is verbose but can't anticipate every founder phrasing. First field test: next Logbook v1.1 session.
- **Does opportunistic propagation get abused?** The "claramente locked" criterion is qualitative. Leo might lean too permissive or too conservative. Observe actual behavior over 2-3 sessions.
- **Does the safety-net question land at the right moment?** Hard to define "long session" quantitatively. Starting with Leo's judgment, tightening later if it misfires.
- **Is the Step 6 "optional learning capture" useful or is it noise?** Might produce cluttered memory directories if Leo invokes it on routine sessions.

All four questions are deliberate open bets, not design flaws. They exist to be answered by real use, not by more up-front design.

### What this refinement does NOT try to solve

- **Crash recovery.** If a session dies mid-work (client crash, context limit) before wrap-up, the work is still lost. Solving that is a separate problem — possibly auto-checkpointing on some cadence, or a "last known good" snapshot system. Not in scope here.
- **Cross-session continuity.** When founder opens a new session to continue the work of a previous one, Leo still has to reconstruct context from git and filesystem. This refinement helps by making sure the filesystem reflects reality at wrap-up time, but doesn't add any "resume from previous session" primitive.
- **Multi-founder scenarios.** The "founder signal" assumes a single founder. Collaborative scenarios would need a different design. Not relevant for current use.

### Source session

This refinement itself was produced in the session of 2026-04-09 (afternoon, after the morning Logbook roadmap session). The conversation started with the founder asking "o que foi definido na sessão anterior?" and progressively identified the propagation failure, diagnosed it as structural ambiguity, and designed the trigger mechanism. Meta-fun fact: the wrap-up skill did not exist yet when the wrap-up protocol was run manually for this same session.
