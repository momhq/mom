<p align="center">
  <img src="assets/logo.png" width="180" alt="MOM">
</p>

<h2 align="center">MOM<br><sub><em>She remembers, so you don't have to_</em></sub></h2>

<p align="center">
  <a href="https://github.com/momhq/mom/releases"><img src="https://img.shields.io/github/v/release/momhq/mom?style=flat-square&color=FFCC2C" alt="Release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-4A6B3A?style=flat-square" alt="License"></a>
  <a href="https://github.com/momhq/mom"><img src="https://img.shields.io/badge/go-1.22+-3B1F0A?style=flat-square" alt="Go 1.22+"></a>
  <a href="https://goreportcard.com/report/github.com/momhq/mom/cli"><img src="https://goreportcard.com/badge/github.com/momhq/mom/cli?style=flat-square" alt="Go Report Card"></a>
</p>


Your AI assistant forgets everything between sessions. You re-explain decisions, conventions, architecture — every time. MOM fixes that.

**MOM** (Memory Oriented Machine) is an open-source CLI that gives AI agents persistent, structured memory. Decisions, constraints, patterns, and learnings — stored in your project, loaded automatically, evolving with every session. Runtime-agnostic. On-prem. Schema-validated.

```
Monday without MOM:                Monday with MOM:

"We use Go with Cobra for the CLI"   "Add the export command."
"Tests go in the same package"       → Your agent already knows the stack,
"Don't mock the database"             the conventions, and the decisions
"JWT auth, not sessions"               from last week.
"The deploy target is AWS Lambda"
```

Self-hosting since v0.2 — MOM builds itself with its own memory.

## Quick Start

```bash
# Install via Homebrew
brew install momhq/tap/mom

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
│   ├── config.yaml                 # preferences (language, communication mode, autonomy)
│   ├── index.json                  # tag-based memory index
│   ├── schema.json                 # document schema
│   ├── identity.json               # project identity
│   ├── memory/                     # memory documents (decisions, patterns, learnings)
│   ├── constraints/                # always-active guardrails
│   ├── skills/                     # composable procedures
│   ├── logs/                       # session logs
│   └── cache/
│
├── .claude/CLAUDE.md               # auto-generated boot file for Claude Code
└── your code...
```

You work with your agent. MOM validates, indexes, and delivers memory to the runtime. Switch runtimes without losing anything.

## What Makes It Different

**Memory compounds.** Month 6 is structurally richer than month 1. Your agent knows the web of decisions behind your codebase. No one starting from zero can match months of accumulated context.

**Runtime-agnostic.** Memory lives in `.mom/`, not in `.claude/` or `.cursor/`. MOM generates the right context for each runtime through adapters. Your memory is yours, not locked to a vendor.

**Schema-validated.** Every memory document is typed, tagged, scoped, and lifecycle-managed. Not a loose Markdown file — a structured, indexed, queryable memory.

**MCP-native.** MOM exposes tools via Model Context Protocol. Agents search, read, and write memory through MCP — no file parsing, no guesswork.

**On-prem by default.** Your memory stays in your repo. No cloud dependency. No data leaving your machine.

## Commands

| Command | What it does |
|---------|-------------|
| `mom init` | Interactive onboarding — runtime, language, mode, autonomy |
| `mom status` | Memory summary — document count, tags, health |
| `mom doctor` | Diagnostic checks on `.mom/` health |
| `mom recall <query>` | Search memory by query |
| `mom promote <id>` | Promote a draft memory to active |
| `mom demote <id>` | Demote a memory back to draft |
| `mom reindex` | Rebuild index from documents on disk |
| `mom validate` | Validate documents against schema |
| `mom export` | Export memory to portable directory |
| `mom import` | Import memory (merge or replace) |
| `mom upgrade` | Migrate from older versions |
| `mom tour` | Guided walkthrough of your memory |
| `mom serve mcp` | Start MCP server |
| `mom version` | Print version |

## Supported Runtimes

| Runtime | Adapter | Status |
|---------|---------|--------|
| Claude Code | Context file + MCP + hooks | Stable |
| OpenAI Codex | Context file | Stable |
| Cline | Context file | Stable |

## Current Status

MOM is in active development (v0.10). It works, and it self-hosts — the tool builds itself with its own memory.

What's working:
- Structured memory with schema validation and tag-based indexing
- Three runtime adapters (Claude Code, Codex, Cline)
- MCP server with search, read, and write tools
- Communication modes (verbose, concise, caveman)
- Session logging and memory lifecycle management
- Multi-repo support with scope-based memory
- Memory graph visualization
- Homebrew installation

What's next (v0.11):
- MCP-first context delivery — behavioral protocol via `mom_status` tool
- Continuous raw capture via hooks
- Herald event bus and Drafter extraction pipeline

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for setup, conventions, and how to submit PRs.

If you work with AI agents and feel the amnesia pain — issues, feedback, and honest criticism are welcome.

## License

[Apache 2.0](LICENSE)
