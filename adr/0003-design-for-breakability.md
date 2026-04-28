# 0003 — Design for breakability: modular monolith with extractable seams

MOM is built as a modular monolith: single-process simplicity by default, but with seams designed so that any module can be extracted into its own service if growth demands it. The discipline is:

1. **Default to single-process simplicity.** Don't introduce a service boundary unless the cost of *not* having it exceeds the cost of having it.
2. **Treat module boundaries as if they could become service boundaries.** Even when extraction isn't planned, design so that future extraction is mechanical — not a rewrite.
3. **Favor event-driven seams** when they fit the domain. Input → process → output, with the process being replaceable.
4. **Coupling is paid daily; modularity is paid at extraction.** Choose the cheaper bill given expected lifetime.

This is a *decision-making framework*, not a one-off architectural choice. It will recur every time we resolve a "should this be one thing or two?" question.

## Examples

- **ADR 0002** (adapter interfaces): Flavor 2 over Flavor 1 — optional interfaces instead of base methods — because the base stays small and focused, allowing future extraction of "integration mechanisms" as a separate service without breaking existing adapters.
- **ADR 0004** (watcher sources): Optional `TranscriptSource` interface over base method, with joining logic at call site only — because the two registries (Harness adapters, watcher adapters) can be extracted into a unified `HarnessRegistry` service without touching either adapter type.

## Consequences

- Interfaces stay small and focused. New capabilities add optional interfaces rather than bloating the base.
- Call sites handle joining logic explicitly (e.g., type assertions, name-keyed lookups). This keeps coupling explicit and extractable.
- If a module grows large enough to warrant its own service, the extraction is mechanical: lift the joining logic into a new struct/package, add HTTP/gRPC serialization, and wire the event-driven seam.
- The monolith stays simple until complexity demands otherwise.