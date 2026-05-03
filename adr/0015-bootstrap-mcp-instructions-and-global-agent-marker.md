# 0015 — Bootstrap: global agent-file marker (and protocol-compliant MCP instructions)

Pre-v0.30, MOM made the harness aware of its presence by generating a per-project context file (e.g. `<project>/.claude/CLAUDE.md`) during `mom init`. v0.30 removes the per-folder `.mom/` (ADR 0009) and stops generating per-project files. Without a replacement, a fresh session in any project has no signal that MOM exists, no instruction to call `mom_status` first, and no way to surface MOM's operating protocol to the agent.

The natural candidate — the MCP spec's server-level `instructions` field — does not work as a bootstrap mechanism in May 2026 across the four supported harnesses (Claude Code, Codex, Windsurf, Pi). Claude Code uses it only as a tool-search discovery hint (2 KB cap, not injected into agent context). Codex closed implementation as "not planned" in December 2025. Windsurf's Cascade MCP integration only consumes `tools` / `resources` / `prompts`. Pi has no first-party MCP support and the third-party adapter does not surface `instructions`. None of the four inject it into model context.

v0.30 ships **path B as the canonical mechanism**: `mom init` writes a sentinel-delimited Markdown block into each detected harness's global agent file. The block contains a short instruction ("call `mom_status` at session start") and a one-line note that MOM is installed. The block is delimited by `<!-- mom:start -->` / `<!-- mom:end -->` so re-running `mom init` updates it in place rather than duplicating. The block is **prepended** to the file (after any leading YAML frontmatter or H1 header) so the bootstrap instruction appears early in the agent's context.

All four supported harnesses **stack** global and project context files (concatenate, do not override). A heavily-customized project file does not shadow the global block; both are loaded. "Project wins on conflict" applies only to contradictory instructions through recency-weighting — and the bootstrap line ("call `mom_status` first") is unlikely to ever conflict with anything in a project file. The global block reliably reaches the agent on every session in every project.

**Path A — MCP server `instructions` — is emitted for protocol compliance only.** The MOM MCP server returns a short `instructions` string in its `initialize` response, matching the MCP specification. As of v0.30 ship no supported harness injects this into agent context, so it does not function as a bootstrap mechanism today. If any harness adds support in the future, the bootstrap fires automatically with no MOM-side code change. Cost is negligible (a constant string in the initialize handler) and the behavior is correct per spec.

## Per-harness paths

| Harness | Global agent file path | Notes |
|---|---|---|
| Claude Code | `~/.claude/CLAUDE.md` | Stacks with project `CLAUDE.md`. |
| Codex | `~/.codex/AGENTS.md` | Concatenated before project files. Also check for `~/.codex/AGENTS.override.md`; if present, warn (it shadows the plain file). |
| Windsurf | `~/.codeium/windsurf/memories/global_rules.md` | Hard 6,000-character limit on this file; the MOM block is short (~280 bytes incl. sentinels) so it coexists fine with user content. |
| Pi | `~/.pi/agent/AGENTS.md` | Pi rejects MCP; MOM's tool surface in Pi continues to use the existing extension model (`.pi/extensions/mom-tools.ts`). The marker block is still useful for the bootstrap instruction. |

`mom init` detects which harnesses are installed (by probing for the relevant config directories) and writes the marker block to each one. Harnesses that are not installed are skipped.

## Known harness fragilities

- **Codex**: known bug ([openai/codex#8759](https://github.com/openai/codex/issues/8759)) where the global `AGENTS.md` is not always read in some configurations. `mom doctor` should detect and surface this.
- **Codex**: if `~/.codex/AGENTS.override.md` exists, it takes precedence over `~/.codex/AGENTS.md` and our block is shadowed. `mom doctor` should detect and warn.
- **Windsurf**: intermittent silent-load failures of `global_rules.md` reported in past versions. `mom doctor` should verify the file is actually being read.

When the bootstrap fails for any of these reasons, the user has no in-session recovery path in v0.30 (manually typing "call `mom_status`" works but is not discoverable). A user-facing `mom` skill exposing `/mom boot` (and the broader command surface) is deferred to a post-v0.30 release; the recovery story consolidates with the skill design.

## Consequences

- A fresh session in any project automatically receives the "call `mom_status` first" signal via the global file marker block, without any project-local MOM artifact.
- `mom init` becomes a one-time-per-machine global operation (updates the marker block in each detected harness's global agent file). A per-project `mom init` mode still exists for registering the MOM MCP server in the project's MCP config (e.g. `.mcp.json`), but it does not generate any project-local context files.
- The user's global agent file is mutated by `mom init`. The marker sentinels make the change auditable and removable; the inserted block is short and clearly attributed.
- The MCP server emits `instructions` for protocol compliance. This is not a working bootstrap path today but costs nothing and self-activates if any harness adds support.
- Bootstrap failures (Codex `AGENTS.md` not read; `AGENTS.override.md` shadowing; Windsurf load bug) are surfaced by `mom doctor` but have no in-session recovery in v0.30. The post-v0.30 `mom` skill (`/mom boot`) is the planned recovery mechanism.
- Pi continues to use its extension model for the actual MOM tool surface; only the bootstrap instruction lands in `~/.pi/agent/AGENTS.md`.

## Considered alternatives

- **Per-project file generation (the pre-0.30 mechanism, kept).** Rejected: contradicts the v0.30 direction of dropping per-folder MOM artifacts (ADR 0009), and requires running `mom init` per project — fails the "I want MOM in a project I haven't initialized" case.
- **MCP server instructions as the primary bootstrap.** Rejected after research: zero of four supported harnesses currently inject server `instructions` into agent context. Promising long-term, non-functional today.
- **Append the block at the end of the global file.** Rejected in favor of prepending. The bootstrap instruction's whole point is "before you do anything else, call `mom_status`" — putting it at the top earns the visual prominence that matches its semantic role. Sentinels keep it removable; we insert after any leading YAML frontmatter or H1 header to avoid breaking existing structure.
- **Ship a minimal `/mom boot` recovery skill in v0.30.** Considered seriously as a safety net for the known harness fragilities. Deferred along with the rest of the `mom` skill surface to a post-v0.30 release, to keep v0.30 scope focused. Manual recovery (user types "call `mom_status`") remains the v0.30 fallback for the bootstrap-failure case.
- **Bootstrap via the user's shell rc files** (`.bashrc` / `.zshrc`). Rejected: shell rc is the wrong layer; agent context is what needs the instruction; shell environment cannot reach into a session.
