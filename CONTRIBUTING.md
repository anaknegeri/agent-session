# Contributing to Agent Session

Thanks for your interest in contributing! Agent Session is a local-first, open-source project — contributions of all sizes are welcome.

## Quick start

```bash
git clone https://github.com/anaknegeri/agent-session.git
cd agent-session
make build          # bin/agent-session + bin/agent-session-mcp
make test           # go test ./...
make vet            # go vet ./...
```

Requires **Go 1.25+** and **Git**.

## Project layout

```
cmd/                    # entry points (CLI + MCP server)
cli/                    # cobra commands
internal/
  domain/               # entities, repository interfaces, errors
  application/          # services (use cases), ports (interfaces)
  infrastructure/       # SQLite stores, MCP server, agent adapters, git runner
  config/               # TOML config
  bootstrap/            # wiring entrypoint
  wire/                 # dependency injection (Wire)
pkg/                    # reusable packages (version, update, logger, ids, port)
```

Clean architecture: `Domain → Application → Infrastructure`. Services depend on interfaces (ports), not concrete implementations.

## Adding a new agent adapter

1. Create `internal/infrastructure/agent/<name>/<name>.go` implementing the `agent.Adapter` interface (`Name`, `Detect`, `Configure`, `Install`, `Uninstall`).
2. Add a case in `cli/setup.go` (`install<Name>`) and `cli/init.go` (`installAgentGlobal`).
3. Add detection in `internal/infrastructure/agent/detect.go`.
4. Add a test.

## Commit conventions

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add session branching
fix: context.get returns empty when no checkpoint exists
docs: update install instructions
build: bump Go version in CI
```

## Pull requests

1. Fork and create a branch from `main`.
2. Run `make test && make vet` — must pass.
3. Keep PRs focused — one feature or fix per PR.
4. Write a clear description of what changed and why.

## Releases

Tags follow semver: `v0.1.0`, `v0.1.1`, etc. Pushing a `v*` tag triggers the release pipeline (GitHub Actions builds 6 platforms, creates a GitHub Release, builds a Homebrew bottle, and updates the tap).

```bash
git tag -a v0.X.Y -m "Release v0.X.Y"
git push origin v0.X.Y
```

## License

By contributing, you agree that your contributions are licensed under the MIT License.
