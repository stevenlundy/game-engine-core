.PHONY: install lint fmt fmt-check type-check test test-all clean

## install: install the package and all dev dependencies
install:
	uv sync --extra dev

## lint: run ruff linter
lint:
	uv run ruff check .

## fmt: auto-format with ruff
fmt:
	uv run ruff format .

## fmt-check: check formatting without modifying files (for CI)
fmt-check:
	uv run ruff format --check .

## type-check: run mypy type checker
type-check:
	uv run mypy game_engine_core/ tests/

## test: run unit tests only (no Go server required)
test:
	uv run pytest tests/test_client.py -v

## test-all: run all tests including integration (requires 'go' on PATH)
test-all:
	uv run pytest tests/ -v

## clean: remove build and cache artefacts
clean:
	rm -rf dist/ .venv/ __pycache__ .mypy_cache .ruff_cache .pytest_cache
	find . -type d -name __pycache__ -exec rm -rf {} + 2>/dev/null || true
