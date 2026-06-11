# Changelog

All notable changes to _mom_ are documented here. The format is loosely based on
[Keep a Changelog](https://keepachangelog.com/), and the project follows a
`vN0.0-alpha` cadence during the alpha line.

## v0.50.0-alpha — Memory is now markdown

_mom_ retires the SQLite vault and the MCP server. Your project's memory is now
plain markdown under `.mom/vault/`. It's written from an append-only log — the
Ledger — that _mom_ keeps on disk, but your agent never queries that log. It reads
the markdown files with the tools it already has.

```
Harnesses / CLI → Editor → Ledger ($HOME/.mom/ledger/, append-only)
                              │  mom vault fold  (/mom-fold, or the daemon)
                              ▼
                  .mom/vault/*.md   ← your agent reads these directly
```

### Why we changed this

Before v0.50, memory lived in a SQLite database. Your agent reached it two ways:
the `mom` CLI and an MCP server wired into each harness. In recent releases the CLI
had already become the primary path, and we were planning to retire the MCP server
regardless — it was fragile, different on every harness, and one more thing to keep
connected.

The bigger problem was the database itself. What _mom_ remembered was rows you
couldn't see or edit. To know what was stored you had to query it; to correct a bad
memory, you mostly couldn't. And keeping recall useful meant running a search
engine, an event bus, and background workers on top of the store.

v0.50 changes what memory *is*. Agents already know how to read files, so memory is
now markdown in your project. The agent opens `.mom/vault/` directly — nothing
queries a store on its behalf.

To be precise, the database didn't vanish — it changed shape. Every captured turn
still lands in a store on disk: an append-only log called the Ledger, the durable
source of truth. What's gone is the queryable relational store and the server in
front of it. The Ledger is never queried; it's folded into plain markdown, and the
markdown is what the agent reads.

What that gets you:

- **Transparent.** Open `.mom/vault/` in any editor and read exactly what _mom_
  knows. Edit or delete anything by hand.
- **Durable.** It's plain text. Nothing to corrupt, and it diffs cleanly in git.
- **Simpler.** We deleted the SQLite store, the MCP server, the search engine, the
  event bus, and the worker pipeline. Fewer parts, fewer failures.
- **Outlasts churn.** Memory isn't tied to a database schema, a protocol, or a
  specific model, so it keeps working as harnesses and models change.

### How the vault is built (the fold)

You don't write or maintain the markdown yourself. _mom_ does it, in a step called
the **fold**.

The fold doesn't ship its own model or service. It runs the coding-agent CLI you
already have installed — Claude Code, Codex, or Pi — and asks it to read the new
entries in the Ledger and organize them into the vault's structure: a router
`INDEX.md` that points to everything, `topics/<subject>.md` for decisions and
patterns by subject, `timeline/<month>.md` for chronological history, and a
high-level `summaries/overview.md`. The same tool you code with is what organizes
your memory, so there's nothing extra to install or run.

_mom_ doesn't pin a model for the fold — synthesis runs on whatever model your
harness CLI defaults to. It isn't user-selectable yet, but letting you choose the
synthesis model is something we plan to add.

Capture is automatic — a background watcher records sessions into the Ledger — and
the fold runs when you call `/mom-fold` or on the daemon's timer. You only touch the
files if you *want* to.

We've been running this on our own work for several weeks now. In practice it has
worked noticeably better than querying the old SQLite memory: the agent starts from
a short, structured, human-readable summary instead of digging through search hits.

Honest tradeoffs: there's no sub-second indexed search anymore, and no automatic
migration from the old SQLite memory — see Upgrading. Full design rationale and the
list of superseded decisions are in
[ADR 0024](adr/0024-v050-markdown-vault-memory.md).

### What's new

- Per-project markdown vault: a router `INDEX.md` plus `topics/`, `timeline/`, and
  `summaries/`, synthesized from the Ledger.
- New skills `/mom-fold` and `/mom-rebuild`; new CLI
  `mom vault fold | rebuild | status | garden`.
- Privacy-gated capture: a session is only recorded in directories bound to a
  project (`.mom-project.yaml`).
- `mom init` now asks whether to start capturing the current directory, so memory
  works from the first session.
- Lens rebuilt on the Ledger; `mom --version` added.

### Breaking changes

- SQLite vault removed (`$HOME/.mom/mom.db`). `MOM_VAULT` now overrides the central
  directory, not a `.db` file.
- MCP server removed — no `mom serve mcp`, no `mom_recall` / `mom_record` /
  `mom_get` / `mom_landmarks` tools.
- `mom recall` search removed — recall is now navigation of the markdown vault.
- `mom export` / `mom import` removed (they dumped SQLite).
- `mom-wrap-up` and `mom-recall` skills removed. The agent reads the vault
  directly at session start, so there is no recall command to run; save with
  `/mom-fold`.

### Upgrading

```bash
brew update && brew upgrade mom
mom upgrade --dry-run   # preview
mom upgrade
```

`mom upgrade` regenerates the harness context blocks, tears down the retired MCP
registration (Claude and Codex), removes obsolete hooks, and prunes the deprecated
`mom-wrap-up` skill.

> Your old SQLite memory is not migrated. v0.50 does not carry a pre-v0.50 `mom.db`
> into the new vault — the vault rebuilds from go-forward capture. `mom upgrade`
> detects a leftover `mom.db` and prints the command to remove it once capture is
> confirmed.

### Validated harnesses

Claude Code, Codex, Pi.

**Full changelog:** https://github.com/momhq/mom/compare/v0.40.0-alpha...v0.50.0-alpha
