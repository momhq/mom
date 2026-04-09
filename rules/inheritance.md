---
name: inheritance
description: Agents with `extends` in the frontmatter load the base file before running and concatenate behavior.
---

## Rule

When an agent file (usually in the project) has an `extends` field in the frontmatter pointing to another file (typically in the core), you **load the base file first** and treat it as the foundation. The project's content **adds** to the base, never replaces.

Concatenation order: **core first, project second**.

## Why

This is the mechanism that lets the core update managers and rules without breaking projects. Without it, a project would have to copy the entire core file and would lose the benefit of global updates. With it, the project adds **only what is specific** (stack, tokens, local decisions) and inherits the rest.

It's also how the "extend, never override" philosophy is executed in practice. If the project could remove core behavior, the core's quality guarantees would disappear.

## Format

An inheriting agent file has frontmatter like this:

```yaml
---
name: Dev Manager (Saintfy)
description: Saintfy's dev tech lead, extending the core Dev Manager
extends: ../../../../.claude/agents/managers/dev.md
tools: Read, Edit, Write, Glob, Grep, Bash, Task
model: sonnet
skills: [saintfy-briefing]
---
```

The `extends` field can use:
- A path relative to the current file (preferred for readability)
- An absolute path starting with `/` (for references to the globally installed core)

In practice, projects extend files that live in `~/.claude/agents/managers/*.md` (symlinked by the core's `sync.sh`).

## How to apply

### When Leo delegates to a project Manager

1. Leo reads the project's agent file (e.g., `~/Github/saintfy/.claude/agents/managers/dev.md`)
2. Sees `extends: ../../../../.claude/agents/managers/dev.md` in the frontmatter
3. Reads the base file (the core Dev Manager at `~/.claude/agents/managers/dev.md`)
4. Concatenates mentally: **core first, project second**
5. Passes the combined context as the briefing for the executing session

Leo **does not inline** the core's content into the project file — they only make sure both are loaded at execution time.

### When a Manager inherits a domain rule

Domain rules live inside the Managers' agent files. When a project's Manager extends the core Manager, the core's domain rules come in automatically, and the project can **add** its own rules or **specialize** the existing ones — but not remove them.

Example: the core Dev Manager has a generic "PR workflow" rule. The Saintfy Dev Manager extends it and adds "in addition, PRs touching `.tsx` run `scripts/lint-shadcn.sh` in self-QA". Core stays valid, the project is stricter.

## What is forbidden

- ❌ **Removing core behavior.** If a project wants to disable a universal rule, it's a sign that either the rule is wrong (propose a core change via R2) or the project is wrong. Never silently disable.
- ❌ **Renaming frontmatter fields.** If the core defines `tools: Read, Edit, ...`, the project cannot use a different name — it can only add tools or use the same ones.
- ❌ **Contradicting principles.** If the core says "PR-first mandatory", the project cannot have a rule "committing directly to main is ok". If there's a genuine need for contradiction, the core is wrong — propose a change via R2.
- ❌ **Inlining core content into the project file.** That kills the benefit of global updates. Use `extends`, not copy-paste.

## What is allowed

- ✅ **Adding principles, self-QA items, escalation triggers** specific to the project
- ✅ **Specializing** generic rules with concrete stack details
- ✅ **Overriding** frontmatter fields when there's a reason (e.g., `model: opus` when evidence shows sonnet is getting it wrong)
- ✅ **Adding tools** to the list beyond the core's (but never removing)
- ✅ **Referencing project-specific skills** in the `skills` field

## Responsibility

Leo is the one who does the mental concatenation when delegating — they are the only agent who **reads** other agents' files to prepare a briefing. Other agents receive the briefing already prepared.

When the founder asks "why did that Manager do X?", Leo can explain which part came from the core and which from the project — transparency of the hierarchy.
