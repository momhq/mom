# Contributing to Leo

## Prerequisites

- Go 1.22+
- make

## Setup

```bash
git clone https://github.com/vmarinogg/leo-core.git
cd leo-core/cli
make build
make test
```

## Project structure

```
cli/
├── cmd/leo/main.go              # entrypoint
├── internal/
│   ├── cmd/                     # cobra commands (init, upgrade, CRUD, ops, export)
│   ├── adapters/runtime/        # RuntimeAdapter interface + impls (claude, codex, cline, …)
│   ├── adapters/storage/        # StorageAdapter interface + impls (JSON)
│   ├── config/                  # .leo/config.yaml handling
│   └── kb/                      # KB document types and validation
├── Makefile
├── go.mod
└── go.sum

.leo/                            # Leo's own config + KB (dogfooding)
├── config.yaml                  # user preferences
├── identity.json                # project identity
├── kb/docs/                     # knowledge documents (JSON)
├── kb/constraints/              # always-active guardrails
├── kb/skills/                   # composable procedures
├── kb/schema.json               # document schema
└── kb/index.json                # tag-based index
```

See [.github/repo-surface.md](.github/repo-surface.md) for the full one-line
justification of every tracked top-level item and the rules for adding new ones.

### Future package naming (v0.9+)

When new internal components land (Watchman, Drafter, Gardener, etc.), they are
created directly with canonical names — no post-hoc rename. The locked mapping:

| Concept | Go package path |
|---|---|
| Capture trigger layer | `cli/internal/watchman/` |
| Memory draft normalizer | `cli/internal/drafter/` |
| Memory schema validator | `cli/internal/validator/` |
| Memory indexer / search | `cli/internal/librarian/` |
| Tag graph | `cli/internal/tagger/` |
| Lifecycle + dedup + stale + conflict (merged) | `cli/internal/gardener/` |
| RBAC + ABAC | `cli/internal/clearance/` |
| Local telemetry emitter | `cli/internal/transponder/` |

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

## PR process

1. Fork the repo
2. Create a feature branch from `main`
3. Implement with tests (TDD)
4. Run `make test` and `make lint`
5. Submit a PR linking the related issue


## License

By contributing, you agree that your contributions will be licensed under the [Apache 2.0 License](LICENSE).
