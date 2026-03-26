# Contributing to game-engine-core

Thanks for contributing! Please read this guide before opening a PR.

---

## Branch Naming

| Type | Pattern | Example |
|---|---|---|
| Feature | `feat/<short-description>` | `feat/grid-pathfinding` |
| Bug fix | `fix/<short-description>` | `fix/replay-gzip-flush` |
| Documentation | `docs/<short-description>` | `docs/headless-mode` |
| Refactor | `refactor/<short-description>` | `refactor/session-config` |
| Chore | `chore/<short-description>` | `chore/bump-grpc` |

Branch off `main`. Keep branches short-lived and focused.

---

## PR Checklist

Before opening a PR, confirm all of the following:

- [ ] `go build ./...` passes with no errors
- [ ] `go test ./...` passes — all tests green, no skips introduced without explanation
- [ ] `go vet ./...` reports no issues
- [ ] If you changed any `.proto` file: run `make proto` and commit the regenerated `*.pb.go` files
- [ ] New public functions and types have GoDoc comments
- [ ] New behaviour is covered by tests (aim for ≥90% coverage on changed packages)
- [ ] No unexported types appear in `go doc` output (`go doc ./...` should be clean)

---

## Code Style

- Follow standard Go idioms — `gofmt`, `goimports`, idiomatic error handling.
- Prefer table-driven tests (`t.Run`) over duplicated test functions.
- Accept explicit `*rand.Rand` instead of the global source in any randomised code.
- Use `log/slog` for structured logging; never `fmt.Println` in library code.
- Keep packages small and focused — a package should have one clear responsibility.
- Export the minimum required surface area; prefer unexported types for internals.

---

## Running Linting

```bash
# Install golangci-lint if needed
brew install golangci-lint

make lint
```

The project enforces `errcheck`, `govet` (with `shadow`), `staticcheck`, `exhaustive`, `wrapcheck`, and `unparam`. Fix all reported issues before opening a PR.

---

## Dev Tooling Setup

After cloning, run this once to enable the pre-commit hook:

```bash
git config core.hooksPath .githooks
```

The hook runs on every `git commit`:
1. **`gofmt`** — fails if any staged `.go` files are unformatted
2. **`go vet`** — fails on suspicious constructs
3. **`golangci-lint`** — runs the full linter suite (skipped gracefully if not installed)

### Race Detector

The CI pipeline runs tests with `-race`. Run it locally before pushing:

```bash
make test-race
```
