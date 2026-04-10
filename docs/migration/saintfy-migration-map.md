# Saintfy Migration Map

> **What this is:** A relational mapping between `~/Github/Saintfy-Copilot/` (the old monolithic copilot) and `~/Github/copilot-core/` (the new replicable system). Written by Leo-Saintfy for Leo-Core to use when onboarding Saintfy as a new copilot-core project.
>
> **How to use:** Read this top-to-bottom when setting up `~/Github/saintfy/.claude/`. For each section, it tells you what exists in Saintfy-Copilot, what the core already covers, what to bring over, what to skip, and why.
>
> **Important:** Saintfy-Copilot is read-only reference. Don't copy files wholesale — read, understand, adapt to the `extends` model. The goal is a lean `.claude/` in `~/Github/saintfy/` that inherits from core and adds only what's specific.

---

## 1. Agents

### What Saintfy-Copilot has

6 agents in `.claude/agents/`, each with a persona name:
- `developer.md` (Tomé) — Shadcn-first enforcement, 3-strike debugging, self-QA
- `designer.md` (Nico) — Design system source of truth, Paper MCP integration
- `marketer.md` (Gil) — Brand voice law, full draft before publishing
- `pm.md` (Davi) — PRD workflow, mission filter
- `researcher.md` (Rafa) — Hypothesis-first research, cost confirmation
- `writer.md` (Eli) — Brand tone, never invent doctrine

Plus CLAUDE.md defines Leo (the manager) inline.

### What the core already covers

4 Managers in `agents/managers/` (Dev, Designer, PM, Marketing) + Leo in `agents/leo.md`. Each has: Role, Principles, Hiring loop, Self-QA, Escalation — generic, stack-agnostic.

### What to do

| Saintfy-Copilot agent | Core Manager | Action |
|---|---|---|
| `developer.md` (Tomé) | `dev.md` | **Extend.** Create `saintfy/.claude/agents/managers/dev.md` with `extends: ~/.claude/agents/dev.md`. Bring over: Shadcn-first enforcement (the 5 grep checks + lint-shadcn.sh), Capacitor-specific rules, Supabase patterns, React/TypeScript conventions. Don't bring: generic debugging rules (core has 3-strikes), generic self-QA (core has it), generic PR workflow (core has it). |
| `designer.md` (Nico) | `designer.md` | **Extend.** Bring over: design system 3-layer source of truth (index.css → Paper → .md), Paper MCP integration rules, specific token paths. Don't bring: generic "never invent" (core has it), generic "screenshot before modifying" (core evidence-over-claim covers it). |
| `marketer.md` (Gil) | `marketing.md` | **Extend.** Bring over: Saintfy brand voice specifics (direct, imperative, fraternal, no coach-speak, minimal emoji), content pillars (saint/art 30%, temperament 25%, etc.), platform strategy (Instagram + X tier 1). Don't bring: generic "full draft before publishing" (core has it), generic "metrics over opinion" (core has it). |
| `pm.md` (Davi) | `pm.md` | **Extend.** Bring over: Saintfy mission filter ("serve the mission of forming holy men?"), PRD location convention (`saintfy/docs/prds/`). Don't bring: generic PRD template (core PM has flow), generic "present options" (core has it). |
| `researcher.md` (Rafa) | — | **No core equivalent.** Create as project-level agent or specialist. Rafa is lightweight — hypothesis-first research + cost confirmation + source citations. Could be a specialist that Dev or Marketing Manager delegates to, or a standalone project agent. Recommend: **specialist** under `saintfy/.claude/specialists/research/researcher.md` since research is always delegated from another Manager, never receives tasks directly from Leo. |
| `writer.md` (Eli) | — | **No core equivalent.** Same as Rafa — Eli is always delegated to. Recommend: **specialist** under `saintfy/.claude/specialists/content/writer.md`. Key content to bring: brand tone rules, doctrinal content source (`docs_igreja/`), never invent product/doctrine. |
| CLAUDE.md (Leo) | `leo.md` | **CLAUDE.md becomes project-level Leo briefing.** Don't extend leo.md — Leo is universal. Instead, `saintfy/CLAUDE.md` should reference the core Leo and add Saintfy-specific context: team roster with names (Nico, Tomé, etc.), multi-stage flows, output locations, Saintfy briefing. See section 8 below. |

