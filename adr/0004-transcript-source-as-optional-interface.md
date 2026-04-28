# 0004 — Transcript source as optional interface on runtime.Adapter

Watcher sources (harness name, transcript dir, watcher.Adapter instance) were reconstructed at every call site via a `switch watchRuntime` (manual path) or `buildWatcherSources` lookup (config path), plus a package-level map `defaultTranscriptDirs`. Adding a Harness required edits in three places.

Transcript source knowledge moves to an optional `TranscriptSource` interface on `runtime.Adapter` (the install-time adapter). Adapters that have transcripts to watch implement it; adapters that don't (e.g., transcript-only) simply don't satisfy the interface. Callers query the adapter for its transcript dir, then join with the watcher adapter registry at call site. This fits the breakability principle: runtime adapters and watcher adapters stay separate, joinable without coupling.

## Consequences

- The legacy `defaultTranscriptDirs` map and the `switch watchRuntime` / `buildWatcherSources` switches are deleted. Call sites become "iterate adapters, ask for TranscriptSource if present, join with watcher registry."
- Adding a Harness → implement `TranscriptSource` on the runtime adapter → both call sites pick it up automatically.
- **Fits the breakability principle (ADR 0003).** If watcher logic extracts to its own service, transcript knowledge stays with runtime adapters; joining moves to the service boundary.
- Semantically honest: harnesses without transcripts (future MCP-only) don't implement the interface.
- **Silent contract drift risk.** If `DefaultTranscriptDir` is renamed, type assertions stop matching. Mitigation: compile-time checks in adapter files.

## Considered alternatives

- **Method on base `Adapter` (B1).** Rejected: forces empty-string returns for transcript-less harnesses; couples transcript knowledge to every adapter.
- **TranscriptSource on `watcher.Adapter` (D3 from grilling).** Rejected: violates breakability; watcher adapters are runtime-time, runtime adapters are install-time.