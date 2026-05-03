# 0014 — Capture filtering: hard redaction for secrets, silent drop for noise

Continuous capture currently writes every conversational turn into the vault. Two problems follow: (a) sensitive content (secrets, credentials, private identifiers) lands in the same store as everything else with no gate, and (b) low-value turns (acknowledgements, tool noise, exact duplicates) inflate the vault and drown signal during recall.

Capture applies two filter layers at the write boundary, before a turn becomes a memory.

**Hard filter — secrets.** Pattern-based detection for credentials and high-entropy strings (provider key shapes, JWT, PEM blocks, common `KEY=`/`TOKEN=`/`SECRET=` assignments, path-based exclusions for `.env`, `*.pem`, `*.key`, etc.). When a match is found, the matched substring is replaced with `[REDACTED]` in the memory's `content` and the memory is persisted with the redaction in place — the surrounding context is usually valuable ("I was confused why my AWS creds weren't working"); the secret string is what must not be retained. Each redaction increments a counter on the `filter_audit` table. The matched substance is never stored anywhere — neither in the memory nor in the audit row.

**Soft filter — noise.** Heuristic detection for low-signal turns: trivially short replies after stop-word removal, acknowledgement and greeting patterns, tool-call/tool-result turns by role, harness-specific chain-of-thought blocks (`<thinking>…</thinking>` and equivalents), pure code blocks captured as file content rather than prose, and exact-content duplicates against a recent window. Soft-filter matches are dropped silently with no audit row — being wrong is cheap because the dropped turn was low-signal by definition.

**User override is per-path, not per-turn.** Explicit user-authored writes (`mom remember "…"` on the CLI, explicit `create_memory_draft` calls over MCP) bypass both layers entirely — the user's explicitness wins over Drafter's heuristics. The filter pipeline only runs on automatic transcript capture.

The two layers are deliberately asymmetric. Privacy errors are expensive enough to leave a counter trail and to redact rather than drop the surrounding memory; quality errors are cheap and stay invisible. The override exists because the user knows things the heuristics cannot.

## Consequences

- The vault contains substance, not chatter, and contains substance with credentials redacted in place. Recall quality improves because there is less noise to outscore and less risk of surfacing a leaked key.
- `filter_audit` becomes the single place to read "how often did the hard filter fire and on what categories" without ever storing what it caught. The matched secret never lands on disk.
- Soft filtering is opinionated and will occasionally drop something the user wanted. The mitigations are the explicit-write bypass and the user's ability to recapture by restating the point in a follow-up turn. We accept the trade.
- Capture filtering is the only place where MOM exercises content-shaped judgement on incoming turns. Recall, curation, and storage stay deterministic and content-agnostic; recall does not re-run filters at read time.
- Pattern libraries (secrets and noise) are configurable in `config.yaml` with sensible defaults. Per-harness chain-of-thought rules are contributed by the harness adapter, since each harness exposes its inner monologue differently.

## Considered alternatives

- **No capture filtering; rely on the user to redact.** Rejected: secrets routinely appear in conversation without the user noticing in time. A default-on filter is the only reliable place to catch them.
- **Drop the entire memory when a secret is detected.** Rejected: the surrounding prose is usually the valuable part. Redact-and-persist preserves the lesson without keeping the credential.
- **Persist the matched secret in a separate privacy log for audit.** Rejected: defeats the purpose of redaction. A counter on `filter_audit` is enough to answer "is the filter firing?"; preserving the matched value re-introduces the leak it was meant to prevent.
- **LLM-based quality scoring at capture.** Rejected: pulls a model into the deterministic capture path, makes capture non-reproducible, and adds latency and cost to every turn. Heuristics are good enough for the soft layer.
- **Stage every turn and filter at curation time.** Rejected: stages sensitive content on disk before the filter runs, which is exactly what hard filtering is supposed to prevent.
- **A single unified filter with a confidence score.** Rejected: collapses the asymmetry between privacy errors (need an audit counter, redact-and-persist) and quality errors (cheap, drop silently). Two layers express the right thing.
