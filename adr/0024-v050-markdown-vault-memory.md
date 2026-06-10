# 0024 — v0.50 markdown-vault memory (SQLite, Crier, event bus, and worker pipeline retirement)

v0.50 replaces MOM's SQLite-backed memory model with a per-project **markdown vault** materialized directly from the Ledger. This ADR records the as-built architecture and supersedes the decisions it makes obsolete.

## Decision

Memory is delivered as plain markdown the agent reads with its file tools — not as queryable rows behind a server.

```
Harnesses/User → Watcher / CLI → Editor → Ledger
                                            │  (mom vault fold / rebuild / daemon timer)
                                            ▼
                                services/projection (Reader → Synthesizer → Writer)
                                            │
                                            ▼
                                .mom/vault/*.md   ← agents read directly
```

- **Editor → Ledger is the only write path.** The Editor canonicalizes a parsed turn into a canonical event and appends it to the append-only Ledger (ADR 0021). There is no event bus; nothing else publishes anywhere.
- **Capture is privacy-gated.** The watcher records a turn only when the directory is bound to a project (`.mom-project.yaml`), enforced in the Editor publish path.
- **The fold is the projection.** `services/projection` reads the Ledger for one project and synthesizes markdown (LLM engine — claude/codex/pi — or a deterministic fallback) into `.mom/vault/`. `garden` is an LLM reorganization pass.
- **Read is direct.** Agents open `.mom/vault/INDEX.md` and navigate; `mom status` and `mom lens` read the Ledger. There is no search service and no MCP server.

## What this supersedes

| ADR | Subject | Status |
|---|---|---|
| 0006 | Unified recall, progressive escalation | **Superseded** — `mom recall` / the search read path is removed; recall is vault-file navigation. |
| 0007 | FTS5 content-first column weights | **Superseded** — the FTS5 Finder is deleted. |
| 0009 | Storage consolidation under `~/.mom` | **Partially superseded** — `~/.mom` as the canonical home survives; the SQLite vault (`mom.db`) and Librarian-as-gate do not. |
| 0010 | Graph fluent schema normalization | **Superseded** — the SQLite graph store is deleted. |
| 0011 | Substance immutability / operational mutability | **Superseded** — the SQLite memory model is deleted; the vault is regenerated from the Ledger. |
| 0012 | Tulving typology, default untyped | **Superseded** — memory typing lived in the SQLite vault. |
| 0013 | UUID-only memory IDs | **Superseded** — SQLite memory rows are gone; vault chunks are content-addressed. |
| 0015 | Bootstrap MCP instructions + global agent marker | **Partially superseded** — the global-agent context marker survives (and is now vault-first); the MCP instruction delivery is retired (see ADR 0023). |
| 0022 | Crier as projector/replayer via Librarian | **Superseded** — there is no Crier and no Librarian-CRUD; the fold projects the Ledger into markdown directly. |

ADR 0023 (MCP server retirement) is **realized in full** here rather than superseded: v0.50 removed the MCP server, harness registration, and `ingress/record`.

## Retired packages

`storage/vault`, `storage/canonical`, `storage/legacy`, `storage/librarian` (the SQLite CRUD layer — the name is reborn as a pure path resolver), `events/crier`, `workers/drafter`, `workers/logbook`, `workers/cartographer`, `workers/gardener`, `services/finder`, `ingress/mcp`, `ingress/record`, and `bus/herald` (the event bus — its canonical envelope moved to `events/envelope`).

## Consequences

- **Librarian** keeps its name but is now a path resolver (`Dir` / `LedgerDir` / `Path`), not a data gate. The "all data access routes through Librarian" invariant is retired; agents read vault files directly.
- **No live event bus.** The write path is a straight line; "where do events go?" has one answer (the Ledger). The canonical envelope is `events/envelope.Event`.
- **The Ledger is the durable backbone.** Lose the vault and `mom vault rebuild` regenerates it; the Ledger is never derived.
- **Telemetry** (the unconsumed smoke-event emitter) is removed; if operational telemetry is wanted, it should read the Ledger rather than re-introduce a bus.