### What NOT to bring

- **Persona names (Tomé, Nico, Gil, etc.)** — these are Saintfy flavor. The core uses generic names (Dev Manager, Designer Manager). The project extension can name them if the founder wants, but the naming is cosmetic, not structural.
- **Agent memory files** (`.claude/agent-memory/`) — these are state memories, not learning. Most are stale or will be rebuilt. See section 5.

---

## 2. Rules

### What Saintfy-Copilot has

3 rules in `.claude/rules/`:
- `propagation.md` — Context propagation discipline
- `design-system.md` — 3-layer source of truth for design system
- `paper-artboards.md` — Paper MCP layout conventions

### What the core already covers

11 universal rules covering propagation, anti-hallucination, think-before-execute, evidence-over-claim, peer-review-automatic, state-vs-learning, hiring-loop, know-what-you-dont-know, escalation-triggers, inheritance, metrics-collection.

### What to do

| Saintfy-Copilot rule | Core equivalent | Action |
|---|---|---|
| `propagation.md` | `rules/propagation.md` | **Core supersedes.** Core version is more complete (3-trigger model, session-wrap-up skill integration). Saintfy-Copilot's version was the prototype. Don't bring — the core rule loads globally via symlink. If Saintfy needs a project-specific propagation-map, create `saintfy/.claude/context/propagation-map.md` (which the core rule already references). |
| `design-system.md` | — | **Bring as project rule.** This is 100% Saintfy-specific (3-layer: index.css → Paper → .md docs). Create `saintfy/.claude/rules/design-system.md`. Content is largely ready — just update paths if the design system directory lives in a different location in `~/Github/saintfy/`. |
| `paper-artboards.md` | — | **Evaluate.** Paper expires 2026-04-19 per memory `project_paper_to_figma.md`. If Figma migration happened, this rule is dead. If not, bring as project rule but mark as temporary. Either way, the core Designer Manager doesn't reference Paper — it's tool-agnostic. |

### Rules that are NEW in core (Saintfy-Copilot didn't have)

These will automatically apply to Saintfy via symlink, no action needed:
- `anti-hallucination.md` — was embedded in CLAUDE.md as a paragraph, now a full rule
- `think-before-execute.md` — was embedded in CLAUDE.md ("Tomé's rules"), now universal
- `evidence-over-claim.md` — was embedded in CLAUDE.md ("self-QA"), now universal with discipline-specific evidence types
- `peer-review-automatic.md` — didn't exist in Saintfy-Copilot
- `state-vs-learning.md` — didn't exist
- `hiring-loop.md` — didn't exist (Saintfy agents were monolithic)
- `know-what-you-dont-know.md` — didn't exist
- `escalation-triggers.md` — partially existed in CLAUDE.md ("confirme antes de chamadas pagas")
- `inheritance.md` — didn't exist (no extends model)
- `metrics-collection.md` — didn't exist

---

## 3. Context files

### What Saintfy-Copilot has

Rich context directory:
- `context/saintfy.md` — what Saintfy is, mission, public, status
- `context/brand.md` — positioning, tone, personas, competitive differentiation
- `context/stack.md` — comprehensive technical reference (React, Supabase, Capacitor, etc.)
- `context/competitors.md` — market analysis, direct/indirect competitors
- `context/metrics.md` — skeleton, no data yet
- `context/propagation-map.md` — routing table for what to update when things change
- `context/decisions/` — 5 domain decision files (product, tech, design, business, marketing)

### What to do

