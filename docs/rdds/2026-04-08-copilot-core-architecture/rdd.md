# RDD: Copilot-Core Architecture

**Status:** `Planning — awaiting open decisions and pilot`
**Author:** `Saintfy Copilot (Leo)` conducting conversation with founder (Vinícius)
**Date:** `2026-04-08`
**Application:** `Saintfy-Copilot` (working method) → future `copilot-core` repo
**Tracking:** vmarinogg/Saintfy-Copilot#1

---

## Summary

This document describes the proposed architecture for a replicable "copilot" system based on specialized agents, universal rules, and on-demand playbooks. The goal is to turn the working method built empirically in Saintfy over the course of weeks into a generic, reusable layer usable by any of the founder's projects, without sacrificing the specialization each project requires.

This document is **not** an implementation plan. It is a record of architectural decisions made during a planning session on 2026-04-08, with open points explicitly marked for future sessions.

---

## 1. Context and motivation

### 1.1. The problem

The founder operates multiple projects simultaneously — Saintfy (native Catholic app), logbook (iOS training app), and others on the horizon. Each project has its particularities but shares a **working method**: conversation with a "manager" (Leo) who delegates to specialists, process rules, canonical decisions, PR-first flow, context propagation.

Today that method lives entirely inside `Saintfy-Copilot`. When the founder opens Claude in another project (logbook, for example), it starts from scratch: no Leo, no rules, no structure. The result is frustrating — the "raw" version of Claude invents unnecessary abstractions, repeats mistakes already fixed in other projects, and loses the philosophical context that makes the work have quality.

Beyond that, within Saintfy itself, the current architecture has structural limits:

- **Monolithic agents.** Tomé (dev) has a single agent file that tries to cover frontend, backend, native Capacitor, Supabase, deploy. In practice, it fails in every domain because it doesn't have a specialized playbook loaded at the right moment. The Apple Sign-In + Push Notifications marathon (2026-04-06/07) had 10 chained bugs that could have been avoided with specific playbooks.
- **Memories as a dumping ground.** Several of Leo's memories are actually skills in disguise — specialized technical playbooks loaded in every session, even when irrelevant. Waste of tokens + context dilution.
- **Orphan workflows.** `workflows/` has valuable SOPs that no agent references, because they were written for invocation via `/commands` (user-invoked skills) that the founder doesn't use.
- **No replication mechanism.** Improvements to the working method made in Saintfy stay stuck in Saintfy.

### 1.2. Objectives

1. **Extract generic method into a reusable core** (`copilot-core`) that any new project can inherit.
2. **Preserve per-project specialization** — the core provides the backbone, each project extends with its stack, domain, decisions.
3. **Propagate improvements automatically** — when the founder refines the method in one project, the improvement becomes available to all the others without manual work.
4. **Solve the "generic Tomé" problem** with an agent hierarchy that separates process (Manager), subject matter (Domain Expert), and specific technical problem (Specialist).
5. **Keep the conversational philosophy** — the founder drives, the agents execute, but without turning the method into a Paperclip-style autonomous system that decides on its own.

---

## 2. Central philosophy

Before any technical decision, it was necessary to name the system's philosophy because it drives every other decision.

### 2.1. Conversational copilot vs. autonomous agent

The current market has two dominant philosophies for AI agents doing real work:

| Dimension | **Paperclip-style** | **Copilot-style** |
|---|---|---|
| Unit of work | Issue/ticket executed autonomously | Conversation between founder and agent |
| Agent model | Flat — "the agent" does the work | Hierarchical — manager delegates to specialists |
| Specialization | Via tools and integrations | Via hireable skills/playbooks |
| Where the human comes in | Creates issue, receives PR | Talks with the manager, approves each relevant step |
| Philosophy | "Delegate and forget" | "Converse and guide" |

This system explicitly adopts the **Copilot-style** philosophy. It is not competition for Paperclip — it is a philosophically opposite alternative for a different user profile.

### 2.2. Three axes of autonomy

The discussion about "how much autonomy to give agents" only becomes clear when you separate what was implicit: autonomy is not one axis, it's three.

| Axis | What it is | Who decides |
|---|---|---|
| **Strategic** | What to work on. Priorities. Project direction. The "does it serve the mission?" filter | **Always the founder.** Non-negotiable. |
| **Tactical** | How to execute what has already been strategically approved. Decomposition, delegation, ordering, execution. | **Leo.** This is literally his job as Manager of Managers. |
| **Creative/Structural** | Creating a new specialist, changing a core rule, opening a PRD, spending money, publishing externally. | **R2:** agent proposes, founder approves. No structural change happens without explicit approval. |

