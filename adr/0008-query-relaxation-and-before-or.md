# 0008 — Query relaxation: AND before OR, within scope before escalating

The current `buildFTSQuery` wraps every token in quotes and joins with spaces — implicit OR in FTS5. Any token match scores, which produces high recall but low precision on multi-word queries.

Query relaxation happens in two steps before scope escalation: first attempt a strict AND query (`+token1 +token2 ...`), requiring all tokens to match; if results are below `recallEscalationThreshold`, retry with an OR query in the same scope. Only after both attempts fail to meet the threshold does the engine move to the next scope in the chain. This keeps broader matching local before jumping to a wider scope.

The full per-scope inner loop is: `AND query → OR query → next scope`. This applies identically in both the curated pass and the draft fallback pass (ADR 0006).

## Consequences

- Precision improves for multi-word queries against rich vaults; single-token queries are unaffected (AND and OR are equivalent).
- Two FTS5 queries are issued per scope entry in the worst case. With typical vault sizes this is negligible; SQLite FTS5 query latency is sub-millisecond locally.
- `buildFTSQuery` is split into two functions: `buildFTSQueryAND` and `buildFTSQueryOR`. The escalation loop calls them in order.
- No static concept-expansion maps or synonym tables are introduced. FTS5's porter stemmer handles morphological variation natively.

## Considered alternatives

- **OR-only (current behavior).** Rejected: too permissive for multi-word queries; any single matching token scores, burying strong multi-term matches.
- **AND-only.** Rejected: too strict for short or ambiguous queries against small vaults; produces empty results with no fallback.
- **NEAR() operator.** Rejected: requires tuning a proximity window; adds complexity without clear benefit over AND with OR fallback.