| File | Action | Notes |
|---|---|---|
| `context/saintfy.md` | **Bring and rename to `context/project.md`.** This is the core's expected file. Update to current state (it says "beta open, redesign underway" — verify if still accurate). Remove references to Saintfy-Copilot paths. |
| `context/brand.md` | **Bring as-is.** 100% project-specific. Core doesn't have a brand file — this is exactly what project context is for. Verify currency (last update 2026-03-12). |
| `context/stack.md` | **Bring and verify.** Comprehensive but may be stale (says Capacitor 8.2, React 18.3 — confirm current versions). This is the reference the Dev Manager extension will cite. |
| `context/competitors.md` | **Bring as-is.** Project-specific market intelligence. Verify Cristeros investigation status (was pending). |
| `context/metrics.md` | **Skip.** Skeleton with no data. The core's `metrics-collection.md` rule handles metrics in `.claude/metrics/YYYY-MM.jsonl` now. If Saintfy wants an app-metrics dashboard (user count, conversion), that's separate from copilot operational metrics — create fresh if needed. |
| `context/propagation-map.md` | **Bring and adapt.** Update file paths to reference `~/Github/saintfy/` instead of Saintfy-Copilot paths. Remove references to files that won't exist in the new structure. |
| `context/decisions/*.md` | **Bring all 5 decision files.** These are locked decisions — product, tech, design, business, marketing. All current. Create `saintfy/.claude/context/decisions/` and place them there. Verify no stale entries. |

### What NOT to bring

- **`context/metrics.md` skeleton** — replaced by JSONL metrics system
- Any file that duplicates what's in code (stack.md is an exception because it provides architecture overview)

---

## 4. Workflows

### What Saintfy-Copilot has

5 workflows in `workflows/`:
- `design_work.md` — 3-phase design workflow (pre-design, post-design, asset generation)
- `prd.md` — 7-step PRD creation process with template
- `instagram_post.md` — Instagram-specific post workflow
- `market_research.md` — Research process
- `draft_email.md` — Email drafting workflow

### What the core covers

The core doesn't have workflows — it has skills (`session-wrap-up`) and conventions (`github-project-management`). Workflows are project-level.

### What to do

| Workflow | Action | Notes |
|---|---|---|
| `prd.md` | **Bring as project workflow or skill.** The PRD template and 7-step process are Saintfy-refined but largely universal. The core PM Manager references "PRD → RDD → execution" flow but doesn't have the template. Place in `saintfy/.claude/workflows/prd.md` or consider proposing to core as a PM skill. |
| `design_work.md` | **Bring as project workflow.** Heavily Saintfy-specific (Paper artboards, asset generation specs with "Caravaggio chiaroscuro" aesthetic). If Paper → Figma migration happened, needs update. Place in `saintfy/.claude/workflows/design_work.md`. |
| `instagram_post.md` | **Bring if still active.** Verify against 2026-04-01 social media strategy. May need alignment. Place in `saintfy/.claude/workflows/instagram_post.md`. |
| `market_research.md` | **Bring as project workflow.** Lightweight, useful. Place in `saintfy/.claude/workflows/market_research.md`. |
| `draft_email.md` | **Bring if used.** Minimal content, may not justify a separate file. Consider embedding in Writer specialist instructions. |

---

## 5. Memories

### What Saintfy-Copilot has

**Leo's memories** (in `~/.claude/projects/-Users-vmarino-Github-Saintfy-Copilot/memory/`):
~35 memory files indexed in MEMORY.md.

**Agent memories** (in `.claude/agent-memory/`):
- Developer: `reference_app_stack.md` (comprehensive stack reference)
- Marketer: `project_social_media_strategy.md`
- Researcher: `research_seo_keywords.md`, `research_viral_artifact.md`

### What to do

**Leo's memories — classify and migrate selectively:**

