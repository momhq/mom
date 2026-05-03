# 0009 — Storage consolidation: $HOME as canonical SQLite vault

Memories are currently scattered across per-folder `.mom/` directories, with raw transcripts (`raw/*.jsonl`), per-memory JSON files (`memory/*.json`), and SQLite indexes living side by side. This produces three problems: (a) the same memory can exist in multiple project vaults with no single source of truth, (b) raw transcripts duplicate substance already captured in memory documents, and (c) progressive scope escalation has to walk a fragile filesystem chain.

`$HOME/.mom/mom.db` is the single canonical store. All memories live in that database regardless of where they were captured. Per-folder `.mom/` directories are removed; the JSON-file vault and the `raw/` transcript log are removed; SQLite is the only persistence layer. The previous notion of filesystem-partitioned scopes (per-folder vaults walked as a chain) is dropped entirely — there is one vault.

Migration runs through `mom upgrade`. When the installed version is older than 0.30.0 and the user invokes upgrade, the command detects the legacy on-disk vaults, warns that a one-way data migration is required, and refuses to proceed without explicit confirmation. A `--dry-run` flag prints exactly what would be imported (source paths, memory counts, ID remappings under ADR 0013) without touching the database. After confirmation, upgrade imports every legacy vault into `$HOME/.mom/mom.db`, mints fresh UUIDs, and writes the old → new ID mapping to a one-shot log. The legacy directories on disk are left in place; the user deletes them when comfortable. Re-running upgrade after migration is a no-op.

## Consequences

- One database to back up, one database to query. Recall no longer walks a scope chain across the filesystem.
- Raw transcripts are no longer retained. Capture writes directly to memory rows; if a turn is rejected by capture filtering (ADR 0014) it is dropped, not staged.
- Multi-machine setups need an external sync mechanism (e.g. the user's existing dotfile sync, or a future explicit sync command). `$HOME/.mom/mom.db` is a single file, which makes this tractable.
- The upgrade flow is the only path forward across the 0.30.0 boundary. `--dry-run` lets the user inspect the import plan before committing. The migration is non-destructive at the source: legacy directories remain readable until the user removes them.
- Tooling that previously inspected `.mom/` directories on disk is replaced by `mom` CLI commands against the single database.

## Considered alternatives

- **Keep per-folder vaults, add a registry.** Rejected: duplicates the source-of-truth problem with an extra index to keep consistent.
- **Multiple SQLite databases (one per former scope).** Rejected: cross-database queries require attaching files; the escalation logic this ADR is removing would come back as SQL plumbing.
- **Retain `raw/` as an append-only audit log.** Rejected: substance is already in the memory row; the log is write-only and grows without bound. Capture filtering (ADR 0014) handles the cases where raw retention was previously useful.
- **Keep the JSON-file vault alongside SQLite as a human-readable mirror.** Rejected: two writers, two failure modes, and the JSON files are not actually read by humans in practice.
- **A separate `mom migrate` command instead of folding migration into `mom upgrade`.** Rejected: the migration is a version-boundary event, not an ongoing operation. Coupling it to upgrade ensures no user crosses 0.30.0 without seeing the prompt.
