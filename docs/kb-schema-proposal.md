# KB Schema — Proposal v1

> Based on reading all 11 existing rules, 1 skill, 2 context files,
> and the memory system. This schema must cover everything that exists
> today + the new concepts we defined.

## Base structure (all docs share this)

Every doc in the KB has these fields:

```json
{
  "id": "string (unique, kebab-case)",
  "type": "string (enum — see types below)",
  "lifecycle": "string: permanent | learning | state",
  "scope": "string: core | project",
  "tags": ["array of strings"],
  "created": "ISO 8601 datetime",
  "created_by": "string — which agent or actor created this doc",
  "updated": "ISO 8601 datetime",
  "updated_by": "string — which agent or actor last updated this doc",
  "content": "object (varies by type — see below)"
}
```

| Field | Required | Notes |
|---|---|---|
| `id` | Yes | Unique across the KB. Kebab-case. Used as filename: `{id}.json` |
| `type` | Yes | Determines which `content` sub-schema applies |
| `lifecycle` | Yes | `permanent` = almost never changes, `learning` = slow decay, `state` = fast decay |
| `scope` | Yes | `core` = universal (came from LEO core), `project` = specific to this project |
| `tags` | Yes (min 1) | Free-form strings, lowercase, kebab-case. These are the synapses. |
| `created` | Yes | When the doc was first created |
| `created_by` | Yes | Who created: `leo`, `engineer-manager`, `designer-manager`, `pm-manager`, `marketing-manager`, `owner`, or specialist name |
| `updated` | Yes | Last modification timestamp |
| `updated_by` | Yes | Who last modified (same values as `created_by`) |
| `content` | Yes | Shape depends on `type` |

---

## Types and their `content` schemas

### 1. `rule`
Operational rules that govern agent behavior.

**Migrates from:** `rules/*.md` (11 files today)

```json
{
  "type": "rule",
  "lifecycle": "permanent",
  "content": {
    "rule": "string — the core rule statement (1-3 sentences)",
    "why": "string — why this rule exists, what problem it prevents",
    "how_to_apply": ["array of strings — each is a concrete instruction or sub-rule"],
    "examples": [
      {
        "situation": "string",
        "wrong": "string",
        "right": "string"
      }
    ],
    "anti_patterns": ["array of strings — common violations to watch for"],
    "responsibility": "string — who enforces this rule",
    "mechanisms": [
      {
        "name": "string",
        "description": "string",
        "when": "string: preventive | reactive | structural | reflective"
      }
    ]
  }
}
```

**Notes:**
- `examples` is optional — some rules have them, others don't
- `mechanisms` is optional — only `know-what-you-dont-know` uses it (4 mechanisms), but other complex rules could too
- `anti_patterns` captures the ❌ sections that most rules have
- `how_to_apply` is an array because most rules have multiple sub-instructions (e.g., escalation-triggers has 5 trigger categories)

**Example — `think-before-execute` converted:**
```json
{
  "id": "think-before-execute",
  "type": "rule",
  "lifecycle": "permanent",
  "scope": "core",
  "tags": ["execution", "alignment", "decision-making", "autonomy"],
  "created": "2026-04-08T00:00:00Z",
  "created_by": "owner",
  "updated": "2026-04-10T00:00:00Z",
  "updated_by": "leo",
  "content": {
    "rule": "Before executing, decide: Direct mode (clear, bounded → execute) or Alignment mode (ambiguous, architectural → present approach and wait for approval).",
    "why": "Claude has a tendency toward direct mode by default. It works for 70% of tasks but fails catastrophically on the 30% that need alignment — the model decides alone on points where the owner should be the decider.",
    "how_to_apply": [
      "Direct mode when: clear bounded instruction, obvious bug fix with known root cause, point adjustment to existing feature, you can describe the final diff in one sentence",
      "Alignment mode when: architectural decision, more than one reasonable way to implement with real trade-offs, affects multiple files non-trivially, you need to infer what the owner meant, task is vague with no metrics",
      "In alignment mode: write task summary, decisions in play, options with pros/cons, recommendation, specific question for the owner. Wait for answer. Don't start while they're deciding."
    ],
    "examples": [
      {
        "situation": "Change the primary button color to gold on the login screen",
        "wrong": "Ask the owner for permission (too much friction)",
        "right": "Direct mode — go ahead"
      },
      {
        "situation": "Change the app's primary color",
        "wrong": "Just change the CSS variable and ship it",
        "right": "Alignment mode — explain it affects 40 components, present options (token only vs full refactor vs staged), ask which approach"
      }
    ],
    "anti_patterns": [
      "Asking permission for everything — becomes friction",
      "Executing an architectural decision without aligning — generates rework"
    ],
    "responsibility": "All agents, self-enforced. The self-check is mandatory, not optional."
  }
}
```

