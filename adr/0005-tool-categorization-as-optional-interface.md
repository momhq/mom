# 0005 — Tool categorization as optional interface on watcher.Adapter

Tool names (e.g., "git", "run_terminal_cmd") come from harness transcripts; categorization buckets them into logbook categories ("git", "search", "mcp", "agentic", "other"). The categorization logic is harness-specific ("what tools does my harness expose") but lived separately: a switch in `cli/internal/logbook/logbook.go::categorizeTool`, a near-duplicate in `cli/internal/adapters/runtime/adapter_pi.go::toolCategory`, and tests in `cli/internal/adapters/runtime/capabilities_test.go::TestCategorizeTool`. Adding a tool in pi required edits in three places.

Tool categorization moves to an optional interface on `watcher.Adapter` (the runtime-time parser that already handles harness-specific transcript parsing). Adapters that expose tools implement `ToolCategorizer`; adapters that don't (transcript-only) simply don't satisfy the interface. The logbook's switch becomes a fallback for unknown tools.

## Consequences

- The duplicate switches in `adapter_pi.go` and `capabilities_test.go` are deleted. Tool additions in pi now require only one edit: implement `CategorizeTool` on the pi watcher adapter.
- The logbook's `categorizeTool` becomes a fallback: if the watcher adapter implements `ToolCategorizer` and returns a non-empty category, use it; else fall back to the existing switch.
- **Fits the breakability principle (ADR 0003).** If watcher logic extracts to its own service, tool categorization goes with it.
- **Symmetric with existing watcher adapter capabilities.** Watcher adapters already have harness-specific parsing logic (`ParseLine`, `ProjectScoper`, `SessionParser`). Categorization joins them.
- Adding a new tool category is harness-local: the watcher adapter defines it.

## Considered alternatives

- **Method on base `watcher.Adapter` (D1).** Rejected: forces every watcher adapter (even transcript-only ones) to implement it, returning empty or "other".
- **ToolCategorizer on `runtime.Adapter` (D2).** Rejected: violates breakability (runtime adapters are install-time; tool knowledge belongs with runtime-time watcher logic).