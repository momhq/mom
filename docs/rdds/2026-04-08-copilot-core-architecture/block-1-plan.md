# Plan: Copilot-Core Architecture — Block 1 Definitions

## Context

On 2026-04-08 we ran a long planning session that produced the RDD for the `copilot-core` architecture (`Saintfy-Copilot/docs/rdds/2026-04-08-copilot-core-architecture/rdd.md`). The RDD left 8 open questions to be resolved before implementation.

This session tackled **Block 1** of those questions — the "form" decisions that unblock everything else: Manager format, initial team scope, and project initialization flow. The goal of the plan is to lock those decisions in an executable document so that the next session (implementation) starts with a single standard and no ambiguity.

**Important**: this plan **implements nothing**. It does not create the `copilot-core` repo, does not write agent files, does not run sync.sh. It is only the Block 1 decisions contract. Implementation is a separate session.

---

## Decisions locked in this session

### D1 — Manager style: **Minimalist**

Chosen via side-by-side comparison with the Verbose style.

**Rationale:**
- Tokens matter in long sessions (memories already load, universal rules already load — Manager doesn't need to repeat what's in other places of the core)
- Easier to maintain consistency across 4-6 different managers
- Casual tone fits better with lean format
- Reference/checklist is more faithful to the Manager's real role (tech lead operating, not training a beginner)

**Reference for how a minimalist Manager should look** (Dev Manager version, as it ended up in the preview):

```yaml
---
name: Dev Manager
extends: core/agents/managers/dev.md
tools: Read, Edit, Write, Glob, Grep, Bash, Task
model: sonnet
skills: [project-briefing]
---
```

```
Dev tech lead. Receives, delegates, reviews, synthesizes.
Executes only in exceptions.

## Principles
- PR-first (worktree + Closes #N)
- 3-strikes debugging before asking for help
- Grep real callsite before refactor
- Self-QA with pasted proof (build/lint/test)

## Hiring loop
Expertise the team doesn't cover → stop and report to Leo.

## Self-QA
- [ ] Build passed (paste output)
- [ ] Lint passed (paste output)
- [ ] Type check passed
- [ ] Real code path exercised

## Escalation
Stop before: spending money, publishing externally,
destructive action, changing a core rule.
```

### D2 — Tone: **Casual**

- Second person (direct to the agent), direct language, zero corporate speak
- PT example: "Você é o tech lead de dev" and not "Você é o líder técnico responsável pela disciplina de desenvolvimento..."
- EN example: "You're the dev tech lead" and not "You are the development technical lead responsible for..."
- **Language** of Leo's interaction with the founder, and language of the core files, is decided at project setup (see D6), not in this architectural decision. The founder has a personal preference to interact with AI in PT but keep project code/docs in EN — this is legitimately per-user/project config, not per-core.

### D3 — Frontmatter: 6 fixed fields

```yaml
---
name: <Manager name>                       # required
description: <short role sentence>          # required
extends: <path relative to base file>       # optional, for project agents that extend core
tools: Read, Edit, Write, Glob, Grep, Bash, Task  # list of allowed Claude Code tools
model: <haiku|sonnet|opus>                  # see selection criterion below
skills: [...]                               # list of model-invoked skills
---
```

**Specific decisions about fields:**

- **`extends` is new** — supports inheritance from §6 of the RDD (project extends core)
- **`Task` tool added by default** to Managers — necessary for sub-invocation of automatic peer review (Q4 deferred, we will test in the pilot)
- **`workflows` NOT added** — workflows that matter become skills; a separate field would be redundant
- **`memory` NOT added** (exists in Saintfy today) — memories should be universal per session, not scoped by agent

**Model selection criterion (part of the D3 decision):**

