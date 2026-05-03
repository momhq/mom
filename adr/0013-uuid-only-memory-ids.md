# 0013 — UUID-only memory IDs

Memory IDs are currently a mix of slug-style strings derived from content (`recall-v2-design-decisions-...`) and session-derived sequences (`<session-uuid>-NNN`). The mix is historical: early memories were hand-curated and slugged for readability; automatic capture later switched to session-derived IDs. The result is an ID space with no single format, where collisions are possible across sources and where renames mean rewriting every reference.

Every memory ID is a UUIDv4 (or v7 — the choice is implementation-detail and does not affect the contract). IDs are opaque, fixed at creation, never derived from content or session, and never reused. The slug-style and session-derived forms are removed.

The migration to `$HOME/.mom/mom.db` (ADR 0009) mints a fresh UUID for every imported memory. The old → new mapping is written to a one-shot log file alongside the database for anyone who needs it for debugging; the mapping is not part of the schema and is not retained as a field on the memory row.

## Consequences

- IDs are uniformly opaque. Any code that parsed structure out of an ID (extracting a session, slugging from content) is removed.
- Cross-vault references are unambiguous: a UUID minted in one vault cannot collide with one minted in another, which matters when memories move between scopes or are shared.
- Memory IDs are no longer human-readable. This is a real cost — `mem-9fe9768b419f` is harder to recognize at a glance than `recall-v2-design-decisions`. The replacement for human-readable identification is the `summary` field (and tags, and entity edges from ADR 0010), not the ID.
- Pre-alpha clean break: existing IDs are dropped on import, not preserved as `legacy_id`. The user base is small enough that the simplification is worth the loss of historical identifiers; the migration log preserves the mapping for the rare debugging case.

## Considered alternatives

- **Keep existing IDs as-is; mixed-format vault forever.** Rejected: every parser and every UI element has to handle two formats indefinitely. The simplification compounds.
- **Mint new UUIDs but preserve old IDs in a `legacy_id` column.** Rejected: adds a permanent column to serve a transient need. The migration log covers the rare debugging case without polluting the schema.
- **ULIDs or k-sortable IDs instead of UUIDv4.** Not rejected on principle — the contract here is "opaque, unique, fixed." Any UUID/ULID-shaped value satisfies it. Implementation may choose v7/ULID for index locality.
- **Content-hash IDs (CID-style).** Rejected: substance-immutability (ADR 0011) already gives the deduplication benefit; tying ID to content means any allowed metadata mutation that touches generation logic risks ID churn.
