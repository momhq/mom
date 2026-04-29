# 0006 — Unified recall with progressive scope and quality escalation

`mom_recall` and `search_memories` converged on the same FTS5 engine with identical options. Maintaining two tools created confusion about which to call and doubled the surface area with no benefit. `search_memories` is deleted; `mom_recall` becomes the single recall tool.

Recall now escalates across two axes — scope and quality — in a defined order. The engine iterates an ordered `[]Searcher` chain (repo → org → user → future vault) and stops as soon as it has collected `recallEscalationThreshold` results (named constant, default 3). Escalation is MOM-internal and transparent to the caller. Scope escalates before quality: the engine makes two full passes over the chain — first querying only `curated` memories, then repeating with drafts included — so that a reviewed org memory ranks above an unreviewed local draft.

Results from all queried scopes are merged and re-ranked by FTS5 score before returning top-N. No score floor is applied; top-N is sufficient given the escalation chain already handles sparse vaults. The chain is designed for breakability (ADR 0003): adding a remote vault scope requires implementing one interface and appending it to the slice — the escalation loop is unchanged.

## Consequences

- `search_memories` MCP tool is removed. Callers using it must migrate to `mom_recall`.
- The old in-memory BM25 implementation (`search/search.go`, `search/bm25.go`) is deleted — it is no longer called.
- `recallEscalationThreshold` is a named Go constant (default 3). It is not configurable yet; promotion to config is mechanical when the need arises.
- Draft memories are included only as a fallback. As the promotion flow matures, Pass 1 (curated-only) will satisfy more queries and draft fallback will be rare.
- The `[]Searcher` interface is the seam for vault extraction (ADR 0003): local scopes are `scope.Walk()` entries; a future remote vault implements the same interface and appends to the slice.

## Considered alternatives

- **Keep both tools with diverging responsibilities.** Rejected: they share the same engine and options; divergence would be artificial and require double maintenance.
- **OR-only scope fallback (no quality pass).** Rejected: a curated org memory is more trustworthy than an unreviewed local draft; quality should be exhausted before accepting lower-trust content.
- **Score cutoff instead of threshold count.** Rejected: BM25 scores are not normalized across vault sizes; a count threshold is stable as the vault grows.