This resolves the apparent paradox between "I want to converse, not delegate blindly" and "I don't want to keep blocking the system at every step." The founder decides the **what**, Leo decides the **how**, and it goes back to the founder at **inflection points**. It's normal human management, just formalized for agents.

This model will be referenced as **"R2 with tactical autonomy"** throughout the rest of the document.

---

## 3. Distribution architecture

### 3.1. Where the core lives

**Decision: Option A — user-level via `~/.claude/`.**

```
~/Github/copilot-core/          ← private git repo (versioned, source of truth)
├── rules/                         (universal rules)
├── agents/                        (Leo + Managers)
├── templates/                     (boilerplate for new projects)
└── README.md

         ↓ sync.sh (rsync)

~/.claude/                      ← Claude Code reads automatically in any project
├── rules/
├── agents/
└── ...
```

When the founder improves something in the core:
1. Edits in `~/Github/copilot-core/`
2. Commits to the repo
3. Runs `bash sync.sh` (or equivalent)
4. All projects (Saintfy, logbook, future) use the new version in the next session

### 3.2. Why Option A

Considered against alternatives:

| Alternative | Why not |
|---|---|
| **Git submodule** in each project | Git UX is bad, requires remembering to update project by project, confusing. |
| **Explicit sync per project** (`copilot sync`) | Guaranteed drift between projects, fragmented source of truth. |
| **Symlinks** | Not portable across machines, fragile in git. |
| **Template copy per project** | Eliminates the benefit of global update. |

Option A wins because:

1. **Native Claude Code mechanism** — doesn't fight the tool
2. **Zero friction for new projects** — clone, open, Leo is there
3. **One single update point, one single rollback point** — if something breaks, `git checkout` on `copilot-core` + `sync.sh` and it's fixed
4. **Multi-machine is 1 command per new Mac** — `git clone copilot-core && bash sync.sh`

### 3.3. Preserving a multi-mode future

One concern raised: what about when this becomes a product/open-source? The ideal distribution changes.

| Horizon | Mode | Reason |
|---|---|---|
| **Now** (private, founder only) | Option A — user-level sync | Zero friction, personal working mode |
| **Future** (open-source / product) | Option C — per-project opt-in with versioning | Per-user control, projects pinned to stable versions |