| Model | When to use | Default agents |
|---|---|---|
| **`opus`** | Big picture reasoning, architectural decisions, cross-project coordination, hiring loop (forming specialists well), synthesis of work from multiple agents | **Leo** (always) |
| **`sonnet`** | Standard execution: writing code, reviewing, delegating, applying rules, self-QA | **Managers** (all by default: Dev, Designer, PM, Marketing) |
| **`haiku`** | Low-reasoning mechanical tasks: formatters, lint fixers, mass renaming, template generators, simple converters | **Mechanical specialists** (when any) |

Practical rules:
1. **Default is sonnet.** If there is no explicit reason to be haiku or opus, it is sonnet.
2. **Leo is always opus** because coordination + big picture are reasoning-intensive, and Leo is invoked few times per session (cost justified).
3. **Haiku only goes into specialists** that do provably mechanical work. Never default for a Manager — the risk of under-thinking in a delegation decision is too high for marginal savings.
4. **Opus in individual Managers** only if there is empirical evidence that sonnet is making mistakes in that domain's decisions specifically. The pilot will inform this.
5. **Projects can override the core's model** via `extends`: the project agent declares `model: opus` and overrides the default. Useful if a project has specific pain that justifies the higher cost.

### D4 — Fixed internal structure of Managers

Every Manager file follows this section order (exact names in Portuguese):

1. **Papel** — 1-2 sentences. What the agent is, when it delegates, when it executes in exception
2. **Princípios** — short bullets. The 3-5 central principles of the role
3. **Hiring loop** — 1-2 sentences about when to trigger specialist hiring
4. **Self-QA** — discipline-specific proof checklist
5. **Escalation** — concrete list of what stops the agent and forces a question to the founder

Extra sections only if justified by the nature of the domain. Default is to keep the 5 above.

### D5 — Initial Managers: **4 (Dev + Designer + PM + Marketing)**

To write first:

| Manager | Why now | Main input for writing |
|---|---|---|
| **Dev Manager** | Saintfy and logbook both need dev; most acute pain observed | `~/Github/Saintfy-Copilot/.claude/agents/developer.md` + Saintfy CLAUDE.md + dev memories |
| **Designer Manager** | Saintfy has a massive design system; logbook needs store assets | `~/Github/Saintfy-Copilot/.claude/agents/designer.md` + `.claude/rules/design-system.md` |
| **PM Manager** | PRD→RDD flow is established (memory `feedback_doc_canonical_locations`) and will continue across all projects | `~/Github/Saintfy-Copilot/.claude/agents/pm.md` + updated `workflows/prd.md` |
| **Marketing Manager** | Saintfy (Instagram/ASO) and logbook (App Store listing) both need it | `~/Github/Saintfy-Copilot/.claude/agents/marketer.md` |

Leo (Manager of Managers) also enters this first batch — he is a prerequisite for any delegation to work.

**Not to write yet:** Research Manager, Writing Manager. They come in when there is real pain from their absence.

### D6 — Project initialization flow

Founder's vision, documented literally:

> "I imagine the following as installation: I have some kind of `copilot init` like you mentioned, that generates an initial copilot setup. In that setup, it will practically clone the copilot-core repo and will ask for the repo (or repos if it's a big project) that the Copilot will manage. With that, it already assembles the initial structure and already links with the code repos themselves. From there, the next step would be to collect more project context, it could be some kind of interaction already via Claude Code, where the user could send some files (doc, md, pdf, whatever) and the Leo of this new Copilot already makes a first version of context and finishes doing the setup."

**Detailed flow (for the implementation session):**

1. **Invocation** — founder runs `copilot init` in some terminal (concrete command implementation is a deferred detail)
2. **Machine bootstrap (first time only)** — if `copilot-core` is not cloned in `~/Github/copilot-core/` nor synced in `~/.claude/`, init:
   - Clones `git clone <url> ~/Github/copilot-core`
   - Runs `bash ~/Github/copilot-core/scripts/sync.sh` (see D8) to populate `~/.claude/` with symlinks
