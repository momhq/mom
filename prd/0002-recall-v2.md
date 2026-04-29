# PRD 0002 — Recall v2: Unified Search, Progressive Escalation, Content-First Ranking

## Problem Statement

MOM's recall returns empty results in most real-world vaults. Three compounding problems cause this:

1. **Draft filtering silences the vault.** `mom_recall` excludes `draft` memories by default. Since no promotion flow exists yet, nearly all memories remain `draft` forever — including those created by cartographer and the watcher pipeline. Recall finds nothing and agents tell users nothing was remembered.

2. **Weak ranking buries relevant memories.** FTS5 scores all columns equally. A query matching a tag scores the same as matching a dense content body. Tags are discovery labels, not relevance signals — equal weighting dilutes the result.

3. **No fallback when scope is sparse.** Recall searches all scopes at once with no awareness of scope proximity or result quality. When the repo scope is empty, the engine gives up instead of looking wider.

Additionally, two tools exist (`mom_recall` and `search_memories`) that are backed by the same engine and return near-identical results — confusing callers about which to use.

## Solution

Recall v2 delivers a single, reliable recall tool backed by a smarter engine:

- One tool (`mom_recall`) replaces both. `search_memories` is removed.
- The engine escalates progressively: AND query in repo → OR query in repo → AND in org → OR in org → AND in user → OR in user. It stops as soon as it has enough results.
- Quality comes before breadth: the engine makes a full curated pass first, then a draft fallback pass if results are still sparse. Agents get reviewed memories when they exist; unreviewed memories fill the gaps.
- FTS5 column weights put content first: `bm25(memories_fts, 0, 2, 1, 10)` — zero on id slug, light on tags, moderate on summary, heavy on content body.
- The escalation chain is an ordered `[]Searcher` slice. Adding a remote vault scope in the future requires implementing one interface and appending to the slice — the engine loop is unchanged.

## User Stories

1. As an agent, I want a single recall tool, so that I don't have to choose between `mom_recall` and `search_memories`.
2. As an agent, I want recall to return results even when the repo scope is sparse, so that org or user memories fill the gap transparently.
3. As an agent, I want recall to prefer curated memories over unreviewed drafts, so that higher-quality context surfaces first.
4. As an agent, I want draft memories to appear as a fallback when curated results are few, so that recall is never silently empty.
5. As an agent, I want AND-first query matching, so that multi-word queries return precise matches before broad ones.
6. As an agent, I want OR fallback when AND yields too few results, so that partial matches surface when exact matches don't exist.
7. As an agent, I want results merged and re-ranked across all queried scopes, so that the best match wins regardless of which scope answered.
8. As a MOM developer, I want the escalation chain to be an ordered `[]Searcher` slice, so that adding a remote vault scope is additive and doesn't touch escalation logic.
9. As a MOM developer, I want a named constant `recallEscalationThreshold` (default 3), so that the trigger is easy to find and promote to config later.
10. As a MOM developer, I want FTS5 column weights to reflect content signal, so that a query matching a content body outscores one matching only a tag.
11. As a MOM developer, I want the `id` column weighted zero in FTS5, so that slug-derived identifiers don't pollute scoring.
12. As a MOM developer, I want `buildFTSQuery` split into AND and OR variants, so that the escalation engine can call each explicitly.
13. As a MOM developer, I want the old in-memory BM25 implementation deleted, so that there is one search engine in the codebase.
14. As a MOM developer, I want cartographer output written as `draft`, not `curated`, so that automated memories don't bypass the quality gate.
15. As a MOM developer, I want wrap-up skill memories written to `memory/` not `logs/`, so that session synthesis is indexed and recallable.
16. As a MOM user, I want recall to find architectural decisions written months ago, so that persistent context isn't buried by recency or weak ranking.
17. As a MOM user, I want recall to work out of the box on a fresh vault, so that the first session produces usable memories without tuning.
18. As a MOM contributor, I want the escalation engine tested with mock Searcher implementations, so that scope chain and quality pass behavior can be verified without a real SQLite index.
19. As a MOM contributor, I want the schema version bumped when column weights change, so that existing indexes rebuild automatically without user intervention.
20. As a MOM contributor, I want `search_memories` removed cleanly, so that the tool surface is minimal and unambiguous.

## Implementation Decisions

### New `recall` package — escalation engine (deep module)

The core of this PRD. Encapsulates all escalation logic behind a simple interface.

