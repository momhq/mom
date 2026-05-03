# 0010 — Graph-fluent schema: tags and entities as first-class nodes

The current schema stores tags as a denormalized array on the memory row and has no notion of entities (people, repos, files, projects) as referenceable objects. This makes it impossible to ask "show me everything about X" without a full-text scan, and tags drift (`mcp` vs `MCP` vs `mcp-server`) because they're free-form strings on each row.

The schema becomes graph-fluent: `memory`, `tag`, and `entity` are separate tables, joined through `memory_tag` and `memory_entity` edge tables. Tags and entities have their own identity (a row, a UUID, optional aliases) and can be queried, renamed, or merged without rewriting memories. The recall engine can traverse edges — "memories tagged `recall` that also reference entity `mom-cli`" — using ordinary SQL joins instead of FTS5 substring matches.

The store remains SQLite and FTS5 still indexes memory `content`. The graph layer sits beside FTS5, not behind it; recall can combine a join filter with an FTS5 ranked match in a single query.

## Consequences

- Tag normalization becomes possible: aliases (`mcp` → `MCP`) and merges are row-level operations, not bulk rewrites of every memory that used the old tag.
- Entities give recall a cheap path for "everything about X" queries that would otherwise depend on lucky FTS5 matches. Wrap-up can suggest entity links the same way it suggests tags and types.
- Migration from the current denormalized form is mechanical: split the tag array into edges, dedupe, write the rows. Entity extraction is deferred — entities start empty and accrue as the user (or wrap-up) creates them.
- Three more tables and two join tables. The complexity is real but contained; SQLite handles this scale without strain.
- Recall queries that previously matched on tag substrings now do explicit joins. This is more code at the query layer, but each query expresses its intent.

## Considered alternatives

- **Keep tags as a denormalized array; add entities only.** Rejected: tag drift is already a real problem in the current vault, and the fix is the same shape as the entity solution. Doing both at once is one migration instead of two.
- **JSON column with indexed paths (SQLite `json_extract`).** Rejected: gives up referential integrity and rename support; tag aliasing still requires bulk row rewrites.
- **Separate graph database alongside SQLite.** Rejected: two stores, two consistency models, two backups. SQLite join tables are sufficient at this scale.
- **Embed full entity records on the memory row.** Rejected: duplicates entity data across every memory that mentions it; renames require rewriting every row.