3. **Project setup** — asks the founder:
   - Path to the project's main repo
   - Paths to additional repos if it's a multi-repo project (e.g., Saintfy = saintfy/ + saintfy-web/)
   - **Interaction language** of Leo with the founder (default: PT)
   - **Language of the project files** — code, docs, PRDs, RDDs (default: EN)
   - These two choices are saved in `.claude/project-config.yml` and are read by Leo at the start of each session
4. **Scaffolding** — creates in each project repo:
   - `.claude/agents/managers/` empty (ready to extend)
   - `.claude/rules/` empty (ready for project-specific rules)
   - `.claude/context/project.md` — empty template with sections to fill
   - `.claude/specialists/` empty (hiring loop will populate)
5. **Interactive context collection** — opens a Claude Code session and puts Leo in the interviewer role:
   - Leo asks the founder to share existing context files (PRDs, docs, README, pitch, anything)
   - Founder throws files into the session (paste, paths, or upload if Desktop)
   - Leo reads everything, synthesizes first version of `context/project.md`
   - Leo asks 3-5 calibration questions if something stayed ambiguous (stack, domain, audience, deadlines)
6. **Setup closure** — Leo confirms with the founder that the captured context is correct, commits the initial scaffolding in the project repo, and reports "ready to work"
7. **Normal use** — from there on founder talks normally, Leo and managers operate, context enriches organically

**Core update (founder on his machine):**

- `cd ~/Github/copilot-core && git pull` — updates source files
- Thanks to the D8 symlinks, updates are **immediate** — no need to re-run sync.sh if only content changed
- Re-run sync.sh only when core topology changes (new or removed files)
- All projects on that machine pick up the new version in the next session

**Multi-machine:** each new Mac needs to run `copilot init` once (for bootstrap). From there on, updates are `git pull + sync`.

### D7 — Hiring loop enforcement: how to "force" the model to recognize gaps

**Problem:** Claude as a model "knows" how to do almost everything superficially. A Dev Manager instructed to implement APNs will try — because it has superficial knowledge of the domain from training. This is exactly the failure mode that the hiring loop should prevent.

The founder raised this point explicitly: "we will need, somehow, to 'force' the model to identify this". It is not enough to have a rule saying "ask for specialist when you don't know" — the model will think it knows.

**Three mechanisms that will be combined in the `know-what-you-dont-know` rule:**

#### Mechanism 1 — Mandatory self-interrogation before executing code

Before writing any line of code, the Manager MUST fill out this mental form and **paste the response in the output** (not just think — write):

```
## Pre-execution check (mandatory)
- What is the specific technical domain of this task? (1 sentence)
- Do I have a specialist on my team with a playbook for this domain?
  [ ] Yes → which specialist, reference to the file
  [ ] No → STOP, trigger hiring-loop
- If I get this task wrong from lack of expertise, what is the worst case?
  [ ] Bug easy to revert → can execute with care
  [ ] Bug hard to revert or catastrophic → STOP, specialist mandatory
- Is my confidence in this domain high or low?
  [ ] High and I know why (cite the specialist that covers) → execute
  [ ] Low OR high without a citable source → STOP
```

The obligation to **write** the answer (not just think) is the trick — it forces the model to materialize meta reasoning that it normally skips. If the model tries to dodge by writing "yes" without a source, the founder sees it and calls it out.

#### Mechanism 2 — Trust gradient per task category

Rule defines categories with different default trust. Some NEVER execute without a specialist. Example for Dev Manager:

| Category | Default trust | Specialist mandatory? |
|---|---|---|
| Editing text/copy/static JSON | High | No |
| Standard CRUD with known ORM/library | High | No |
| Pure UI (new component, style, layout) | High | Only if project-specific design system |
| Database migration | Medium | Yes for complex schema, no for simple add column |
| Integration with external API | Medium | Yes if it's a protocol (APNs, OAuth, WebAuthn), no if it's common REST |
| Crypto / auth / security | **Low** | **Always** |
| Native bridging (Capacitor, React Native) | **Low** | **Always** |
| Infra / deploy / CI | **Low** | **Always** |