- **`Searcher` interface** — one method: search a single scope with a given query string, query type (AND or OR), and inclusion flag for drafts. Returns ranked results. Implemented by a local SQLite scope adapter; future remote vault implements the same interface.
- **`Engine`** — holds an ordered `[]Searcher` chain. `Engine.Search(query, opts)` runs the two-pass escalation loop: curated pass (AND→OR per scope) then draft fallback pass (AND→OR per scope). Stops when `results >= recallEscalationThreshold`. Merges and re-ranks by FTS5 score before returning top-N.
- `recallEscalationThreshold` is a named package-level constant (default 3). Not configurable yet.
- No score floor. Top-N is the only result cap.
- No recency boost. No source quality multipliers. Landmark boost (`+0.3`) is retained.

### SQLite search layer — query building and column weights

- `buildFTSQuery` is split into two functions: `buildFTSQueryAND` (all tokens required, using `+` prefix) and `buildFTSQueryOR` (implicit OR, current behavior).
- `bm25()` call updated to `bm25(memories_fts, 0, 2, 1, 10)`: zero on `id`, 2 on `summary`, 1 on `tags`, 10 on `content_text`.
- Schema version constant is bumped. Version mismatch triggers automatic full reindex on first open — no user action required.
- The `sqliteIndex.Search` method accepts a `includeDrafts bool` parameter to support the two-pass pattern cleanly.

### MCP layer — tool surface cleanup

- `search_memories` tool is removed from the tool catalogue, dispatcher, and all documentation.
- `mom_recall` is wired through `recall.Engine` instead of direct `idx.Search()`.
- `mom_recall` gains an optional `scope` parameter for callers that want to restrict explicitly (does not affect MOM-internal escalation when omitted).

### Dead code removal

- `search/search.go` and `search/bm25.go` are deleted. No callers remain after the MCP wiring change.

### Bug fixes

- `cmd/map.go`: cartographer output `PromotionState` changed from `"curated"` to `"draft"`.
- Wrap-up skill: memory documents written to `memory/` directory, not `logs/`. Promotion state set to `"curated"` (wrap-up is deliberate user synthesis).

### promotion_state semantics (clarified, not changed)

The curated/draft distinction is made explicit across the codebase:
- `"curated"` — deliberate, user-initiated: `create_memory_draft` (MCP tool called by agent at user request), wrap-up skill output.
- `"draft"` — automated, unreviewed: cartographer output, raw transcript pipeline. Surfaces in recall only as fallback.

## Testing Decisions

A good test covers external behavior, not implementation details — "does the engine escalate to org scope when repo returns fewer than threshold results?" not "does `buildFTSQueryAND` produce a specific string?"

**Modules to test:**

- **`recall.Engine`** — primary test target. Mock `Searcher` implementations drive all escalation scenarios: AND success, AND→OR fallback, scope escalation, curated pass exhausted→draft fallback, merge+re-rank correctness, threshold boundary conditions, empty chain.
- **`adapters/storage/sqlite` query building** — test `buildFTSQueryAND` and `buildFTSQueryOR` in isolation; test that column weights are applied (indirectly via score ordering on a small fixture set).

**Not tested:**
- MCP tool wiring (`mcp/tools.go`) — pure plumbing; behavior is covered by engine and storage tests.
- Dead code removal — nothing to test.

**Prior art:** `cli/internal/adapters/storage/sqlite_test.go`, `cli/internal/adapters/storage/multiscope_test.go`.

## Out of Scope

- Remote vault / central memory store — the `[]Searcher` interface is designed for this, but the vault itself is not built here.
- Promotion flow UI — the curated/draft distinction is clarified and enforced, but no user-facing promotion command is added.
- Source quality multipliers (cartographer vs agent-created) — deferred until cartographer output quality is measurable and a promotion flow exists to validate calibration.
- Recency boost — not applied; MOM's value is persistent context, not recent context.
- Score cutoff / minimum relevance floor — superseded by the escalation chain.
- Per-project `recallEscalationThreshold` configuration — the constant is named for easy extraction; config surface is deferred.
- Changes to MRP events or memory schemas beyond `promotion_state` semantics.
- New harness adapters.

## Further Notes

ADRs 0006, 0007, and 0008 document the decisions behind this PRD. Bug fixes (#195, #196) are tracked as separate issues but are part of the v0.14 release scope — they are prerequisites for the quality pass to be meaningful.

The escalation chain will spend most of its time in the draft fallback pass until the promotion flow matures. This is expected and honest — recall works, quality improves incrementally as users promote memories.