---

### 2. `skill`
Executable workflows that agents can invoke.

**Migrates from:** `skills/*/SKILL.md` (1 file today)

```json
{
  "type": "skill",
  "lifecycle": "permanent",
  "content": {
    "description": "string — what this skill does",
    "triggers": ["array of strings — natural language patterns that invoke this skill"],
    "invoked_by": "string — who can invoke (e.g., 'leo', 'any-manager', 'owner')",
    "steps": [
      {
        "name": "string",
        "instruction": "string — what to do in this step",
        "wait_for_approval": "boolean — does this step require R2 before next?"
      }
    ],
    "do_not": ["array of strings — explicit anti-patterns"],
    "output_format": "string — what the final output looks like"
  }
}
```

**Notes:**
- `triggers` enables script-level routing before spending tokens
- `steps` with `wait_for_approval` makes the R2 gates explicit and queryable
- `do_not` captures the "What this skill does NOT do" and "Anti-patterns" sections

---

### 3. `identity`
Core identity of the project — what it IS.

**Migrates from:** `.claude/context/project.md`, `.claude/context/stack.md`, `brand.md`

```json
{
  "type": "identity",
  "lifecycle": "permanent",
  "content": {
    "what": "string — what this project/product is (1-3 sentences)",
    "stack": ["array of strings — technologies, frameworks, tools"],
    "philosophy": "string — core principles that guide the project",
    "constraints": ["array of strings — hard limits, non-negotiables"]
  }
}
```

---

### 4. `decision`
A decision that was made with context and rationale.

**Migrates from:** `context/decisions/*.md`, session wrap-ups

```json
{
  "type": "decision",
  "lifecycle": "learning",
  "content": {
    "decision": "string — what was decided",
    "context": "string — what led to this decision",
    "alternatives_considered": ["array of strings — what else was on the table"],
    "impact": ["array of strings — what this decision affects"],
    "reversible": "boolean"
  }
}
```

---

### 5. `pattern`
A reusable convention, template, or way of doing things.

**Migrates from:** conventions, templates, learned patterns

```json
{
  "type": "pattern",
  "lifecycle": "learning",
  "content": {
    "pattern": "string — what the pattern is",
    "when_to_use": "string — in what situations this applies",
    "how": ["array of strings — steps or instructions"],
    "template": "string (optional) — a template or example to follow"
  }
}
```

---

### 6. `fact`
A temporary piece of information that will age.

**Migrates from:** state memories, current status docs

```json
{
  "type": "fact",
  "lifecycle": "state",
  "content": {
    "fact": "string — the factual statement",
    "source": "string (optional) — where this fact came from",
    "expires": "ISO 8601 datetime (optional) — when this fact should be re-validated"
  }
}
```

---

### 7. `feedback`
Owner correction or guidance on agent behavior.

**Migrates from:** feedback memories

```json
{
  "type": "feedback",
  "lifecycle": "permanent",
  "content": {
    "feedback": "string — the correction or guidance",
    "why": "string — why the owner gave this feedback",
    "how_to_apply": "string — when and where this guidance kicks in"
  }
}
```