This table is **Manager-specific** (Dev has his, Designer has another, etc.). It lives inside the Manager's agent file in the "Trust gradient" section. The "Always specialist" category is the hard list — even under pressure, Manager does not execute.

#### Mechanism 3 — Post-failure hardening

When the Manager executed without a specialist and made a mistake (detected in peer review, or worse, in production), that failure becomes automatic input to the trust gradient. The `propagation` rule comes in here:

- Peer review detected that Manager erred in domain X because a specialist was missing
- Leo propagates: adds X to the "Always specialist" list of the Manager in the project
- In the next session, Manager has the trust gradient updated

This turns failures into automatic enforcement — each error corrects the system so as not to repeat. It is the opposite of the current pattern where memories went stale.

#### Mechanism 4 — Lessons learned pass after failure (new, inspired by autoresearch)

When peer review rejects work from a Manager or specialist, before simply correcting the task, the agent runs a **lessons learned pass**:

```
## Lessons learned (mandatory after peer review rejection)
- Which rule or checklist item would have prevented this failure?
- Is this lesson specific to the current project or universal?
- If universal: propose to Leo addition to the core agent file
- If specific to the project: propose to Leo addition to the project's extended agent file
- Proposals go to the founder via R2 before being applied
```

This institutionalizes learning by failure. Without this step, lessons stay in the founder's head (until forgotten) or in memories that can go stale. With this step, each failure has a chance to become permanent enforcement.

**Implication for writing the `know-what-you-dont-know` rule:** these 4 mechanisms are mandatory requirements. When Q2 is addressed, the rule needs to describe:
- Pre-execution check template (mechanism 1)
- Format of the trust gradient in the agent file (mechanism 2)
- Post-failure hardening process (mechanism 3)
- Lessons learned pass template (mechanism 4)

### D8 — sync.sh: concrete design

**Problem:** founder wants core updates via simple `git pull`, without breaking local files in `~/.claude/` (memories, settings, projects, etc.), working multi-machine, and recoverable via rollback.

**Options evaluated:**

| Option | How it works | Why rejected |
|---|---|---|
| `rsync --delete` | Copies core to `~/.claude/`, deletes orphans | `--delete` is dangerous; a bug can erase the founder's memories |
| Git submodule in `~/.claude/` | `~/.claude/` becomes partially a git checkout | Mixes user state with core content; submodule UX is bad; confuses Claude Code |
| Symlink of the whole directory | `ln -s core/agents ~/.claude/agents` | Replaces the entire directory — founder loses local agents if any exist |
| **Per-file symlinks (recommended)** | Script symlinks each individual file from the core to the corresponding locations in `~/.claude/` | Works with Claude Code's flat loading, preserves local files, git pull = immediate update, idempotent |

**Chosen design: `sync.sh` with per-file symlinks**

