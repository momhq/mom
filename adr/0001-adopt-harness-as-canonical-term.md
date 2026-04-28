# 0001 — Adopt "Harness" as the canonical term for AI agent frameworks

MOM integrates with AI agent frameworks (Pi, Claude Code, Codex, Windsurf, Cursor, Cline, ...). The codebase and architecture docs originally called these **Runtimes** — a term overloaded in computer science (language runtime, container runtime, Go's `runtime` package) and giving no signal that the integration target is specifically an AI agent framework.

We adopt **Harness** as the canonical term going forward. It is the AI-domain term of art (Pi describes itself as a "coding agent harness"; the broader ecosystem uses it for the framework around an LLM that adds tools, hooks, MCP, and prompt scaffolding). It is unambiguous in this domain and carries product-narrative leverage.

## Consequences

- All new code, docs, and PRs use **Harness** / `HarnessAdapter` / `harness` naming.
- The legacy term **runtime** remains in the existing codebase and is migrated incrementally (`cli/internal/adapters/runtime/` → `cli/internal/adapters/harness/`, `RuntimeAdapter` → `HarnessAdapter`, etc.) as a separate mechanical PR.
- Existing architecture docs (`mom-engineer/architecture/v1/glossary.md`, `11-full-architecture.md`, top-level `README.md`) are updated in the same migration PR.
- `CONTEXT.md` flags "runtime" as the alias to avoid.
