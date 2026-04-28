<p align="center">
  <img src="assets/logo.png" width="180" alt="MOM">
</p>

<h2 align="center">MOM<br><sub><em>She remembers, so you don't have to_</em></sub></h2>

<p align="center">
  <a href="https://github.com/momhq/mom/releases"><img src="https://img.shields.io/github/v/release/momhq/mom?style=flat-square&color=FFCC2C" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-4A6B3A?style=flat-square" alt="License"></a>
  <a href="https://github.com/momhq/mom"><img src="https://img.shields.io/badge/go-1.26+-3B1F0A?style=flat-square" alt="Go 1.26+"></a>
  <a href="https://goreportcard.com/report/github.com/momhq/mom/cli"><img src="https://goreportcard.com/badge/github.com/momhq/mom/cli?style=flat-square" alt="Go Report Card"></a>
</p>


Your AI assistant forgets everything between sessions. You re-explain decisions, conventions, architecture — every time. MOM fixes that.

**MOM** (Memory Oriented Machine) is an open-source CLI that gives AI agents persistent, structured memory. Decisions, constraints, patterns, and learnings — stored in your project, loaded automatically, evolving with every session. Runtime-agnostic. On-prem. Schema-validated.

Self-hosting since v0.2 — MOM builds itself with its own memory.

## Quick Start

```bash
# Install via Homebrew
brew tap momhq/tap
brew install mom

# Update
brew update && brew upgrade mom

# Or build from source
git clone https://github.com/momhq/mom.git
cd mom/cli && make install

# Initialize in your project
cd your-project
mom init

# Done. Your agent now has persistent memory.
```

## How It Works

MOM creates a `.mom/` directory in your project — a structured memory layer your AI agent loads at every session.

```
your-project/
├── .mom/                           # MOM's home
│   ├── config.yaml                 # preferences (language, communication mode)
│   ├── schema.json                 # document schema (v2)
│   ├── identity.json               # project identity
│   ├── memory/                     # memory documents (structured JSON)
│   ├── constraints/                # always-active guardrails
│   ├── skills/                     # composable procedures
│   ├── raw/                        # continuous session capture (JSONL)
│   ├── logs/                       # session logs
│   └── cache/
│
├── .mcp.json                       # MCP server config (auto-injected)
├── .claude/CLAUDE.md               # auto-generated boot file for Claude Code
└── your code...
```

You work with your agent. MOM validates, indexes, and delivers memory to the runtime. Switch runtimes without losing anything.

## What Makes It Different

**Memory compounds.** Month 6 is structurally richer than month 1. Your agent knows the web of decisions behind your codebase. No one starting from zero can match months of accumulated context.

**Runtime-agnostic.** Memory lives in `.mom/`, not in `.claude/` or `.cursor/`. MOM generates the right context for each runtime through adapters. Your memory is yours, not locked to a vendor.

**Schema-validated.** Every memory document is tagged, scoped, and promotion-managed. Not a loose Markdown file — a structured, queryable memory with free-form content.

**MCP-first.** MOM delivers context via Model Context Protocol. Agents search, read, and write memory through MCP tools — no file parsing, no guesswork. `.mcp.json` is auto-injected on `mom init`.

**On-prem by default.** Your memory stays in your repo. No cloud dependency. No data leaving your machine.

## Commands

| Command | What it does |
|---------|-------------|
| `mom init` | Interactive onboarding — runtime, language, mode |
| `mom status` | Memory summary — document count, tags, health |
| `mom doctor` | Diagnostic checks on `.mom/` health |
| `mom recall <query>` | Search across memory (SQLite FTS5) |
| `mom record` | Record raw conversation data (legacy — watcher preferred) |
| `mom draft` | Extract memory drafts from raw session capture |
| `mom log` | Generate session-level observability data from transcript |
| `mom diagnose` | Compute derived metrics from session logs |
| `mom map` | Cartographer — scan repo and generate initial memory |
| `mom tour` | Show top landmark memories at current scope |
| `mom promote <id>` | Move a memory doc up to a broader scope |
| `mom demote <id>` | Move a memory doc down to the nearest scope |
| `mom validate` | Validate documents against schema |
| `mom export` | Export memory to portable directory |
| `mom import` | Import memory (merge or replace) |
| `mom reindex` | Rebuild the SQLite search index from JSON memory files |
| `mom watch` | Watch runtime transcripts and ingest turns automatically |
| `mom sweep` | Delete old raw JSONL recordings based on retention policy |
| `mom serve mcp` | Start MCP stdio server |
| `mom serve status` | Show MCP server activity |
| `mom upgrade` | Upgrade `.mom/` to the latest version (preserves memory) |
| `mom uninstall` | Remove all MOM files from this project |
| `mom version` | Print version |

## Supported Runtimes

| Runtime | MCP | Watcher | Boot file | Status |
|---------|-----|---------|-----------|--------|
| Claude Code | Yes | Yes | CLAUDE.md | Full support |
| OpenAI Codex | Yes | — | AGENTS.md | Boot file + MCP |
| Windsurf | Yes | Yes | .windsurf/rules/ | Full support |

## Current Status

MOM is in active development (v0.13). It works, and it self-hosts — the tool builds itself with its own memory.

What's in v0.13:
- **Watcher-based ingestion** — global daemon watches Claude Code and Windsurf transcripts via fsnotify, replacing hook-based recording
- **SQLite FTS5 search** — `mom_recall` and MCP search use a full-text index, self-healing from JSON source of truth
- **Global watch daemon** — single launchd (macOS) or systemd (Linux) service manages all registered projects
- **Runtime-specific logbook parsing** — `SessionParser` adapter interface with native parsers per runtime
- **CLI design system** — spinners, color-coded output, structured status views across all commands
- **MCP-first context delivery** — behavioral protocol via `mom_status` tool, `.mcp.json` auto-injected
- **Drafter pipeline** — RAKE + BM25 extraction from raw capture into memory drafts
- **Cartographer** — AST-based repo scanning for initial memory bootstrap
- Three runtime adapters (Claude Code, Codex, Windsurf)
- Communication modes (verbose, concise, normal, caveman)
- Multi-repo support with scope-based memory
- Homebrew installation with automated tap updates

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for setup, conventions, and how to submit PRs.

If you work with AI agents and feel the amnesia pain — issues, feedback, and honest criticism are welcome.

## License

[Apache 2.0](LICENSE)