**How to preserve both horizons in a single design:** `copilot-core` will be structured as a **pure content repo** (markdown, no logic). `sync.sh` lives **outside** the repo (founder's personal scripts). This ensures that:

- Today: founder uses it via sync.sh → `~/.claude/` (Option A)
- Tomorrow: someone else clones and uses it via their own mechanism (Option C, CLI, etc.) — **without needing to rewrite the content**

The content doesn't change. The delivery mechanism is configurable.

### 3.4. Rules about the core's content

Decided during the session:

1. **New private git repo**, created when the time comes to implement
2. **Zero mention of specific projects** in any core file — no "Saintfy", no "logbook", no "vmarino". Universal rules, neutral templates
3. **`sync.sh` script lives outside the core repo** (founder's personal scripts in `~/bin/` or `.zshrc`)
4. **Credentials, IDs, service names** never go in the core — they always stay in the project

---

## 4. Agent hierarchy

### 4.1. Three distinct types of hireable agents

During the session it was discovered that "agent" is a concept that aggregates three different things. Each deserves different treatment.

| Type | Role | Lives where | Who creates | Lifecycle | Example |
|---|---|---|---|---|---|
| **Manager** | **Tech lead** of the discipline — receives, decomposes, delegates, reviews, synthesizes. Executes only in exceptions | **Core** (universal) | Already exists in core; extended by the project | Permanent | Engineer Manager, Design Manager |
| **Domain Expert** | Subject matter consultant (what is true about X) | **Project** | Hiring Loop (proposed by the founder) | Permanent in the project | Theologian (Saintfy), Strength Coach (logbook) |
| **Specialist** | Executor of technical tasks — composes the Manager's "team" | **Project** | Hiring Loop (proposed by the Manager) | Created on demand, permanent in the project | Frontend dev, APNs protocol, SwiftData |

**Critical distinction between Manager and Specialist:**

The Manager **is not the default executor**. He is the tech lead of his team. When he receives a task from Leo, he **delegates to the specialists on his team**, reviews their work, and synthesizes the result for Leo. He can execute directly in exceptions (micro-tasks, emergency, briefing creation), but the default is delegation.

**Critical distinction between Domain Expert and Specialist:**

- **Specialist** is hired by the **Manager** to solve **technical tasks**. His content is an actionable playbook and he **executes** concrete work.
- **Domain Expert** is hired by the **founder** (via Leo) as a **permanent consultant** for the project. His content is broad knowledge about the subject, consulted in various decisions over time. He **does not execute** — he is a consultative reference.

A Engineer Manager + a Theologian collaborate: Engineer Manager leads implementation of the feature with his team of specialists; Theologian validates whether the feature aligns with doctrine. Neither replaces the other.

### 4.2. Managers in the core (initial list)

Six managers make up the "default team" that any project can use:

1. **Leo** — Manager of Managers. The only one with this role. Coordinates, delegates, has cross-project big picture.
2. **Engineer Manager** — Software development process. Domain rules: PR workflow, systematic debugging, real callsite first, self-QA with proof.
3. **Design Manager** — Design process. Domain rules: source of truth, don't invent undefined elements, design system as authority.
4. **Marketing Manager** — Marketing and growth process. Domain rules: ASO fundamentals, content calendar, brand voice.
5. **Research Manager** — Research process. Domain rules: source credibility, primary vs secondary, synthesis.
6. **Writing Manager** — Writing and communication process. Domain rules: voice by audience, structure, CTA.
7. **Product Manager** — Product process. Domain rules: feature filter, PRD→RDD handoff, scope creep detection.

Notes:

- The structure **supports "hiring" new managers** via conversation with Leo. Example given by the founder: a manager with "theological formation" to validate Saintfy content, or a "strength coach" for the logbook. Domain Experts cover most of those cases, but a new manager makes sense if it is a recurring **professional discipline**, not a body of knowledge.
- "DevOps manager" or "Data manager" were not included initially — there is no real pain in those domains in current projects. They can be added via hiring loop when they arise.
- **Each Manager's minimum team grows organically, not by anticipatory decision.** The core delivers the Managers "empty of team". In the first interactions with a project, the Manager identifies which specialists he needs based on the stack and real tasks, and triggers Hiring Loop with Leo to hire them. No specialist is pre-defined in the core. This keeps the core lean and honest with the principle "nothing in the core without evidence of real use".

### 4.3. Leo — Manager of Managers

Leo has a unique role and is not "just another manager". Exclusive responsibilities:

1. **Routing** — receives requests from the founder, identifies the domain, delegates to the right manager
2. **Hiring Loop contractor** — when a manager reports a gap, Leo hires the specialist (see §4.4)
3. **Cross-project big picture** — Leo sees other projects in `~/Github/` when the task requires reference (e.g., "did we already implement push in another project?")
4. **Context propagation** — Leo is ultimately responsible for ensuring decisions propagate to memories, canonical docs, relevant rules
5. **Synthesis for the founder** — consolidates the managers' work into actionable reports

### 4.4. Hiring Loop

Central decision: **recognizing a gap** and **filling a gap** are separate responsibilities, assigned to different roles.

```
Manager:  "I need someone who knows APNs protocol deeply"
       ↓ structured request (what, what for, scope)
Leo:      sees big picture (projects, existing specialists in other
          projects, memories, decisions, constraints)
       ↓ formats the correct specialist
Leo:      "hired. Specialist `apns-push-protocol` created.
          Handing back to Engineer Manager."
Manager:  delegates the task to the specialist, who executes
```

**Hiring Loop is used in two distinct cases:**

1. **Constitute the Manager's initial team** — in the first interactions with a project, the Manager identifies which generalist specialists he needs (e.g., frontend dev, backend dev) and triggers Hiring Loop to constitute his basic team.
2. **Fill a specific domain gap** — when a task requires deep expertise that the current team doesn't cover (e.g., APNs protocol, WebCrypto), Manager triggers Hiring Loop to hire a specific specialist.

In both cases, specialists **always live in the project**, never in the core.

**Why Leo hires and not the Manager himself:**

1. **Leo sees duplication** — if another manager has already asked for a similar specialist, Leo remembers. A manager alone doesn't have that cross-view.
2. **Leo sees cross-project reuse** — if another project already has a similar specialist, Leo proposes adapting it.
3. **Leo imposes structural standard** — frontmatter, format, level of detail. Avoids messy specialists.
4. **Manager stays focused** — asked, went back to executing. Doesn't lose context on the meta-task of "writing a specialist".

This mirrors how headhunting works in real companies. Engineering manager says "I need senior iOS with push". HR/CTO writes JD, searches, interviews, hires, delivers. Manager executes. Separation of concerns.

**Critical additional rule:** Managers **stop and report to Leo before executing** when they recognize that the domain is outside their capability. This is made explicit as the universal rule `know-what-you-dont-know`, applicable to all agents. It is a direct antidote to the "Tomé thought he knew and didn't" pattern — the main cause of the 2026-04-06/07 bug marathon.

### 4.5. Manager as tech lead: delegation and peer review

The Manager is not the default executor — he is the **tech lead** of his team. The default flow of a task follows this sequence:

```
Founder → Leo → Manager receives
                    ↓
                 Manager decomposes + decides which team specialists to use
                    ↓
                 Manager delegates with briefing
                    ↓
                 Specialist executes → self-QA → reports to Manager
                    ↓
                 Manager reviews (natural peer review, same domain, more senior)
                    ↓ approves → synthesizes to Leo
                    ↓ rejects → back to specialist with comments
                 Leo → Founder
```

**Review is embedded in the Manager's role.** It is not an extra step nor a separate agent — it is part of what it means to be tech lead of the discipline. Manager reviews a specialist from the same domain because he has the expertise to do it rigorously.

#### Exception: when the Manager executes directly

In some cases it makes sense for the Manager to execute instead of delegating:

- Task so small that creating/hiring a specialist becomes overhead (changing a color, renaming a file)
- Meta task that is inherently the Manager's (planning decomposition, writing briefing for specialist)
- Emergency where the specialist isn't available and Manager needs to take over

In those cases, review happens via **transparent sub-invocation of the Manager himself in a new instance**, with context isolation:

```
Manager executes small task → self-QA
    ↓
Manager triggers sub-invocation of himself in review mode
    ↓ (same founder session, no manual intervention,
       analogous to how Claude Code triggers sub-agents via Task tool today)
New Manager instance receives only the output (diff + self-QA),
WITHOUT access to the execution's context/reasoning
    ↓
Reviews adversarially, approves or requests adjustment
    ↓
Result goes back to the founder's main session, who receives
everything together: execution + review + final result
```

**Critical property:** the founder **never opens another session manually**. The entire execution + review cycle happens inside the session the founder is working in, transparently. The founder sees the progress (analogous to seeing a sub-agent running), receives the final result, and done.

Context isolation of the reviewer instance is mandatory — without it, confirmation bias returns ("I decided this way because X, Y, Z" → reviewer reads and agrees). The new instance receives only the artifacts: changed files/diff, self-QA report, and the review context ("you are [Manager name] in review mode, be adversarial, look for bugs that self-QA doesn't catch").

#### Why the same Manager in review mode (and not a separate Reviewer in the core)

The option of having Reviewers separate 1:1 with Managers in the core was considered. Rejected because:

1. **Fragments knowledge.** Updating the Engineer Manager required updating the Dev Reviewer in parallel. Guaranteed drift.
2. **One single source of truth.** Technical expertise lives in a single file. Invocation mode changes the lens, not the content.
3. **Mirrors real companies.** There is no "iOS Reviewer" position — it's a senior iOS reviewing another iOS's code. Same person, same expertise, different role.
4. **Project extends once.** `.claude/agents/managers/engineer.md` with `extends: core/managers/engineer.md` serves for execution AND review. No duplication.

---

## 5. Rules in two scopes

### 5.1. The scope discovery

During the session, it became clear that "rule" is a category that aggregates two very different things:

- **How the company works** — philosophical principles that apply to any agent in any domain
- **How that team works** — technical practices specific to a professional domain

Mixing the two in the same basket generates:
- Token overhead (marketing doesn't need to load PR workflow)
- Confusion about what is universal vs specific
- Difficulty evolving one without affecting the other

### 5.2. Universal rules (core/rules/)

Loaded **always**, in any session, for any active agent. They define the "constitution" of the system.

| Rule | What it imposes |
|---|---|
| `propagation.md` | Every decision/change must propagate to relevant memories, context, rules before closing the task |
| `anti-hallucination.md` | A wrong answer is 3x worse than "I don't know". Mark `[INFERRED]` when it didn't come from a verifiable source |
| `think-before-execute.md` | On ambiguous/complex tasks, ask before implementing. On direct ones, go directly. Clear criterion |
| `evidence-over-claim.md` | Never report work as complete without attached verifiable evidence. Each domain defines its form of evidence (build/test/lint for dev, screenshot for design, complete draft for marketing, cited sources for research, etc). The founder should not need to believe — he should be able to verify |
| `peer-review-automatic.md` | All work goes through peer review before reaching the founder. Review is done by another instance of the same Manager (review mode, isolated context, adversarial), triggered automatically via transparent sub-invocation — founder never opens another session manually |
| `state-vs-learning.md` | State memories age fast, learning memories remain. State needs to be revalidated before citing |
| `hiring-loop.md` | Manager reports gap → Leo hires specialist → hands back to Manager. Used both to constitute initial team and to fill specific domain gaps |
| `know-what-you-dont-know.md` | Manager detects domain outside his capability → stops and reports the gap BEFORE executing |
| `escalation-triggers.md` | Explicit list of situations that always stop the agent and force a question to the founder (spending money, external publishing, destructive action, creating a specialist/manager, changing a core rule, contradiction between existing rules) |
| `inheritance.md` | When an agent has `extends` in the frontmatter, load the base file before executing and concatenate behavior |

### 5.3. Domain rules

They live **inside the corresponding manager's agent file**, either embedded in the markdown itself or in files referenced by the frontmatter. They load **only when** Leo delegates to that manager.

Examples from the Engineer Manager:
- PR workflow (worktree + branch + PR + Closes #N)
- 3-strikes debugging
- Real callsite first
- Specific self-QA checklist: build output, lint, type check, proof of execution of the real code path
- Code review checklist

Examples from the Design Manager:
- No inventing design elements
- Design system as source of truth
- Artboard conventions (tool-agnostic)
- Specific self-QA: comparison screenshot with spec, link to artboard, token verification

Examples from the Product Manager:
- Feature filter (does it serve the mission?)
- PRD→RDD handoff
- Scope creep detection
- Specific self-QA: PRD with all sections filled, traceable links, explicit decisions

Note on self-QA: the universal rule `evidence-over-claim` requires **that there be evidence**. The type of evidence and the specific checklist live as a domain rule inside each Manager, because the way of proving that dev worked is different from the way of proving that designer worked.

### 5.4. Project-specific rules

`shadcn-first-enforcement` (Saintfy), `swiftui-conventions` (logbook), and equivalents stay in the **project**, not in the core. The core imposes no specific stack.

---

## 6. Inheritance — how project extends core

### 6.1. Form 1: extends via frontmatter

**Decision:** project declares explicit extension via the `extends` field in the frontmatter.

```markdown
---
name: Engineer Manager (Saintfy)
extends: core/agents/managers/engineer.md
---

In addition to the rules and principles of core/managers/engineer.md, you also:

- Work with React + TypeScript + Vite + shadcn/ui + Supabase + Capacitor stack
- Follow the specific rules of shadcn-first-enforcement
- Priority debugging on native iOS (Capacitor)
...
```

**Mechanism:** when Leo delegates to a manager, he reads the project's agent file, sees the `extends`, reads the core file, concatenates both in order (core first, project after) and passes it as the final briefing.

### 6.2. Why extends won

Considered against:

- **Compile step** (script generates single file) — build step adds complexity, fragments source of truth
- **Template copy** (core becomes a template copied into the project) — kills the benefit of global update

Extends wins because:

1. **Doesn't break global update.** Core updates, project automatically pulls the new content in the next session.
2. **Explicit over magic.** You read the project file, see the `extends`, know exactly what will be concatenated.
3. **Extends instead of overwriting.** The architecture's philosophy is clear: project **adds** knowledge, never replaces the core. Bugs, inconsistencies, and quality loss stay confined to the project.
4. **Traceable.** `extends: core/managers/engineer.md` makes clear what is being inherited.

### 6.3. Universal rule that sustains the mechanism

`rules/inheritance.md` in the core instructs Leo (and any agent that delegates to another) to respect the `extends`:

> When an agent has an `extends` field in the frontmatter, load the base file before executing and concatenate the behavior. Order: core first, project after. The project cannot remove behavior from the core — only add or refine.

---

## 7. Multi-surface — Remote Control as secondary surface

### 7.1. The original problem

The founder currently uses Claude via VS Code extension, with full access to the filesystem and Copilot structure. It works well but has an obvious limit: **it only works when the founder is on the Mac**. Ideas captured in the field (conversation with parish folks, training at the gym) stay outside the system until the founder gets back to the computer.

The real pain: "I want to send a message to Saintfy's Leo from my phone and capture an idea".

### 7.2. The discovery

During the session, the official documentation for **Cowork/Dispatch** and **Remote Control** (both features of the Claude ecosystem) was investigated. Conclusion:

| Feature | Works as | Fit with Copilot model |
|---|---|---|
| **Cowork/Dispatch** | "Delegate and forget" — task executed in background, result via push notification | **Contradicts conversational philosophy.** Research preview, instability, "instructions from phone can trigger real actions". Parking lot. |
| **Remote Control** | Connects the phone to a Claude Code session running locally on the Mac — same session, multiple devices | **Preserves philosophy 100%.** Same `~/.claude/`, same project, same memories, same context. Mature, GA. |

### 7.3. Decision: Remote Control is the recommended secondary surface

**Conceptual setup:**

1. Founder runs `claude remote-control --name "Saintfy"` on the project's Mac (or leaves it running in background)
2. Leaves for parish / training / café
3. Opens Claude mobile, sees "Saintfy" in the session list
4. Talks normally — it is literally the same session from the Mac, seen from another device

**Important properties:**

- Nothing moves to the cloud. Claude keeps running locally on the Mac.
- Filesystem, MCP servers, `.claude/`, agents — everything remains available just like in the local session.
- Conversation syncs between devices in real time. Founder can start on the phone and continue in VS Code or vice versa.
- Reconnects on its own if laptop sleeps or network drops.
- Works on Pro/Max (the founder's plan covers it).

### 7.4. Implication for copilot-core: none

Because Remote Control is just a way to **access** the same local session, **nothing in the core changes**. The architecture designed in sections 3-6 works identically in VS Code, terminal, and mobile. The only addition is the `claude remote-control` command that the founder runs when he wants to activate the secondary surface.

### 7.5. Cowork stays in the parking lot

Explicit reasons not to adopt now:

1. **Research preview** — expected instability, not worth betting architecture on it
2. **Wrong philosophy** — "delegate and forget" contradicts "converse and guide"
3. **Security risk** — remote instructions triggering local actions without checkpoint
4. **Duplication** — Remote Control solves the real use case ("send a message to Leo from the phone") without the downsides

**When to reconsider:** if Cowork comes out of preview and the founder identifies a genuine "Paperclip mode" use case for massive tasks (e.g., "execute this refactor of 50 files while I have dinner"). For now, no.

---

## 8. Block 1 decisions (session 2026-04-08, part 2)

After the initial drafting of this RDD, a second session resolved "Block 1" of the open questions — decisions about form that unblock everything else. Corresponding plan file at `~/.claude/plans/snoopy-prancing-corbato.md`.

### 8.1. D1 — Manager style: Minimalist

Identity + principles + checklist, no long prose. Manager is a tech lead operating, not a tutorial for beginners. Rationale: tokens matter in long sessions, memories already load, universal rules already load — Manager doesn't need to repeat.

### 8.2. D2 — Tone: Casual, configurable language

Second person, zero corporate speak ("You are the dev tech lead" not "You are the technical leader responsible for the discipline of..."). Language is decided at project setup via `.claude/project-config.yml`, not baked into the core. Founder has a personal preference to interact with AI in PT but keep code/docs in EN — legitimate per-user/project config.

### 8.3. D3 — Frontmatter: 6 fixed fields

```yaml
---
name: <Manager name>
description: <short role sentence>
extends: <path relative to base file>    # optional
tools: Read, Edit, Write, Glob, Grep, Bash, Task
model: <haiku|sonnet|opus>
skills: [...]
---
```

- **`Task` tool** included by default — necessary for sub-invocation of automatic peer review (Q4)
- **`workflows` not added** — important workflows become skills
- **`memory` not added** — memories should be universal per session

**Model selection:**

| Model | When | Default for |
|---|---|---|
| `opus` | Big picture, coordination, hiring loop, synthesis | Leo (always) |
| `sonnet` | Standard execution: code, review, delegation | All Managers |
| `haiku` | Low-reasoning mechanical work | Mechanical specialists |

Project can override the model via `extends` — e.g., `model: opus` on the project's Engineer Manager if there is empirical evidence of sonnet making mistakes.

### 8.4. D4 — Fixed internal structure of Managers

Every Manager file follows this order:

1. **Role** — 1-2 sentences (what it is, when it delegates, when it executes in exception)
2. **Principles** — short bullets (3-5 central principles)
3. **Hiring loop** — 1-2 sentences about when to trigger specialist hiring
4. **Self-QA** — discipline-specific proof checklist
5. **Escalation** — concrete list of what stops the agent and forces a question to the founder

Extra sections only if justified by the nature of the domain.

### 8.5. D5 — Initial Managers: Leo + 4

First batch: **Leo, Engineer Manager, Designer Manager, PM Manager, Marketing Manager**. They cover all the pains observed in Saintfy and logbook. Research and Writing come in when there is real need.

### 8.6. D6 — Project initialization flow (`copilot init`)

Founder's documented vision:

1. **Machine bootstrap (first time):** clone `copilot-core`, run `sync.sh` to populate `~/.claude/` with symlinks
2. **Project setup:** asks for repo path(s), interaction language, file language. Saves to `.claude/project-config.yml`
3. **Scaffolding:** creates empty structure in `.claude/agents/managers/`, `rules/`, `context/project.md`, `specialists/`
4. **Interactive context collection:** session with Leo interviewing founder, collecting context files (PRDs, docs, README), synthesizing first version of `context/project.md`
5. **Closing:** commit the scaffolding, ready to work
6. **Normal use:** founder talks, managers operate, context enriches organically

**Core updates:** `cd ~/Github/copilot-core && git pull`. Thanks to the D8 symlinks, updates are immediate — re-run `sync.sh` only when topology changes (files added/removed).

### 8.7. D7 — Hiring loop enforcement: forcing the model to recognize gaps

**Problem identified by the founder:** Claude "knows" how to do almost everything superficially. A rule saying "ask for specialist when you don't know" is not enough — the model will think it knows.

**4 mechanisms combined in the `know-what-you-dont-know` rule:**

1. **Mandatory pre-execution check**: template that forces the Manager to paste a written response (not just think) about the task's domain, available specialist, worst-case error scenario, justified confidence
2. **Trust gradient per category**: Manager-specific table listing categories with hard "always specialist" rules (crypto, auth, native bridging, infra — for Engineer Manager)
3. **Post-failure hardening**: peer review rejection → Leo adds the gap to the project's trust gradient automatically
4. **Lessons learned pass**: after review rejection, agent fills out a "which rule would have prevented this?" form, proposes refinement to the core or the project via R2

### 8.8. D8 — sync.sh: per-file symlinks

Script lives in `copilot-core/scripts/sync.sh`. Design chosen after rejecting `rsync --delete` (dangerous), git submodule (bad UX), whole-directory symlink (erases local).

**Mechanism:** individual symlink of every `.md` from `copilot-core/agents/` and `copilot-core/rules/` to the corresponding folders in `~/.claude/`.

**Properties:**
- Idempotent (safe to run N times via `ln -sf`)
- Zero-copy after first run — `git pull` updates content via symlinks
- Re-run only when topology changes
- Founder's local files (`memory/`, `settings.json`, `projects/`) preserved
- Trivial rollback: `git checkout <rev>` in the core
- Automatic dangling cleanup

Concrete script is in the plan file.

### 8.9. D9 — Outcome metrics (inspired by autoresearch)

Karpathy's autoresearch insists on a measurable fitness function. Adding **5 basic metrics** to collect from the pilot onwards:

- **Peer review pass rate** — % of tasks that pass on the first try
- **Founder rejection rate** — % of deliveries rejected by the founder
- **Self-QA honesty rate** — % of honest self-QAs (not "passed" that failed in review)
- **Rework cycles** — average back-and-forth per task
- **Hiring loop hit rate** — % of times Manager recognized a gap vs tried without specialist

Logs live in `.claude/metrics/<YYYY-MM>.jsonl`. New universal rule `metrics-collection.md` defines format and responsibility.

### 8.10. D10 — Agent auto-refinement (two horizons)

**Horizon 1 — Online (during real use):** baked into D7 Mechanism 3 + 4. Real failures become refinement proposals via R2.

**Horizon 2 — Offline (deliberate benchmark):** replicate Karpathy's autoresearch paradigm. Founder prepares a benchmark of representative tasks from the domain → runs Manager against benchmark → agent proposes changes to the agent file → re-runs → keeps if improved. **Not MVP** — depends on metrics (D9) and volume of real data from the pilot. Trigger: ~1 month of logbook pilot with enough data.

### 8.11. D11 — Parking lot updates

Added: configurable style/tone per project (not now), workflow field in the frontmatter (rejected due to redundancy with skills), offline auto-refinement with clear trigger (pilot + metrics).

---

## 9. Open questions (after Block 1)

5 remaining questions for future sessions:

1. **Q2 — Exact content of the 10 universal rules.** Leo drafts, founder reviews. Next session.
2. **Q3 — Adversarial prompt for review mode.** Part of the `peer-review-automatic` rule. Possibly "core + specification per domain".
3. **Q4 — Technical mechanism of sub-invocation.** Test Claude Code's native Task tool in the pilot. If it works, lock it. If not, rethink.
4. **Q7 — Logbook pilot strategy.** Decide after having Managers + rules written.
5. **Q8 — Saintfy migration.** Only after the pilot validates the model.

**Resolved in Block 1:**
- ~~Q1 (exact manager content)~~ — form resolved (D1-D5), content in active implementation
- ~~Q5 (sync.sh)~~ — resolved in D8
- ~~Q6 (new project initialization)~~ — resolved in D6

---

## 9. Parking lot (future ideas)

Captured during the session so as not to lose them. **Not part of the MVP.**

- **Random names for managers per project.** Product idea: when a new project is initialized, managers receive unique names (like "gracie" for the logbook design manager). Generates personality, helps branding. Not MVP — fits when the core becomes a product.
- **`.claude/project.yml` declaring active managers.** Option B from the debate about which managers to load. Current decision was Option A (all always active, Leo decides by context) because it's simpler. When the dead weight of irrelevant managers becomes annoying, migrate to B.
- **Deterministic hooks via `update-config`.** Some memories/rules (`lint-before-accept`, `pr-workflow` regarding commits) are "enforcement behavior" that markdown doesn't guarantee. Claude Code hooks solve it — but they are separate complexity. Worth a dedicated session.
- **`cross-repo-reference.md` as formal rule.** Decided that, for now, founder asks explicitly when to remember. When cross-project reuse becomes frequent, formalize as a rule.
- **`copilot-core` as a product or open-source.** The whole design is already compatible. When the founder decides to make that jump, the content is ready — only the distribution mechanism changes (Option A → Option C with CLI/npm/etc).
- **Channels and Scheduled Tasks.** Discovered during multi-surface research. Channels forwards Telegram/Discord/iMessage messages to the Claude session. Scheduled tasks runs recurring routines. Not recommended now — worth knowing they exist.
- **`morning-brief.md` as a rule.** Suggested during the session and **explicitly rejected** by the founder: "my ideal routine is enter the board, see issues, and go asking". Documented so as not to propose again.
- **`autonomy-audit-trail.md` as a rule.** Suggested during the session and rejected after the discovery of Remote Control: there is no longer work running "without the founder seeing it", so audit trail loses purpose.

---

## 10. Next steps

**This session closes in planning.** No code or configuration was implemented during the conversation. Proposed order for future sessions:

1. **Next dedicated session (planning, continuation)**
   Resolve the 6 open questions (§8). Output: final version of this RDD, ready for implementation.

2. **Logbook pilot**
   - Create private `copilot-core` repo with minimal structure (Leo + Engineer Manager + universal rules)
   - Write `sync.sh`
   - Activate in `~/.claude/`
   - Open logbook in Claude, run a real work session
   - Validate: does Leo work? Does Engineer Manager load? Does extends work? Does the founder feel a qualitative difference vs "raw Claude"?
   - Adjust based on real observation

3. **Saintfy migration**
   After a successful pilot, decide and execute the migration strategy (open question §8.6).

4. **Core expansion**
   Add remaining managers as real demand arises. Don't create by anticipation.

5. **Iteration and refinement**
   A **replicable** working method means continuous refinement. Each new project is an opportunity to discover gaps in the core.

---

## 11. Principles that emerged during the session

Recorded so they are not lost — they may become universal rules in the future:

1. **Recognizing is different from filling.** Manager recognizes the gap, Leo fills it. Separation of responsibilities reflects real companies.
2. **Autonomy is not one axis, it's three.** Strategic always from the founder, tactical from Leo, creative/structural in R2.
3. **Extend instead of overwriting.** Project adds knowledge to the core, never removes. Bugs stay confined to the project.
4. **State memories vs learning memories.** State ages fast and needs revalidation. Learning remains. Mixing the two is debt.
5. **Philosophy drives mechanism.** Cowork is technically great but philosophically wrong for this model. Tool choice comes after philosophy choice.
6. **Outdated information is worse than missing information.** The propagation rule exists because stale memory generates silent errors — worse than having no memory at all.
7. **Manager is tech lead, not implementer.** The Manager's role is to decompose, delegate, review, synthesize — executing is the exception. His team's specialists execute. This mirrors real professional senior engineering practice.
8. **Author checks, peer validates.** Self-QA and peer review are complementary layers, never redundant. Self-QA catches what the author can check; peer review catches what the author doesn't see because of being immersed in his own reasoning. Both are mandatory.
9. **Review transparent to the founder.** The founder never opens another session manually to review work. The entire execution → self-QA → peer review → adjustment → approval cycle happens inside the session the founder is working in, analogous to how Claude Code triggers sub-agents today. The founder sees the progress and receives the final result, does not coordinate the middle of the path.
10. **Nothing in the core without evidence of real use.** Specialists, new managers, new rules — everything enters the core only after proving usefulness in a real project. Anticipatory decisions are a source of debt.

---

## Appendix A — Cross-references

**Relevant memories** (Saintfy-Copilot/memory/):
- `feedback_doc_canonical_locations` — PRDs in `saintfy/docs/prds/`, RDDs in `saintfy/docs/rdds/`
- `feedback_pr_workflow` — PR+worktree+Closes #N flow established 2026-04-07
- `feedback_real_callsite_first` — origin of the "grep real callsite before delegating refactor" principle
- `feedback_strategy_before_processing` — reason for having done this RDD before implementing
- `project_session_2026_04_06_07_recap` — marathon that exposed the limits of the current architecture
- `project_copilot_replicable` — first mention of the replicability goal

**Issue tracker:**
- vmarinogg/Saintfy-Copilot#1 — parent issue of the skills architecture

**Official documentation investigated:**
- https://support.claude.com/en/articles/13947068 — Cowork/Dispatch
- https://code.claude.com/docs/en/remote-control — Remote Control

---

**End of RDD.**

This document is the snapshot of the planning session of 2026-04-08. It is not a living source — it represents the decisions made on this date. Structural changes to the architecture generate new RDDs; minor adjustments are recorded in commits in the `copilot-core` repo when it exists.
