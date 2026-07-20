# Changelog

All notable changes to _mom_ are documented here. The format is loosely based on
[Keep a Changelog](https://keepachangelog.com/), and the project follows a
`vN0.0-alpha` cadence during the alpha line.

## v0.52.0-alpha — The ICM vault

The projected-memory vault is now a proper **ICM** (Interpretable Context
Methodology) structure in **OKF** (Open Knowledge Format): raw episodes roll up
into one concept file per subject (`reference/` for durable facts, `contracts/`
for process rules), then into a living `identity.md`, each folder carrying its
own `INDEX.md` router. Synthesis is **LLM-only** — the vault's structure depends
on reasoning, so the templated fallback engine is gone. Folds are now
interrupt-safe, parallel, cheap by default, and cannot destroy a vault.

This release also lands **operational event ingress** (ADR 0025): a loopback
HTTP API and the OATS transcript adapter let momOS and any conformant writer
feed memory, and the global daemon **auto-folds** vaults in the background. And
_mom_ now builds and runs on **Windows**.

### Added

- **ICM/OKF vault.** The vault is projected from the Ledger into ICM layers —
  L0 episodes → L1 `reference/`/`contracts/` concepts (one per subject, updated
  in place) → L2 `identity.md` — written in OKF with per-folder `INDEX.md`
  routers and `type`/`name`/`description` frontmatter an agent scans before
  opening. Concepts are structured documents (titled, sectioned) cross-linked
  into a navigable graph by shared source-provenance.
- **Operational event ingress (ADR 0025).** A loopback HTTP server inside the
  watch daemon accepts the `operational.*` event family (8 types), and the
  **OATS** transcript adapter ingests momOS sessions and any conformant writer.
- **Automatic vault folding.** The global watch daemon folds project vaults in
  the background (idle/backlog-triggered, per-project quota-guarded), and
  restarts itself onto a freshly installed binary from the sweep path.
- **Parallel, checkpointed folds.** L0/L1 synthesis runs concurrently
  (`--parallel`, default 4) and every completed step is persisted immediately,
  so an interrupted fold (Ctrl-C, crash, usage-limit) loses at most one call and
  resumes exactly where it stopped.
- **Windows support.** _mom_ cross-compiles and runs on Windows (amd64 + arm64);
  the release ships `.exe` artifacts. Background auto-capture degrades gracefully
  (foreground `mom watch` + `mom vault fold`) where launchd/systemd is absent.
- **Cheap-by-default synthesis.** Folds pin the engine's cheapest model
  (Claude: haiku); override with `--model` or the `vault.fold_model` config key.

### Changed

- **Harness-agnostic entry files.** The managed context block is written to each
  enabled harness's entry file — `CLAUDE.md` for Claude Code, `AGENTS.md` for
  Codex and Pi — instead of assuming Claude.
- **Fact-time dates.** Concept and episode `time_range` frontmatter now reflects
  the dates of the underlying captured events, not the fold time, so recency is
  meaningful.

### Removed

- **`mom vault garden`** (breaking). Its dedup/merge job is now handled
  structurally by the ICM fold (one concept per subject, update-in-place), and
  its whole-vault prune path was a latent data-loss hazard.
- **The deterministic (non-LLM) fold engine.** Synthesis is LLM-only.

### Fixed

- **Vaults are never destroyed by a failed fold.** A rebuild prunes stale files
  only when it fully completes; an aborted rebuild keeps the existing vault, and
  the watermark never advances past events that were not actually synthesized.
- **Synthesis robustness.** Prompts carry no machine-owned provenance (fixed an
  L2 context overflow), hooks are disabled in fold subprocesses (they no longer
  fail the call), harness errors surface actionably (e.g. a logged-out CLI), and
  a systemic failure aborts the pass instead of hammering the CLI.
- **Security hardening.** The ingest server rejects cross-origin/DNS-rebinding
  requests, caps body size, and sets server timeouts; watcher logs are no longer
  world-readable.

## v0.50.1-alpha

Patch release for the v0.50 line.

### Fixed

- **Fold JSON parsing.** `mom vault fold` could fail to read the synthesizer's
  output when the model prepended prose containing braces (e.g. *"the config
  {mode: gateway}. Here is the vault: {…}"*) — producing `invalid character …
  after object key:value pair` errors and a silent fall back to the deterministic
  (non-LLM) engine for those chunks. The extractor now finds the actual JSON
  envelope (the object carrying `files`/`index`) instead of trusting the first
  brace it sees, so a brace-laden preamble no longer breaks the fold.

### Internal

- CI hygiene after the v0.50 dependency removals: `go mod tidy` (dropped the now
  unused SQLite/finder modules) and the coverage floor adjusted for the smaller
  codebase.

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
