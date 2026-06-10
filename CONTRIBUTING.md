# Contributing to MOM

## Prerequisites

- Go 1.22+
- make

## Setup

```bash
git clone https://github.com/momhq/mom.git
cd mom
make build
make test
```

## Project structure

The codebase uses a role-based top-level layout (see [ADR 0017](adr/0017-role-based-repo-layout.md)). Each bucket maps 1:1 to the role a package plays in the dataflow — not to whether it reads or writes.

```
cmd/mom/main.go                  # entrypoint
ingress/                         # external input → in-process events
├── cli/                         # cobra subcommands (init, status, project, vault, …)
├── watcher/                     # harness transcript watchers (Claude, Codex, Pi)
└── harness/                     # harness capability registry (detection, context blocks)
events/                          # canonical event pipeline
├── editor/                      # canonicalization gateway (post-ingress, pre-Ledger)
├── registry/                    # schema registry (governance level B, ADR 0019)
└── envelope/                    # canonical event type (Event/EventType)
services/                        # the fold and read-side application code
├── projection/                 # Ledger → markdown vault (Reader → Synthesizer → Writer)
└── lens/                        # local dashboard HTTP server
storage/                         # durable state
├── ledger/                      # append-only event Ledger (the durable backbone)
└── librarian/                   # path resolver (~/.mom, $MOM_VAULT, legacy mom.db detection)
ops/                             # background lifecycle
├── daemon/                      # platform service management
└── diagnose/                    # introspection
shared/                          # cross-cutting utilities
├── config/                      # config.yaml (harness + watcher settings)
├── pathutil/                    # path canonicalization
├── scope/                       # project-local .mom/ discovery (NearestWritable)
├── project/                     # .mom-project.yaml resolution (ADR 0016)
├── ux/                          # TUI/output helpers
└── archtest/                    # architectural invariant tests
Makefile
go.mod
go.sum

.mom/                            # MOM's own memory (dogfooding)
├── config.yaml                  # preferences
├── identity.json                # project identity
├── ledger/                      # append-only captured events
└── vault/                       # projected markdown memory (INDEX.md, topics/, timeline/, …)
```

See [.github/repo-surface.md](.github/repo-surface.md) for the full one-line
justification of every tracked top-level item and the rules for adding new ones.

## Adding a runtime adapter

1. Create a new file in `internal/adapters/runtime/` (e.g. `cursor.go`)
2. Implement the `Adapter` interface defined in `runtime.go`
3. Add tests in a `_test.go` file (TDD: tests first)
4. Register the adapter in the `init` command

Use the `ClaudeAdapter` as reference.

## Commit conventions

We use [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:` new feature
- `fix:` bug fix
- `docs:` documentation
- `test:` tests
- `refactor:` code restructuring

## Code style

Follow patterns from [go-patterns](https://github.com/tmrts/go-patterns). Key principles:

- Strategy pattern for adapters
- Factory functions (`New...`) for constructors
- Interfaces accepted, structs returned
- Table-driven tests

## TDD

All code must follow test-driven development:

1. Write tests first
2. Verify they fail
3. Implement
4. Verify they pass

## Architecture guardrails

The CLI package ships a small set of guardrail tests that enforce
post-alpha main-flow invariants — public CLI surface, harness terminology,
and core-flow architecture. They run as part of the normal Go test suite
and in CI. To run them in isolation:

```bash
go test ./ingress/cli/ -run 'TestGuardrail_'
```

Adding a new exception requires updating the corresponding allowlist in
`ingress/cli/architecture_guardrails_test.go` with explicit rationale.

## PR process

1. Fork the repo
2. Create a feature branch from `main`
3. Implement with tests (TDD)
4. Run `make test` and `make lint`
5. Submit a PR linking the related issue


## License

By contributing, you agree that your contributions will be licensed under the [Apache 2.0 License](LICENSE).