| Category | Memory files | Action |
|---|---|---|
| **Learning (bring)** | `feedback_shadcn_first_enforcement`, `feedback_lint_before_accept`, `feedback_real_callsite_first`, `feedback_cap_sync_duplication_trap`, `feedback_capacitor_push_setup`, `feedback_webcrypto_vs_node_crypto`, `feedback_pr_workflow`, `feedback_strategy_before_processing`, `feedback_reusable_components`, `feedback_no_toasts`, `feedback_no_inventing_design`, `feedback_native_plugin_self_qa` | These are genuine learning memories — patterns, traps, enforcement rules that prevent repeated mistakes. Bring to `saintfy/` project memory scope. Verify each is still accurate (code may have changed). |
| **Learning (skip — now in core)** | `feedback_doc_canonical_locations`, `feedback_home_not_dashboard` | These are now either covered by core rules or are too minor. Skip. |
| **State (verify before bringing)** | `project_saintfy_status`, `project_supabase_migration`, `project_componentization_done`, `project_apple_signin_done`, `project_push_notifications_done`, `project_domain_structure` | These describe state at a point in time. Verify each against current reality. If still accurate, bring. If stale, update or skip. |
| **State (skip — likely stale)** | `project_paper_to_figma` (Paper expires 2026-04-19 — check if migration happened), `project_session_2026_04_06_07_recap` (session recap, no durable value), `project_saint_images_regen`, `project_temperament_formula_missing` | Ephemeral. Don't migrate. If the pending items are still pending, track them as GitHub issues, not memories. |
| **Reference (bring)** | `feedback_project_field_options_orphan`, `reference_context_mode_setup` | Operational knowledge about GitHub Projects v2 and MCP setup. Still useful. |
| **Reference (skip)** | `reference_paperclip_issues` (decision made — no Paperclip), `reference_skills_sh` (one-time evaluation) | Decision already captured. No future value. |
| **Platform (bring)** | `feedback_native_only` | Still relevant — Saintfy is going native-only. |
| **Design (evaluate)** | `feedback_design_system_paper`, `feedback_paper_artboard_creation`, `feedback_nav_bubble_icon` | If Paper → Figma happened, these are dead. If Paper still in use, bring. |

**Agent memories:**

| Memory | Action |
|---|---|
| Developer `reference_app_stack.md` | **Don't bring as memory.** This is `context/stack.md` content. The new Dev Manager extension should reference `context/stack.md`, not carry a separate memory. |
| Marketer `project_social_media_strategy.md` | **Bring as context/decisions/marketing.md content** (it already overlaps with `context/decisions/marketing.md`). Don't duplicate — merge into the decision file. |
| Researcher `research_seo_keywords.md`, `research_viral_artifact.md` | **Bring as context or specialist reference.** These are research outputs — either place in `context/research/` or reference from the researcher specialist file. |

---

## 6. Design system

### What Saintfy-Copilot has

12 files in `design-system/` — comprehensive component documentation from `_index.md` (routing) through `00-principles.md`, `01-tokens.md`, to `10-auth-screens.md`.

### What to do

**Keep in `~/Github/saintfy/`** — the design system docs describe the Saintfy app's visual language. They should live alongside the code, not in `.claude/`.

The Designer Manager extension should reference the design system location:
```markdown
## Design system
Source of truth lives in `~/Github/saintfy/design-system/`.
Entry point: `_index.md` (read this first to route to the right module).
3-layer hierarchy: token values in `src/index.css` → visual specs in Figma/Paper → behavior in .md docs.
```

Don't copy design system files into `.claude/`. The rule `saintfy/.claude/rules/design-system.md` governs behavior; the design system content lives in the app repo.

---

## 7. Outputs

### What Saintfy-Copilot has

Rich `outputs/` directory with ~20 date-tagged folders across app, business, website, other.

### What to do

**Don't migrate.** Outputs are historical artifacts — research reports, implementation guides, componentization reports. They live in Saintfy-Copilot as an archive.

Future outputs go to `~/Github/saintfy/outputs/` (or wherever the project decides). The core's propagation rule defines the convention: `outputs/{YYYY-MM-DD}-{task}/`.

