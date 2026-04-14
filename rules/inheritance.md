---
name: inheritance
description: Specialists inherit context from Leo's briefing + project context + core rules. Extend, never override.
---

## Rule

When Leo spins up a specialist sub-agent, the specialist inherits context from multiple layers. The briefing Leo constructs is the mechanism that delivers this inheritance. The project's context **adds** to the core, never replaces.

Concatenation order: **core rules first, project context second, task-specific briefing last**.

## Why

This is the mechanism that lets the core update rules and specialist playbooks without breaking projects. Without it, a project would have to duplicate the entire core and would lose the benefit of global updates. With it, the project adds **only what is specific** (stack, tokens, local decisions) and inherits the rest.

It's also how the "extend, never override" philosophy is executed in practice. If a project could remove core behavior, the core's quality guarantees would disappear.

## Inheritance layers

When Leo prepares a specialist's briefing, it assembles context from these layers:

### Layer 1 — Core rules (universal)

All rules in `leo-core/rules/` apply to every specialist. These are non-negotiable and cannot be overridden by project-level configuration.

### Layer 2 — Specialist playbook (domain-specific)

The specialist's playbook file (e.g., `.claude/specialists/dev/frontend-react-specialist.md`) provides domain expertise, self-check criteria, and technical content.

### Layer 3 — Project context (project-specific)

Project-level context files (`context/stack.md`, `context/decisions/*.md`, `context/project.md`, etc.) provide the specialist with project-specific knowledge — what stack is used, what decisions have been made, what conventions apply.

### Layer 4 — Task briefing (task-specific)

Leo's routing briefing for the specific task: what to do, why, constraints, and any relevant context from the current session.

## How Leo applies inheritance

### When Leo spins up a specialist

1. Leo identifies the task domain and selects the appropriate specialist playbook
2. Leo loads core rules that apply to the task
3. Leo loads the project context relevant to the task
4. Leo constructs the briefing: core rules + specialist playbook + project context + task details
5. Leo passes the assembled briefing to the specialist sub-agent

Leo **does not inline** everything — they select the relevant subset to keep the specialist's context focused and token-efficient.

### When a project specializes a playbook

Project-level specialist playbooks can extend core playbooks by adding project-specific knowledge. For example, a project's `frontend-react-specialist.md` can add "in addition, PRs touching `.tsx` run `scripts/lint-shadcn.sh` in self-QA". Core stays valid, the project is stricter.

## What is forbidden

- ❌ **Removing core behavior.** If a project wants to disable a universal rule, it's a sign that either the rule is wrong (propose a core change via R2) or the project is wrong. Never silently disable.
- ❌ **Contradicting principles.** If the core says "PR-first mandatory", the project cannot have a rule "committing directly to main is ok". If there's a genuine need for contradiction, the core is wrong — propose a change via R2.
- ❌ **Duplicating core content into project files.** That kills the benefit of global updates. Reference, don't copy-paste.

## What is allowed

- ✅ **Adding self-QA items, escalation triggers, domain rules** specific to the project
- ✅ **Specializing** generic playbooks with concrete stack details
- ✅ **Adding context** beyond the core's (project decisions, conventions, patterns)
- ✅ **Referencing project-specific skills** in the briefing

## Responsibility

Leo is the one who assembles the inherited context when spinning up specialists — Leo is the only agent who **reads** playbooks, context files, and core rules to prepare a briefing. Specialists receive the briefing already prepared.

When the owner asks "why did that specialist do X?", Leo can explain which part came from the core rules, which from the project context, and which from the task briefing — transparency of the inheritance chain.
