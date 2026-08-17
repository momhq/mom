<div align="center">

<img src="assets/logo.svg" alt="MOM" width="120" />

# _mom_

_Memory Oriented Machine — she remembers, so you don't have to._

<p>
  <a href="https://github.com/momhq/mom/releases"><img src="https://img.shields.io/github/v/release/momhq/mom?style=flat-square&color=FFCC2C" alt="Release"></a>
  <a href="https://github.com/momhq/mom/actions"><img src="https://img.shields.io/github/actions/workflow/status/momhq/mom/ci.yml?style=flat-square&label=CI" alt="CI"></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.26+-3B1F0A?style=flat-square" alt="Go 1.26+"></a>
  <a href="https://github.com/momhq/homebrew-tap"><img src="https://img.shields.io/badge/Homebrew-momhq/tap-4A6B3A?style=flat-square" alt="Homebrew tap"></a>
  <a href="https://github.com/momhq/mom/blob/main/LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-4A6B3A?style=flat-square" alt="Apache 2.0 license"></a>
  <a href="https://skills.sh/momhq/mom"><img src="https://skills.sh/b/momhq/mom" alt="skills.sh installs"></a>
</p>

[Install](#install) • [Quick start](#quick-start) • [How it works](#how-it-works) • [Typical workflow](#typical-workflow) • [Harness support](#harness-support)

</div>

_Mom_ gives AI coding agents persistent memory across sessions, projects, and tools.

Instead of re-explaining architecture, decisions, conventions, and constraints every time you start a new chat, _mom_ records them to an append-only Ledger and projects them into a per-project **markdown vault** your agent reads with its normal file tools.

> [!IMPORTANT]
> `v0.53.0-alpha` is the current public alpha. It replaces the SQLite vault and MCP server with a per-project markdown vault projected from an append-only Ledger (see [ADR 0024](adr/0024-v050-markdown-vault-memory.md)). Pi, Claude Code, and Codex flows are validated end-to-end. Upgrading from an older install? See [Upgrade](#upgrade-from-older-mom-installs).

## Why _mom_?

AI agents are useful, but they forget.

_Mom_ is the memory layer beside them:

- **Persistent** — memory survives `/clear`, compaction, restarts, and tool switches.
- **Local-first** — captured turns live in an append-only Ledger at `$HOME/.mom/ledger/`; the projected vault lives per-project under `.mom/vault/`.
- **Plain markdown** — memory is `.md` files you can open, read, and diff. The agent reads them with its normal file tools; there is no query server.
- **Harness-agnostic** — Pi, Claude Code, and Codex are integration targets, not storage silos.
- **Agent-integrated** — memory is delivered through the harness context file and _mom_ skills.
- **Continuously recorded** — supported harness transcripts are watched and folded into the vault.

## Install

### Homebrew

```bash
brew install momhq/tap/mom
```

To upgrade:

```bash
brew update && brew upgrade mom && mom version
```

### From source

```bash
git clone https://github.com/momhq/mom.git
cd mom
make install
```

## Quick start

Initialize _mom_ once:

```bash
mom init
```

_Mom_ will set up its central store at `~/.mom` (config + Ledger), configure detected harness integrations, install _mom_ skills where supported, ask whether to start capturing the current directory, and start the watch daemon.

Then open your agent and work normally. _Mom_ records sessions in the background and folds them into the vault. At the start of each session your agent reads `.mom/vault/` on its own, so it already knows your project's history — just ask. The skills you'll actually run:

```text
/mom-fold     # save recent sessions into the vault (e.g. when wrapping up)
/mom-status   # check that mom is capturing, and what it knows
```

## How it works

The write path is a straight line; agents read the projected markdown directly.

```text
AI harnesses (Pi · Claude Code · Codex)
        │  transcripts
        ▼
mom watcher → Editor → Ledger          ($HOME/.mom/ledger/, append-only)
        │
        │  mom vault fold  (/mom-fold, or the daemon timer)
        ▼
services/projection  (Reader → Synthesizer → Writer)
        │
        ▼
.mom/vault/*.md   ← the agent reads these with its file tools
  ├─ INDEX.md               # routing-only: what to read for what task
  ├─ identity.md             # layer A: always-load project orientation
  ├─ reference/INDEX.md      # layer B: decisions, facts, ingested books
  ├─ conventions/INDEX.md    # layer B: process/workflow rules
  └─ episodes/<hash>.md      # layer C: raw provenance, evidence-loaded last
```

Lose the vault and `mom vault rebuild` regenerates it from the Ledger; the Ledger is the durable backbone and is never derived.

## Typical workflow

After installing _mom_, open your agent and work normally.

_Mom_ watches supported transcript sources in the background and records them to its Ledger. After a long session, or whenever you want to preserve recent context, ask your agent to run:

```text
/mom-fold
```

The skill folds newly captured sessions into a navigable markdown vault under `.mom/vault/`.

There is no recall command to run. At the start of each session your agent reads `.mom/vault/INDEX.md` and follows it to the relevant memory, so it answers from what _mom_ already knows — decisions, conventions, and context — without you re-explaining them. Just work normally.

To check that _mom_ is connected:

```text
/mom-status
```

### Explore with Lens

For a visual view of captured sessions and privacy-projected tool activity, run:

```bash
mom lens
```

Lens is local (loopback only) and reads the append-only Ledger.

## Harness support

| Harness | Current status | Notes |
| --- | --- | --- |
| Pi | Validated | Native extension support via `pi install npm:pi-mom`. Gold standard for _mom_. |
| Claude Code | Validated | Fluent speaker. Provides all the necessary tools _mom_ needs. |
| Codex | Validated | Hooks and per-turn project scoping working end-to-end. |

> [!NOTE]
> _Mom_ uses **harness** to mean the agent framework around the model: tools, hooks, transcripts, and prompt/context files.

## Upgrade from older _mom_ installs

If you already have an older _mom_ install:

```bash
mom upgrade --dry-run
mom upgrade
```

Upgrade regenerates the harness context blocks, tears down the retired MCP registration, removes obsolete hook commands and deprecated skills, and installs or updates the current skills as a soft-fail step.

> [!IMPORTANT]
> v0.50 does **not** migrate a pre-v0.50 SQLite vault (`$HOME/.mom/mom.db`). The new vault rebuilds from go-forward capture in the Ledger. `mom upgrade` detects a leftover `mom.db` and prints the command to remove it once capture is confirmed.

## Data and privacy

_Mom_ is local-first.

- The append-only Ledger lives at `$HOME/.mom/ledger/`; the projected vault lives per-project under `.mom/vault/`.
- `MOM_VAULT=/path/to/dir` overrides the central `$HOME/.mom` location for tests or isolated runs.
- Capture is privacy-gated: a turn is only recorded when the directory is bound to a project (`.mom-project.yaml`).
- Lens uses privacy-projected metadata.
- Raw tool arguments, raw user text, shell command arguments, query strings, paths, and flags are not stored as operational log detail.
- Explicit record flows reject invented session IDs; harness session IDs must come from the harness.

## Troubleshooting

### Check installation

```bash
mom version
mom status
mom doctor
```

### Watcher is not seeing new sessions

```bash
mom watch --status
mom watch --sweep
```

_Mom_ canonicalizes project paths, including macOS `/tmp` and `/private/tmp`, so watcher state should not split across symlinked aliases.

### Skills are missing

Run init or upgrade again:

```bash
mom init
# or
mom upgrade
```

Skills install is a soft-fail step, so _mom_ can be usable even if the external skills installer is temporarily unavailable.

## Project layout

The codebase is organised into role-based top-level buckets (see [ADR 0017](adr/0017-role-based-repo-layout.md)).

```text
.
├── cmd/mom/                # entrypoint
├── ingress/                # external input: CLI, watcher adapters, harness detection
├── events/                 # canonical event Editor, schema Registry, envelope
├── services/               # projection (the fold) and lens (dashboard)
├── storage/                # durable state — ledger (append-only), librarian (path resolver)
├── ops/                    # background lifecycle — daemon, diagnose
├── shared/                 # cross-cutting utilities — config, pathutil, scope, project, ux, archtest
├── adr/                    # architecture decisions
├── prd/                    # product requirements
├── skills/                 # _mom_ slash skills
├── assets/                 # logo and brand assets
├── Formula/mom.rb          # Homebrew formula
└── .github/workflows/      # CI and release automation
```

## Resources

- [Latest release](https://github.com/momhq/mom/releases/latest)
- [Issues](https://github.com/momhq/mom/issues)
- [Homebrew tap](https://github.com/momhq/homebrew-tap)
- [Privacy policy](https://github.com/momhq/mom/blob/main/PRIVACY.md)
