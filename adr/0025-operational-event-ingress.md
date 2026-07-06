# 0025 — Operational event ingress (Toad OS company record)

## Context

Toad OS is a founder command center that orchestrates agents, channels, tasks, and approvals. It needs a canonical, durable record of every company-level operation — who said what, which agent ran, whether an action was approved. MOM's Ledger is already the durable spine for capture events (ADR 0021); making it the canonical company record is a natural extension.

External local clients (the Toad OS daemon and its agents) need to:

1. **Append** company-record events to MOM's Ledger: chat messages, agent runs, human approvals, tasks, workspace registrations.
2. **Read** them back for projection rebuild or downstream queries.

The write path must respect MOM's architectural invariant (ADR 0020): all writes go through `editor.Editor.Publish`, never directly to the Ledger. And the Ledger is single-writer-per-process (ADR 0021): a separate appender process is forbidden.

## Decision

### Ingress server inside the watch daemon

The ingest HTTP server (`services/ingest`) runs **inside `mom watch --global`**, sharing the existing `*editor.Editor` and `*ledger.Ledger` instances that the watcher already holds. No separate process opens the Ledger for writing.

```
Toad OS daemon ──HTTP POST──▶ services/ingest (port 7475)
                                      │ editor.Editor.Publish
                                      ▼
                               storage/ledger (shared handle)
                                      │  (mom watch --global reads back via GET)
                                      ▼
                           services/projection/Reader (fold)
```

### Editor-only writes

`services/ingest` calls `editor.Editor.Publish(rawCanonicalizer, Source{Adapter:"ingest"})` for every event. The Editor canonicalizes, stamps `provenance_actor="ingest"`, validates against the registry (level B — marks but never blocks), and appends to the Ledger. No `ledger.Append` call exists in the ingest package.

### Loopback HTTP, port 7475

The server binds on `localhost:7475` (loopback only) using `ingest.ListenWithFallback`, which is a direct copy of the `services/lens` listener pattern. Loopback-only prevents company-record data from being exposed to other machines on the network. Up to 10 fallback ports are tried (7475–7485).

Two flags are added to `mom watch`:
- `--ingest-port` (default 7475) — override the bind port.
- `--no-ingest` — disable the server entirely.

The server starts only in persistent (non-sweep) global mode. Non-global `mom watch` does not start ingress.

### New package: services/ingest

Write paths are prohibited in `services/lens` by its archtest. A new package `services/ingest` carries the HTTP handlers without contaminating the read-only lens surface.

### Operational event family

Eight event types are added under the `operational` family (already an allowed family in ADR 0018 / the registry):

| Event type | What it records |
|---|---|
| `operational.message.posted` | A message posted in a channel or thread |
| `operational.run.started` | An agent run began |
| `operational.run.finished` | An agent run completed |
| `operational.approval.requested` | A human approval was requested |
| `operational.approval.resolved` | A pending approval was resolved |
| `operational.task.created` | A task was created |
| `operational.task.updated` | A task's status or owner changed |
| `operational.workspace.registered` | A workspace was registered |

Each type has a JSON schema under `events/registry/schemas/operational/` and a constant in `events/envelope`. The coverage test `events/registry/coverage_test.go` is updated to include all eight.

### HTTP API

```
POST /api/ingest/events
  Body: single {type, payload} or array thereof.
  Validates: type has prefix "operational." (400 otherwise);
             payload.project_id is present (400 if missing).
  Response: {appended: n, head: <offset>}

GET  /api/ingest/events?from=<offset>&project=<id>&type_prefix=<p>&limit=<n≤1000>
  Iterates the Ledger from `from`, filters by payload.project_id and event
  type prefix (default "operational."), returns up to `limit` records.
  Response: {records: [{offset, appended_at, type, payload}], next: <offset|null>, head}

GET  /api/ingest/head
  Response: {head}

GET  /api/ingest/health
  Response: {ok: true, version}
```

### Fold inclusion

`services/projection/reader.go` (the Reader that projects Ledger records for the vault fold) adds two cases:

- `operational.message.posted` with `kind` in `{chat, plan, handoff, artifact, decision}` → `FoldEvent{Type:"os.message", Role: senderName+" ("+senderType+")", Text:"[kind] body"}`. Kinds `tool_result` and `system` are dropped — they are machinery, not company-record prose.
- `operational.approval.resolved` → `FoldEvent{Type:"os.approval", Text:"approval "+status[+": "+note]}`.

All other operational types (`run.*`, `task.*`, `workspace.registered`, `approval.requested`) are stored durably in the Ledger but not folded into the markdown vault. They are available to read via `GET /api/ingest/events`.

## Why this does not violate ADR 0023

ADR 0023 retired the MCP *invocation* server — a long-lived process through which LLM harnesses called MOM operations as tools. That server is gone.

This ADR introduces *ingress*: an HTTP surface through which an external daemon appends data to the Ledger and reads it back. It is structurally identical to the transcript watcher (which is also ingress, just file-based). The ingest server does not expose MOM operations as callable tools; it does not speak MCP; it is not a harness adapter. The watch daemon is the right home for it: both the watcher and the ingest server are write surfaces for the same shared Ledger, and co-locating them upholds the single-writer-per-process invariant.

## Rejected alternatives

**Standalone `mom serve ingest` process.** Rejected because the Ledger is single-writer-per-process (ADR 0021). A standalone server would need its own Ledger handle, creating two concurrent writers. Solving that with a lock file (`flock`) is possible but a larger change than warranted; the shared-handle approach is strictly correct and simpler.

**Direct file access from Toad OS clients.** Rejected. Clients writing segment files directly would bypass the Editor's canonicalization and the registry's schema validation. The Editor invariant (ADR 0020) exists precisely to prevent this.

**`flock`-based multi-writer Ledger.** Rejected for this milestone. Making the Ledger multi-process-safe is a legitimate future concern (v0.60 retention + compaction will revisit the segment model anyway), but adding `flock` now adds complexity without benefit: the single-process model already works, and the ingest server is local.

**Adding write routes to `services/lens`.** Rejected. `services/lens` has an archtest that enforces its read-only contract. Mixing write paths into a read-only service muddies the architecture and would require updating the archtest in a way that weakens its guarantees.

**An event bus between the watch daemon and Toad OS.** Rejected. ADR 0024 specifically retired the event bus. A direct HTTP surface with the Ledger as the canonical record is simpler, durable, and consistent with the v0.50 architecture.

## Consequences

- The Ledger is now the canonical company record for Toad OS operational events. `mom vault rebuild` will include prose messages and approval resolutions in the project vault.
- The ingest server adds one goroutine and one open TCP port to the global watch daemon. It is disabled with `--no-ingest` and does not start at all in non-global mode.
- Toad OS clients communicate with MOM over HTTP (loopback). The API is intentionally minimal — it is not a general-purpose database endpoint.
- Adding new operational event types requires: (a) a schema under `events/registry/schemas/operational/`, (b) a constant in `events/envelope`, (c) an entry in `events/registry/coverage_test.go activeEventTypes`. The ingest handler accepts any `operational.*` type without code changes.
