# 0002 — Adapter integration mechanisms as non-exclusive optional interfaces

MOM Adapters integrate with different Harnesses (Pi, Claude Code, Codex, Windsurf, ...) using fundamentally different mechanisms: Native-tier harnesses (Pi) accept programmable extensions; Fluent and Functional-tier harnesses accept hook configuration files. A single base `Adapter` interface that required `RegisterHooks([]HookDef)` from every adapter resulted in `pi.go::RegisterHooks` ignoring its `[]HookDef` argument and writing a TypeScript extension instead — the type signature lied for one of four implementations, and adding a runtime forced edits in three places (the adapter, a free `XxxHooks()` function, and a name-keyed `HooksForRuntime(name)` switch).

The base `Adapter` interface now declares only what every Harness must support: `Name()`, `Tier()`, `DetectHarness()`, `GenerateContextFile()`, `RegisterMCP()`, `GeneratedFiles()`, `GeneratedDirs()`, `GitIgnorePaths()`, `Watermark()`, and `Capabilities()`. Integration mechanisms move to optional, **non-exclusive** interfaces — `HookInstaller` for harnesses with hook systems, `ExtensionInstaller` for harnesses with extension systems. A future Harness that supports both may implement both. Callers dispatch via Go type assertion, matching the existing `ProjectScoper` precedent.

## Consequences

- The legacy `HooksForRuntime(name string)` lookup table and the per-adapter `DefaultHooks()`/`CodexHooks()`/`WindsurfHooks()`/`PiHooks()` free functions are deleted. Each adapter owns its hooks internally via parameterless `RegisterHooks()`.
- The legacy `SupportsHooks() bool` method is deleted. The type assertion `_, ok := adapter.(HookInstaller)` is the support check.
- Adding a future integration mechanism (supervisor process, native tool injection, etc.) is additive: a new optional interface, no breaking change to the base `Adapter`.
- **Tier and interface implementation are independent.** Tier (Native / Fluent / Functional) is editorial integration-quality judgment; the optional interfaces describe install mechanics. A Functional-tier Adapter may implement `HookInstaller` (Windsurf does today). A Native-tier Adapter may implement both interfaces if its Harness gains a hook system later.
- **Silent contract drift risk.** If `RegisterHooks` or `RegisterExtension` is renamed, the type assertion `a.(HookInstaller)` compiles successfully but stops matching at runtime, silently disabling the install. **Mitigation:** every adapter file ends with a compile-time interface check, e.g. `var _ HookInstaller = (*ClaudeAdapter)(nil)`. Same pattern adopted for `ProjectScoper` in `cli/internal/watcher/adapter_pi.go`.

## Considered alternatives

- **Two new methods on base `Adapter` (Flavor 1).** Each adapter would implement both, returning nil from the unused one. Rejected: forces no-op stubs and doesn't shrink the base interface.
- **Tier-gated dispatch by caller (Flavor 3).** Caller switches on `adapter.Tier()`. Rejected: leaks tier dispatch to every call site, defeating the depth of the `Adapter` abstraction.
- **Keep "hooks" as the umbrella concept, just consolidate the lookup (Option A from grilling).** Rejected: the type signature continues to lie for pi (`[]HookDef` is ignored). The split is editorial but worth encoding — see `CONTEXT.md` for the **Tier** rationale.