Script lives in `~/Github/copilot-core/scripts/sync.sh` (inside the core repo, so it's automatically available on any machine that clones the repo).

Behavior:

```bash
#!/bin/bash
# sync.sh — idempotent sync from copilot-core to ~/.claude/
set -e

CORE_DIR="${CORE_DIR:-$HOME/Github/copilot-core}"
CLAUDE_DIR="$HOME/.claude"

# Sanity check
if [ ! -d "$CORE_DIR" ]; then
  echo "Error: copilot-core not found at $CORE_DIR"
  echo "Clone it first: git clone <url> $CORE_DIR"
  exit 1
fi

mkdir -p "$CLAUDE_DIR/agents" "$CLAUDE_DIR/rules"

# Symlink every markdown file from core agents → ~/.claude/agents/
# Uses find to recurse in case managers/, specialists/, etc.
find "$CORE_DIR/agents" -type f -name "*.md" | while read src; do
  basename=$(basename "$src")
  ln -sf "$src" "$CLAUDE_DIR/agents/$basename"
  echo "synced agent: $basename"
done

# Symlink every rule file from core rules → ~/.claude/rules/
find "$CORE_DIR/rules" -type f -name "*.md" | while read src; do
  basename=$(basename "$src")
  ln -sf "$src" "$CLAUDE_DIR/rules/$basename"
  echo "synced rule: $basename"
done

# Clean up dangling symlinks (files removed from core on pull)
find "$CLAUDE_DIR/agents" -type l ! -exec test -e {} \; -delete 2>/dev/null || true
find "$CLAUDE_DIR/rules" -type l ! -exec test -e {} \; -delete 2>/dev/null || true

echo ""
echo "✓ Sync complete."
echo "Future updates: cd $CORE_DIR && git pull"
echo "Re-run sync.sh only if new files were added to core."
```

**Important properties:**

1. **Idempotent.** Safe to run N times. `ln -sf` overwrites existing symlink, doesn't error.
2. **Zero-copy after the first run.** Once the symlinks are created, `git pull` in the core is enough for an update — the symlinks point to live files in the repo.
3. **Re-run only when topology changes.** If the core adds a new `agents/managers/research.md`, founder runs `sync.sh` to create the new symlink. If the core only edits the content of an existing `dev.md`, zero work — the symlink already points to the file.
4. **Local files preserved.** Memories in `~/.claude/memory/`, settings in `~/.claude/settings.json`, projects in `~/.claude/projects/` — none of that is touched.
5. **Trivial rollback.** `cd ~/Github/copilot-core && git checkout <rev>` — symlinks follow automatically. If the checkout removed files, running sync.sh cleans up dangling symlinks.
6. **Dangling cleanup.** Find with `! -exec test -e` detects symlinks whose target was removed and cleans them — avoids clutter.
7. **Multi-machine.** Each new Mac: `git clone <core-url> ~/Github/copilot-core && bash ~/Github/copilot-core/scripts/sync.sh`. Two lines, done.

**Potential conflict with local files of the same name:** if the founder has a local `~/.claude/agents/dev.md` and the core also has `dev.md`, the symlink overwrites the local. Solution: core uses distinctive names (e.g., `core-dev-manager.md`) OR founder uses a local subdirectory that doesn't conflict. **Decision:** core uses clean names (`dev.md`, `designer.md`), founder avoids conflicts by keeping custom local agents with unique names (`dev-experimental.md`). Edge case, unlikely in practice.

**When to actually write it:** together with the creation of the `copilot-core` repo in the pilot session. Not before — no point without having content in the repo to sync.

### D9 — Outcome metrics (inspired by autoresearch)

Karpathy's autoresearch insists on a measurable fitness function as a prerequisite for auto-refinement. Our architecture so far has not had metrics. We will collect them from the pilot to have real data to refine the core.

**5 basic metrics to collect from the logbook pilot:**

| Metric | What it measures | How to collect |
|---|---|---|
| **Peer review pass rate** | % of tasks that pass peer review on the first try | Review instance logs approval/rejection |
| **Founder rejection rate** | % of deliveries the founder rejects saying "not what I asked for" | Leo logs when the founder rejects the final synthesis |
| **Self-QA honesty rate** | % of tasks where the agent's self-QA was honest (not "said it passed but failed in review") | Compare self-QA output with review result |
| **Rework cycles** | Average number of back-and-forths per task before approval | Leo counts iterations per task |
| **Hiring loop hit rate** | % of tasks where Manager recognized a gap correctly (reported to Leo) vs tried without specialist and broke | Compare hiring requests with failures in uncovered domains |

**Where the logs live:** `~/Github/<project>/.claude/metrics/<YYYY-MM>.jsonl` — file per month, one entry per task. Simple, readable, greppable format. Outside `outputs/` because it is continuous operational metric, not a work artifact.

**How it becomes refinement:** after ~20-30 tasks in the pilot (2-4 weeks), founder and Leo review the metrics together. Where are the worst numbers? That's the area that needs refinement in the core. Avoids "guesswork" about what is wrong.

**Active decision (not parking lot):** metrics enter the universal rules to be written in Q2. Needs a `metrics-collection.md` rule that all agents load and respect.

### D10 — Agent and skill auto-refinement (inspired by autoresearch, two horizons)

You brought Karpathy's autoresearch as a question: "does it make sense to leverage it to auto-train agents and skills?". The honest answer has **two different horizons**, because autoresearch is a paradigm that applies in two ways in our context:

#### Horizon 1 — Online learning (during real use)

Already covered by D7 Mechanism 3 (post-failure hardening) + D7 Mechanism 4 (lessons learned pass). Each real failure detected in peer review becomes a proposal for agent file refinement, validated by the founder via R2, applied.

**Status:** baked into this session. Part of D7.

#### Horizon 2 — Offline auto-refinement (deliberate training loop)

What you originally read in autoresearch. The idea is:

1. Founder chooses a Manager or skill to refine (e.g., Dev Manager)
2. Prepares a **benchmark** — set of representative tasks from the domain with "expected answers" or success criteria
3. Runs the current Manager against the benchmark, measures result
4. Another instance of the agent (or the founder via Claude) analyzes the results, proposes changes to the agent file
5. Applies change, re-runs benchmark, compares
6. Keeps if improved, discards if worsened
7. Iterates until convergence or diminishing returns

**Why this has value:** allows refining a Manager **before** putting it in production, or **between projects**, without depending on waiting for real failures to appear. It is the equivalent of "training the team before the season starts" — normal professional practice.

**Why it's NOT MVP:** three concrete reasons:

1. **We need metrics first.** Without D9 implemented, there is no way to measure "improved or worsened". Horizon 2 depends on D9 working.
2. **We need a benchmark.** Building a representative benchmark for each Manager is work — it involves collecting past tasks, defining criteria, validating that they are realistic. Premature optimization before the pilot is running.
3. **We need volume of data.** Refining something without having run it in production becomes shooting in the dark. Even if the loop is closed, what is "better" depends on what happens in real use.

**When to make it active:** after the logbook pilot produces ~1 month of real data (Q7). With D9 metrics and usage feedback, we can build a benchmark for Dev Manager (the discipline where we have the most observed pain) and run the first offline refinement loop. If it works, replicate to the other Managers.

**Status:** added as a post-pilot next step. It is not an indefinite parking lot — it is a parking lot with a clear trigger (pilot + 1 month + enough metrics).

### D11 — Parking lot updates

Added to the RDD parking lot (§9):

- **Style configurable per project**: founder suggested that verbosity (minimalist vs verbose) could be configurable via `project-config.yml`. Decision: **not now**. If one day a project needs a different style (e.g., a formal corporate project that requires explanatory prose), add it. For now, minimalist is baked into the core.
- **Tone configurable per project**: tone is decided (casual) but could become config in the future if someone open-sources uses it in a formal corporate context. Not now.
- **Workflow field in the frontmatter**: considered and rejected due to redundancy with skills. If this becomes useful again in the future (e.g., workflows that are not skills), reevaluate.
- **Language configured per project is an active decision (D6), not parking lot.** Confirmed that it will be real config at setup.
- **Offline agent auto-refinement (D10 horizon 2)**: not an indefinite parking lot. Has a trigger: logbook pilot + 1 month of D9 metric data. After that, first experimental Dev Manager refinement loop.

---

## Deferred to future sessions

These decisions were left **intentionally open** in this session:

- **Q2 — Exact content of the 10 universal rules**: founder trusts Leo to write the draft, he reviews. Next implementation session.
- **Q3 — Adversarial prompt for review mode**: part of the `peer-review-automatic` rule. Leo drafts, founder reviews. Possibly with a "core + per-domain specification" structure.
- **Q4 — Technical mechanism of sub-invocation**: test Claude Code's native Task tool in the pilot. If it works, lock it. If not, rethink.
- **Q7 — Logbook pilot strategy**: decide after having the 4 managers written
- **Q8 — Saintfy migration**: decide after the pilot validates the model

**Q5 (sync.sh) left the deferred list** — resolved in D8.
**Q6 (project initialization) left the deferred list** — resolved in D6.

---

## Critical files for the implementation session

**Manager sources (to read when starting to write):**
- `~/Github/Saintfy-Copilot/CLAUDE.md` — identity + current rules of Leo + Tomé
- `~/Github/Saintfy-Copilot/.claude/agents/developer.md` — base Dev Manager
- `~/Github/Saintfy-Copilot/.claude/agents/designer.md` — base Designer Manager
- `~/Github/Saintfy-Copilot/.claude/agents/pm.md` — base PM Manager
- `~/Github/Saintfy-Copilot/.claude/agents/marketer.md` — base Marketing Manager
- `~/Github/Saintfy-Copilot/.claude/rules/propagation.md` — already existing universal rule
- `~/Github/Saintfy-Copilot/.claude/rules/design-system.md` — for Designer Manager
- `~/Github/Saintfy-Copilot/.claude/rules/paper-artboards.md` — for Designer Manager (will be generalized to "artboard-conventions" without tool mention)

**Memories to consult as input:**
- `feedback_pr_workflow` — base of the Dev Manager's PR-first principle
- `feedback_real_callsite_first` — Dev Manager principle
- `feedback_doc_canonical_locations` — base of the PM Manager's PRD→RDD flow
- `feedback_strategy_before_processing` — base of the universal `think-before-execute`
- `feedback_no_inventing_design` — Designer Manager principle
- `feedback_reusable_components` — Dev Manager principle
- `feedback_shadcn_first_enforcement` — **DOES NOT** go in the core (Saintfy-specific), stays in the extension

**Architectural source:**
- `~/Github/Saintfy-Copilot/docs/rdds/2026-04-08-copilot-core-architecture/rdd.md` — complete spec

---

## Verification (after Manager implementation session)

When the 4 managers + Leo are written, verify:

1. **Structural consistency** — each file follows the 5 fixed sections (Papel, Princípios, Hiring loop, Self-QA, Escalation) in order, with the exact names
2. **Valid frontmatter** — all 6 fields correct, valid YAML, `model: sonnet` default
3. **Tone and style** — fast read (founder reads each manager in <3 minutes and understands the role)
4. **Zero mention of a specific project** — no core file mentions "Saintfy", "logbook", specific stack, person name, credential, ID
5. **Cross-reference** — `extends` paths make sense; cited domain rules exist or are on the "to create in Q2" list
6. **Conceptual extension test** — can I mentally imagine how Saintfy would extend the Dev Manager (adding shadcn-first) without conflict?

Empirical verification only becomes possible with the pilot (Q7), which depends on having the Managers + some universal rules ready.

---

## Next steps after this plan is approved

1. **Manager implementation session** — write Leo + 4 Managers following D1-D5. Output: 5 files in `Saintfy-Copilot/docs/rdds/2026-04-08-copilot-core-architecture/draft-managers/` (draft for review, not yet the final core because the `copilot-core` repo doesn't exist)
2. **Universal rules session (Q2)** — Leo drafts the 10 universal rules + `metrics-collection.md` (new, D9), founder reviews
3. **Pilot decision (Q7)** — with managers + rules ready, decide pilot scope on logbook
4. **Pilot** — create `copilot-core` repo, populate with approved content, run sync.sh (D8) to activate in `~/.claude/`, test on logbook
5. **Pilot-based adjustments** — Q4 (sub-invocation mechanism via Task tool) is resolved here
6. **Metrics collection (D9)** — during ~1 month of real use, accumulate peer review pass rate, hiring loop hit rate, etc. data
7. **First auto-refinement loop (D10 horizon 2)** — with metrics in hand, build a Dev Manager benchmark and run autoresearch-style offline refinement loop
8. **Saintfy migration (Q8)** — only after the pilot validates the model

This plan closes Block 1. The next plan (Manager implementation session) will be active (creates files), not passive (only decides).