If any output contains reusable reference material (e.g., the Apple config guide, the componentization checklist), that knowledge should be captured as a specialist playbook or context file, not as a raw output reference.

---

## 8. CLAUDE.md (the project-level Leo briefing)

### What Saintfy-Copilot has

CLAUDE.md defines Leo inline — role, team roster, delegation rules, multi-stage flows, Tomé's special rules, output conventions, anti-hallucination paragraph.

### What to do

**Rewrite for the extends model.** The new `~/Github/saintfy/CLAUDE.md` should:

1. **State that this project uses copilot-core** — Leo loads from `~/.claude/agents/leo.md` via symlink
2. **Define the Saintfy team** — names (Nico, Tomé, Gil, etc.) mapped to core Managers + project specialists
3. **Define multi-stage flows** — same table (PRD → Design → Dev), but referencing Manager roles not personas
4. **Reference project context** — `context/project.md`, `context/brand.md`, `context/decisions/`
5. **NOT duplicate core rules** — no inline anti-hallucination, no inline self-QA, no inline propagation. Those are in `~/.claude/rules/` now.

**Structure suggestion:**

```markdown
# Saintfy — Project Copilot

This project uses [copilot-core](~/Github/copilot-core/).
Leo, rules, and Managers load from ~/.claude/ (symlinked from core).

## Team

| Role | Core Manager | Project name | Extension |
|------|-------------|--------------|-----------|
| Dev lead | Dev Manager | Tomé | .claude/agents/managers/dev.md |
| Design lead | Designer Manager | Nico | .claude/agents/managers/designer.md |
| Marketing lead | Marketing Manager | Gil | .claude/agents/managers/marketing.md |
| Product lead | PM Manager | Davi | .claude/agents/managers/pm.md |

## Specialists (project-level)
| Name | Domain | File |
|------|--------|------|
| Rafa | Research | .claude/specialists/research/researcher.md |
| Eli | Content/Writing | .claude/specialists/content/writer.md |
| (others hired via Hiring Loop as needed) |

## Flows
[same multi-stage flows table]

## Context
- Project overview: context/project.md
- Brand: context/brand.md
- Stack: context/stack.md
- Decisions: context/decisions/*.md

## Project rules
- .claude/rules/design-system.md
```

---

## 9. Settings and hooks

### What Saintfy-Copilot has

`.claude/settings.local.json` with:
- Tool permissions (Bash, Read, Edit, Write, Paper MCP tools)
- Hooks: PreToolUse for Paper operations (governance check), PostToolUse for design finalization
- Environment variables (cleanup, output limits, autocompact)

`.mcp.json` with Paper.design MCP connection.

### What to do

| Item | Action |
|---|---|
| Tool permissions | **Create fresh for saintfy.** Same structure, adapted tools. If Paper is dead, remove Paper MCP tools. |
| Hooks | **Evaluate.** Paper governance hooks are valuable pattern. If Figma replaced Paper, adapt hooks to Figma workflow. If neither, skip. |
| MCP config | **Create fresh.** Whatever MCPs the project uses (context-mode, Figma, etc.). |

---

## 10. GitHub conventions

### What Saintfy-Copilot has

No formal convention docs — conventions were learned organically via memories and CLAUDE.md rules.

### What the core now has

`docs/conventions/github-project-management.md` + issue/PR templates in `docs/conventions/templates/`.

### What to do

1. **Copy templates** from `copilot-core/docs/conventions/templates/` to `saintfy/.github/ISSUE_TEMPLATE/` and `saintfy/.github/pull_request_template.md`
2. **Set up GitHub Project v2** following the convention (Status, Agent, Type fields)
3. **Clean default labels** per convention
4. **Open current milestone** (whatever version Saintfy is shipping)

---

## 11. Doctrinal content (docs_igreja/)

### What Saintfy-Copilot has

`docs_igreja/` with curated Bible translations and Catholic prayers — source of truth for theological content.

### What to do