---

### 8. `reference`
Pointer to an external resource.

**Migrates from:** reference memories, external links

```json
{
  "type": "reference",
  "lifecycle": "state",
  "content": {
    "description": "string — what this resource is",
    "url": "string (optional) — link to the resource",
    "purpose": "string — why this reference matters"
  }
}
```

---

### 9. `metric`
A task execution metric entry.

**Migrates from:** `.claude/metrics/*.jsonl`

```json
{
  "type": "metric",
  "lifecycle": "state",
  "content": {
    "task_id": "string",
    "owner_prompt_summary": "string",
    "manager": "string",
    "specialist_used": "string (optional)",
    "review": {
      "first_pass": "boolean",
      "iterations": "number"
    },
    "owner": {
      "accepted_on_delivery": "boolean"
    },
    "rework_cycles": "number",
    "hiring_loop": {
      "outcome": "string: triggered | missed | na"
    },
    "delegation": {
      "quality": "string: correct_first | self_attempted_then_delegated | wrong_agent_then_corrected"
    },
    "internal_iterations": "number",
    "leo_errors": ["array of strings"],
    "self_qa": {
      "honest": "boolean (optional)"
    },
    "duration_minutes_approximate": "number (optional)",
    "notes": "string (optional)"
  }
}
```

**Note:** Metrics are already JSON (JSONL). The migration is mostly structural — they join the KB as docs instead of living in a separate folder. The index can aggregate them (`by_type.metric`), and agents can query metrics history via tags (e.g., tag `engineer` shows all tasks delegated to engineering).

---

## Fields NOT in the schema (by design)

| Excluded | Why |
|---|---|
| `version` (per doc) | Git handles versioning. Adding doc-level versions creates sync problems. |
| `status` (active/archived) | Use `lifecycle` + `updated` timestamp. A `state` doc not updated in 30 days is implicitly stale — `check-stale.sh` catches this. |
| `priority` | Knowledge doesn't have priority. All rules are equally mandatory. Decisions don't compete. |
| `parent` / `children` | No hierarchy. Tags create connections. No parent-child relationships. |
| `visibility` | Future feature (RBAC/ABAC). Not in v1 — flagged in product radar. |

---

## Index structure (generated by `build-index.sh`)

```json
{
  "version": "1",
  "last_rebuilt": "ISO 8601 datetime",
  "stats": {
    "total_docs": "number",
    "total_tags": "number",
    "docs_by_type": {
      "rule": "number",
      "skill": "number",
      "identity": "number",
      "decision": "number",
      "pattern": "number",
      "fact": "number",
      "feedback": "number",
      "reference": "number",
      "metric": "number"
    },
    "stale_count": "number",
    "most_connected_tag": "string (tag with most docs)"
  },
  "by_tag": {
    "tag-name": ["doc-id-1", "doc-id-2"]
  },
  "by_type": {
    "rule": ["doc-id-1", "doc-id-2"],
    "decision": ["doc-id-3"]
  },
  "by_scope": {
    "core": ["doc-id-1"],
    "project": ["doc-id-2"]
  },
  "by_lifecycle": {
    "permanent": ["doc-id-1"],
    "learning": ["doc-id-2"],
    "state": ["doc-id-3"]
  }
}
```

---

## Validation rules (enforced by `validate.sh`)

1. Every doc has all required base fields (`id`, `type`, `lifecycle`, `scope`, `tags`, `created`, `updated`, `content`)
2. `type` is one of the defined enum values
3. `lifecycle` is one of: `permanent`, `learning`, `state`
4. `scope` is one of: `core`, `project`
5. `tags` has at least 1 element
6. `tags` elements are lowercase kebab-case
7. `id` matches filename (minus `.json`)
8. `content` has the required fields for its `type`
9. No duplicate `id` across all docs
10. Dates are valid ISO 8601
