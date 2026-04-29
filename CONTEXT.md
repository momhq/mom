# MOM

MOM (Memory Oriented Machine) is a local, harness-agnostic memory layer for AI coding assistants. Memory lives in `.mom/`; MOM integrates with multiple AI Harnesses via Adapters.

## Language

**Harness**:
The agent framework that wraps a Model and provides tools, hooks, MCP, and prompt scaffolding. The integration target for MOM Adapters.
_Examples_: Pi, Claude Code, Codex, Windsurf, Cursor, Cline, Aider.
_Avoid_: runtime (overloaded with CS-generic uses — language runtime, container runtime; legacy term in MOM's own codebase, slated for migration), agent (the LLM itself, not its framework), IDE.

**Adapter**:
A MOM module that translates one Harness's native events, file conventions, and integration mechanisms into MOM's pipeline. Each supported Harness has exactly one Adapter.
_Avoid_: connector, plugin.

**Tier**:
A classification of Harness integration quality from MOM's perspective. Tier is **editorial** — it reflects MOM's judgment of how natively, faithfully, and automatically the Harness integrates with MOM's pipeline. Three values, named after language-fluency levels because the metaphor maps closely to MOM's depth of integration with each Harness:

- **Native** — MOM speaks the Harness's language fluently and can riff in it. The Harness offers programmable extensibility (loadable extensions, event interception, project-scoped configuration). MOM integrates natively, captures everything automatically, and can expose its full feature set as native Harness tools.
  _Example_: Pi.

- **Fluent** — MOM speaks the Harness's language correctly through standard idioms but cannot improvise outside them. The Harness offers a fixed integration surface (hook system, settings file, transcript directory). MOM integrates well but may lack data fidelity (e.g., per-turn usage metrics not exposed) or occasionally lose data due to Harness limitations.
  _Examples_: Claude Code, Codex.

- **Functional** — MOM knows enough phrases to communicate basic needs but cannot hold a conversation outside the script. The Harness offers minimal integration (e.g., MCP without project-local config, no extensions, opinionated workflow). MOM works but users often must trigger memory capture manually, automation is unreliable, and cross-project data bleed is possible.
  _Example_: Windsurf.

## Relationships

- A **Harness** has exactly one **Adapter** in the MOM codebase.
- An **Adapter** declares its **Harness**'s **Tier**.
- **Tier** drives MOM's expectations of what the Adapter can do: Native adapters may install extensions; Fluent adapters use hooks; Functional adapters provide best-effort capture only.

## Example dialogue

> **Dev:** "Should we add an MRP event for compaction?"
> **Architect:** "Pi can support it natively — it's Native, the extension subscribes to compaction events. Claude is Fluent but happens to expose `PreCompact`, so we can wire it via hook. Windsurf is Functional; we won't capture compaction reliably and shouldn't promise it."

## Dev conventions

**Local build for testing**: before releasing from a `-dev` branch, build the binary locally and point it to `/tmp/mom` for testing without affecting the Homebrew installation:

```bash
cd cli && go build -ldflags "-X github.com/momhq/mom/cli/internal/cmd.Version=vX.X.X-dev" -o /tmp/mom ./cmd/mom/
```

Then run as `/tmp/mom <command>` from any directory.

## Coding conventions

**Code comments**: Concise, objective, descriptive. Clarify what code does, not why a design was chosen. No ADR cross-references in `.go` files, no storytelling, no rationale prose. Rationale lives in commit messages, PR descriptions, and ADRs — not in code.

_Origin_: PR #188 inline review by vmarinogg.

## Flagged ambiguities

- "runtime" is the legacy term used throughout MOM's existing codebase and architecture docs. Resolved: **Harness** is canonical going forward; "runtime" is the alias to avoid; codebase migration tracked separately.
- "tier" could be confused with **AdapterCapability** (the YAML files declaring which MRP events a Harness natively supports). Resolved: **Tier** is editorial integration-quality judgment; **AdapterCapability** is a finer-grained per-event declaration. They are independent — a Fluent Harness can have full event coverage; a Native Harness can have partial coverage. Pi today is `(Native tier, partial event coverage)`; Claude is `(Fluent tier, full event coverage)`.
