# Changelog

All notable changes to _mom_ are documented here. The format is loosely based on
[Keep a Changelog](https://keepachangelog.com/), and the project follows a
`vN0.0-alpha` cadence during the alpha line.

## v0.50.0-alpha

**Memory is now per-project markdown.** v0.50 removes the SQLite global vault and
the MCP server, and makes a per-project markdown vault — projected from an
append-only Ledger — the sole memory interface. Your agent reads memory with its
normal file tools; there is no query server. See
[ADR 0024](adr/0024-v050-markdown-vault-memory.md) for the full as-built design.

```
Harnesses / CLI → Editor → Ledger ($HOME/.mom/ledger/, append-only)
                              │  mom vault fold (/mom-fold, or the daemon timer)
                              ▼
                  services/projection (Reader → Synthesizer → Writer)
                              ▼
                  .mom/vault/*.md   ← agents read these directly
```

### Breaking changes

- **SQLite global vault removed.** `$HOME/.mom/mom.db` is gone. The `MOM_VAULT`
  override now points at a directory (the central `$HOME/.mom` location), not a
  `.db` file.
- **MCP server removed.** No `mom serve mcp`, no harness MCP registration, and no
  `mom_recall` / `mom_record` / `mom_get` / `mom_landmarks` MCP tools. Memory is
  delivered through the harness context file and skills instead (ADR 0023).
- **`mom recall` search read path removed.** The FTS5 finder is deleted; recall is
  now navigation of the markdown vault (the `/mom-recall` skill reads files).
- **`mom export` / `mom import` removed** — they dumped SQLite tables.
- **`mom-wrap-up` skill removed.** The draft-curation flow it drove no longer
  exists; use `/mom-fold` (`mom vault fold`).

### Added

- **Markdown vault** under `.mom/vault/`: a router `INDEX.md` plus `topics/`,
  `timeline/`, `summaries/`, and `episodes/` files synthesized from the Ledger.
- **`mom vault fold` / `mom vault rebuild` / `mom vault status` / `mom vault garden`**
  and the **`/mom-fold`** and **`/mom-rebuild`** skills.
- **`services/projection`** — the fold (Reader → Synthesizer → Writer), with a
  deterministic fallback and pluggable LLM engines (claude / codex / pi).
- **`mom --version`** flag alongside the `version` subcommand.
- Bare **`mom project`** reports the current directory's binding (the session-start
  check the harness context block runs).
- `mom init` binds the current directory (when it is a real project root) so
  capture starts on first run.

### Changed

- **`mom lens`** now reads the append-only Ledger (loopback only) and shows
  captured sessions and privacy-projected tool activity.
- **`mom status`** reads the Ledger directly.
- Capture is **privacy-gated**: a turn is recorded only when the directory is
  bound to a project (`.mom-project.yaml`).
- Retired the in-process event bus (`bus/herald`), the worker pipeline
  (drafter, logbook, cartographer, gardener), `events/crier`, and the unconsumed
  telemetry emitter. The canonical event type now lives in `events/envelope`.

### Upgrade

```bash
mom upgrade --dry-run   # preview
mom upgrade
```

`mom upgrade` regenerates the harness context blocks, tears down the retired MCP
registration (Claude `mcpServers.mom` in `~/.claude.json` / `~/.mcp.json` and the
Codex `[mcp_servers.mom]` block), removes obsolete hook commands, and prunes
deprecated skills (`mom-wrap-up`).

> **Your old SQLite memory is not migrated.** v0.50 does not carry a pre-v0.50
> `mom.db` into the new vault — the vault rebuilds from go-forward capture in the
> Ledger. `mom upgrade` detects a leftover `mom.db` and prints the command to
> remove it once you have confirmed capture is working.

### Superseded ADRs

ADR 0024 supersedes 0006, 0007, 0010, 0011, 0012, 0013, and 0022 in full, and
0009 and 0015 in part. ADR 0023 (MCP server retirement) is realized in full.
