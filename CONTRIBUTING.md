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
│   ├── cmd/                     # cobra commands (init, update, CRUD, ops, export)
│   ├── adapters/runtime/        # RuntimeAdapter interface + impls (claude, cursor, …)
│   ├── adapters/storage/        # StorageAdapter interface + impls (JSON)
│   ├── config/                  # .leo/config.yaml handling
│   ├── kb/                      # KB document types and validation
│   └── profiles/                # specialist profile management
├── Makefile
├── go.mod
└── go.sum

.claude/                         # Leo's own KB (self-hosting / dogfooding)
├── kb/docs/                     # knowledge documents (JSON)
├── kb/schema.json               # document schema
├── kb/index.json                # tag-based index
└── kb/scripts/                  # build-index, validate, check-stale
```

## Adding a runtime adapter

1. Create a new file in `internal/adapters/runtime/` (e.g. `cursor.go`)
2. Implement the `Adapter` interface defined in `runtime.go`
3. Add tests in a `_test.go` file (TDD: tests first)
4. Register the adapter in the `init` command

Use the `ClaudeAdapter` as reference.

## Adding a profile

Create a YAML file in `.leo/profiles/` following this schema:

```yaml
name: Profile Name
description: What this profile does
focus:
  - area of expertise
tone: communication style
default_model: sonnet
context_injection: |
  Instructions injected into the AI context.
```

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