**This content should live in `~/Github/saintfy/`** (the app repo), not in `.claude/`. It's app content, not copilot config. If it's already there, great. If it only exists in Saintfy-Copilot, move it to the app repo.

The Writer specialist should reference it: "For theological content, consult `~/Github/saintfy/docs_igreja/`."

---

## 12. Migration checklist

The Leo doing the onboarding should go through this list:

### Phase 1 — Scaffold

- [ ] Create `~/Github/saintfy/.claude/` directory structure:
  - [ ] `agents/managers/` (dev.md, designer.md, pm.md, marketing.md — all with `extends`)
  - [ ] `specialists/` (research/, content/)
  - [ ] `rules/` (design-system.md, others as needed)
  - [ ] `context/` (project.md, brand.md, stack.md, competitors.md, propagation-map.md)
  - [ ] `context/decisions/` (product.md, tech.md, design.md, business.md, marketing.md)
  - [ ] `metrics/` (empty, will be populated by metrics-collection rule)
  - [ ] `workflows/` (prd.md, design_work.md, instagram_post.md, market_research.md)
- [ ] Create `~/Github/saintfy/CLAUDE.md` (project-level Leo briefing)
- [ ] Copy GitHub templates from core to `~/Github/saintfy/.github/`

### Phase 2 — Content migration (from Saintfy-Copilot)

- [ ] Read each context file in Saintfy-Copilot, verify currency, adapt to new path structure
- [ ] Read each decision file, verify no stale entries, bring to new context/decisions/
- [ ] Read each learning memory, verify still accurate, bring to project memory scope
- [ ] Skip state memories unless verified current
- [ ] Read agent files, extract project-specific content into extends files
- [ ] Read workflows, adapt paths, place in new location

### Phase 3 — Validation

- [ ] Run a simple task through Leo → Manager → execution → self-QA cycle
- [ ] Verify extends works (core + project combine correctly)
- [ ] Verify design-system rule loads and references correct paths
- [ ] Verify metrics entry is created
- [ ] Verify propagation works (session wrap-up updates context files)

### Phase 4 — Cleanup

- [ ] Archive Saintfy-Copilot (make read-only or mark as superseded in README)
- [ ] Update any external references to Saintfy-Copilot paths
- [ ] Confirm `~/Github/saintfy/` is self-sufficient (doesn't depend on Saintfy-Copilot for anything)

---

## 13. Key warnings

1. **Don't copy Saintfy-Copilot's propagation.md** — the core version supersedes it. Using the old one would create a conflict with the core rule loaded via symlink.

2. **Don't inline core rules into project files** — the `inheritance.md` rule explicitly forbids this. If you see Saintfy-Copilot's CLAUDE.md has "anti-hallucination" paragraphs, those are now in `~/.claude/rules/anti-hallucination.md`. Don't duplicate.

3. **Shadcn-first enforcement is the most critical project rule** — it cost 48h of refactoring work when violated. It MUST be in the Dev Manager extension with the same severity. This is not a guideline, it's a non-negotiable enforcement rule with `lint-shadcn.sh` as gate.

4. **Paper may be dead** — check if the Paper → Figma migration happened (was scheduled for 2026-04-18). If yes, all Paper-specific rules and workflows need adaptation or removal.

5. **Agent persona names are optional but valued** — the founder uses "Tomé", "Nico", etc. in conversation. The core doesn't require names, but the project can name its agents. Keep the names if the founder wants them.

6. **Researcher and Writer are NOT Managers in the core** — they don't have core equivalents. In the Saintfy-Copilot they were peer agents to Tomé/Nico. In the new model, they should be **specialists** that Managers delegate to, not standalone agents that Leo delegates to directly. This is a philosophical change: Leo → Manager → Specialist, not Leo → Specialist.

7. **`docs_igreja/` is app content, not copilot content** — it should live in the app repo, referenced by the Writer specialist. Don't put it in `.claude/`.

---

*Written 2026-04-10 by Leo-Saintfy. Based on complete inventory of both repos.*
