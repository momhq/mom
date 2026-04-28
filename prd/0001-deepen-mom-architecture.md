## Problem Statement

As MOM's codebase grows with support for more AI harnesses (Pi, Claude Code, Codex, Windsurf, etc.), adding new harnesses or features (hooks, extensions, tools) requires repetitive, error-prone edits across multiple files (e.g., switches, lookups, tests). This friction slows development, introduces bugs, and violates the principle of harness-agnostic design — MOM should be extensible without touching core logic for each new harness.

From the user's perspective: "We need to make MOM more harness-agnostic and reduce the coupling so adding a new harness doesn't require touching N files."

## Solution

Refactor MOM's harness integration architecture to use optional, non-exclusive interfaces and centralized knowledge ownership. This makes harness-specific logic opt-in and breakable into separate services if needed, while keeping the monolith simple today. The solution addresses four architectural friction points identified via the `improve-codebase-architecture` skill, documented in ADRs 0001-0005.

From the user's perspective: MOM becomes a flexible, harness-agnostic memory layer where new harnesses "just work" with minimal code changes, and the architecture supports future extraction (e.g., watcher as a microservice) without rewrites.

## User Stories

1. As a MOM developer, I want to add a new harness (e.g., Cursor or Cline) without editing switches in `logbook.go`, `watch.go`, or adapter files, so that I can ship faster and reduce bugs.
2. As a MOM developer, I want harness-specific integration mechanisms (hooks, extensions, transcripts) to be opt-in and non-exclusive, so that a harness can use multiple methods (e.g., hooks + extensions) without interface conflicts.
3. As a MOM developer, I want tool categorization to live with the harness's watcher adapter, so that adding a new tool in Pi only requires editing one file, and the logbook provides a fallback for unknown tools.
4. As a MOM developer, I want the codebase to use "Harness" as the canonical term instead of "Runtime", so that the vocabulary aligns with the AI domain and reduces confusion.
5. As a MOM developer, I want the architecture designed for breakability (modular monolith with extractable seams), so that if a module (e.g., watcher) grows large, it can be extracted into its own service without rewriting adapters.
6. As a MOM user (developer using MOM), I want better harness support (e.g., Native-tier extensibility in Pi, Fluent-tier hooks in Claude), so that MOM integrates more deeply with my preferred AI harness and captures more knowledge automatically.
7. As a MOM maintainer, I want ADRs documenting architectural decisions in `/adr/`, so that future contributors understand the why behind interfaces and tiering without guessing.
8. As a MOM contributor, I want CONTEXT.md to define domain terms (Harness, Adapter, Tier), so that PRs use consistent vocabulary and discussions stay aligned.
9. As a MOM developer, I want compile-time checks for optional interfaces, so that silent contract drift (e.g., renaming `RegisterHooks`) is caught at build time.
10. As a MOM user, I want `mom doctor` to surface harness tiers and capabilities clearly, so that I can understand why certain features work in Pi but not in Windsurf.
11. As a MOM developer, I want the watcher source assembly (harness name, transcript dir, adapter) to be owned by the runtime adapter but joinable at call sites, so that adding a harness requires only one edit, and the logic is extractable to a registry service later.
12. As a MOM user, I want fallback mechanisms (e.g., MCP when transcripts change to SQLite), so that MOM remains robust as harness architectures evolve.
13. As a MOM developer, I want optional interfaces for capabilities like `HookInstaller`, `ExtensionInstaller`, `TranscriptSource`, `ToolCategorizer`, so that the base `Adapter` stays small and focused on universal harness facts.
14. As a MOM maintainer, I want the codebase to migrate from "runtime" to "harness" terminology incrementally, so that docs, types, and directories align with the new canonical term.
15. As a MOM contributor, I want the tier system (Native / Fluent / Functional) to be editorial and independent of interface implementation, so that a Functional harness can still support hooks without becoming Fluent.
16. As a MOM user, I want MOM to be harness-agnostic, meaning it adapts to how each harness works (hooks, extensions, MCP, transcripts), so that I can switch harnesses without losing memory functionality.
17. As a MOM developer, I want the logbook's tool categorization to fall back to a default switch, so that unknown tools are categorized as "other" without breaking.
18. As a MOM contributor, I want ADRs to include consequences and alternatives, so that decisions are reversible and context is preserved.
19. As a MOM developer, I want the architecture to favor event-driven seams (input → process → output), so that modules can be replaced or extracted easily.
20. As a MOM maintainer, I want all ADRs numbered sequentially in `/adr/`, so that the decision history is linear and searchable.

## Implementation Decisions

- Adopt "Harness" as canonical term (ADR 0001); migrate codebase incrementally (e.g., `cli/internal/adapters/runtime/` → `harness/`, types renamed).
- Base `Adapter` interface shrinks to universal harness facts (Name, Tier, DetectHarness, GenerateContextFile, RegisterMCP, GeneratedFiles/Dirs, GitIgnorePaths, Watermark, Capabilities); integration mechanisms become optional interfaces (HookInstaller, ExtensionInstaller, TranscriptSource, ToolCategorizer).
- Tier (Native / Fluent / Functional) declared via `Tier() Tier` method; editorial judgment independent of interface implementation.
- Watcher source assembly owned by runtime adapter via optional `TranscriptSource` interface; joining logic at call sites (name-keyed lookups) for breakability.
- Tool categorization owned by watcher adapter via optional `ToolCategorizer`; logbook provides fallback.
- Design for breakability (ADR 0003): optional interfaces over base methods, explicit joining at call sites, no hard dependencies that block future extraction.
- Compile-time interface checks (e.g., `var _ HookInstaller = (*PiAdapter)(nil)`) to prevent silent contract drift.
- ADRs in `/adr/` at repo root, minimal template (title + 1-3 sentences + consequences + alternatives).
- CONTEXT.md created lazily with domain terms; updated as terms resolve.

## Testing Decisions

A good test focuses on external behavior, not implementation details — e.g., "does the adapter register hooks when implemented?" not "does this internal method return true?"

Modules to test:
- New optional interfaces: Test that adapters implementing them (e.g., Pi for ExtensionInstaller) behave correctly, and adapters not implementing them don't break (type assertions return false).
- Tier declaration: Test `adapter.Tier()` returns expected values (Pi = Native, Claude = Fluent, Windsurf = Functional).
- Watcher source assembly: Test that call sites (watch.go, watch_helpers.go) correctly assemble sources from adapters with/without TranscriptSource.
- Tool categorization: Test watcher adapters with ToolCategorizer override logbook fallback; test unknown tools fall back to "other".
- Breakability: No specific tests, but ensure no hard imports that would block extraction (e.g., runtime package doesn't import watcher types directly).
- Harness rename: Tests updated to use new terminology, but behavior unchanged.

Prior art: Existing tests like `capabilities_test.go::TestCategorizeTool` (update to new interface); `adapter_pi_test.go` for extension registration.

## Out of Scope

- Implementation of fallback mechanisms (e.g., MCP when transcripts change to SQLite) — acknowledged as future need but not built now.
- Full codebase migration to "Harness" (mechanical PR separate from this).
- New harness additions (e.g., Cursor) — this is the enabling architecture.
- User-facing features beyond better integration (e.g., new memory types).
- Extraction of modules into services (design for it, but don't do it).
- Changes to MRP events or memory schemas.

## Further Notes

This PRD synthesizes the conversation's ADRs into an implementable plan. The goal is harness-agnostic extensibility without over-engineering. If modules don't match expectations, adjust before slicing into issues. For testing, focus on adapters with optional interfaces; no need for full integration tests yet. Reference ADRs 0001-0005 for detailed rationale.