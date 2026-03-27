# Contributing to game-engine-core

Thanks for contributing! Please read this guide before opening a PR.

---

## Repository

**GitHub:** https://github.com/stevenlundy/game-engine-core

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
- [ ] If changing a client SDK: run the relevant `make check`, `make type-check`, and `make test-all`

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

---

## Releasing a New Version

Releases are fully automated via the [release workflow](.github/workflows/release.yml). A single tag push publishes all four packages simultaneously.

### Prerequisites (one-time setup)

1. **PyPI Trusted Publisher** — configure OIDC at https://pypi.org/manage/account/publishing/
   - Owner: `stevenlundy`, Repo: `game-engine-core`, Workflow: `release.yml`, Environment: `pypi`

2. **npm token** — create an Automation token at https://www.npmjs.com/settings/~/tokens
   - Add as `NPM_TOKEN` secret at https://github.com/stevenlundy/game-engine-core/settings/secrets/actions

3. **GitHub Environments** — create two environments at https://github.com/stevenlundy/game-engine-core/settings/environments
   - `pypi` (used by the Python publish job)
   - `npm` (used by both TS publish jobs)

### Release Steps

```bash
# 1. Ensure main is clean and all tests pass locally
git checkout main
git pull
make test

# 2. Update version numbers in all four packages
#    clients/python/pyproject.toml  → version = "X.Y.Z"
#    clients/ts-node/package.json   → "version": "X.Y.Z"
#    clients/ts-web/package.json    → "version": "X.Y.Z"
#    (Go module versioning is handled entirely by git tags)

# 3. Commit the version bumps
git add clients/python/pyproject.toml clients/ts-node/package.json clients/ts-web/package.json
git commit -m "chore: bump versions to vX.Y.Z"

# 4. Tag and push — this triggers the release workflow
git tag vX.Y.Z
git push origin main --tags
```

The release workflow will:
1. Run the full test suite for all four language targets
2. Publish `game-engine-core` to PyPI
3. Publish `game-engine-core-node` to npm
4. Publish `game-engine-core-web` to npm
5. Create a GitHub Release with auto-generated notes and install instructions

### What gets published per package

| Package | Registry | Version source |
|---|---|---|
| Go module `github.com/stevenlundy/game-engine-core` | GitHub (automatic) | git tag |
| `game-engine-core` | PyPI | `clients/python/pyproject.toml` |
| `game-engine-core-node` | npm | `clients/ts-node/package.json` |
| `game-engine-core-web` | npm | `clients/ts-web/package.json` |

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
