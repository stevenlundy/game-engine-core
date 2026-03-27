.PHONY: install lint fmt fmt-check check type-check test clean

## install: install all dependencies
install:
	npm install

## lint: run Biome linter
lint:
	npx biome lint src/

## fmt: auto-format with Biome
fmt:
	npx biome format --write src/

## fmt-check: check formatting without modifying (for CI)
fmt-check:
	npx biome format src/

## check: lint + format check together
check:
	npx biome check src/

## type-check: run TypeScript compiler check
type-check:
	npx tsc --noEmit

## test: run all tests
test:
	npm test

## clean: remove build artefacts
clean:
	rm -rf dist/ node_modules/
