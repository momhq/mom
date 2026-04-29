# 0007 — FTS5 column weights: content-first ranking

The FTS5 virtual table `memories_fts` has four columns: `id`, `summary`, `tags`, `content_text`. With equal weights, a query matching a tag scores the same as matching a dense content body — but tags are discovery labels, not relevance signals. The `id` column is a slug derived from the summary; matching it adds noise, not signal.

Column weights are set to `bm25(memories_fts, 0, 2, 1, 10)`: zero on `id` (slug, not meaningful for search), light on `tags` (discovery, not relevance), moderate on `summary` (one-line intent), heavy on `content_text` (the full knowledge body). Content has the signal; tags navigate to it.

## Consequences

- Schema version is bumped. Existing indexes are wiped and rebuilt on first open — a full reindex is triggered automatically.
- Source quality multipliers (e.g. cartographer vs agent-created) are **not** applied at this stage. The promotion flow does not yet exist to calibrate per-source trust; adding guessed multipliers would create tech debt. This decision is revisited once cartographer quality is measurable.
- Recency boost is not applied. MOM's value is persistent context — architectural decisions from months ago remain relevant. Recency would bury them.
- Landmark boost (`+0.3` applied in Go post-processing) is retained; landmarks are editorially significant regardless of query.

## Considered alternatives

- **Equal weights (default).** Rejected: tags and slugified IDs dilute content signal.
- **Tags weighted higher than summary.** Rejected: tags are applied by the creator for discovery, not by the query author for relevance; content is the ground truth.
- **Source quality multiplier (cartographer = 0.8×, mcp = 1.0×).** Rejected: no quality data exists to calibrate these values; deferred until promotion flow is in place.
