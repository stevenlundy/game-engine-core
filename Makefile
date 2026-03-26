.PHONY: build test lint proto proto-python clean

GO      ?= go
GOFILES := $(shell find . -name '*.go' -not -path './vendor/*')

## build: compile all packages and binaries
build:
	$(GO) build ./...

## test: run all tests
test:
	$(GO) test ./...

## lint: run golangci-lint (must be installed separately)
lint:
	golangci-lint run ./...

## proto: regenerate protobuf/gRPC Go bindings
## Requires: protoc (libprotoc 34.1), protoc-gen-go (v1.36.11), protoc-gen-go-grpc (v1.6.1)
## Install plugins: go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
##                  go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
proto:
	@mkdir -p api/proto/gen
	PATH="$(PATH):/Users/stevenl/go/bin" protoc \
		--proto_path=api/proto \
		--go_out=api/proto/gen --go_opt=paths=source_relative \
		--go-grpc_out=api/proto/gen --go-grpc_opt=paths=source_relative \
		api/proto/common.proto \
		api/proto/matchmaking.proto \
		api/proto/gamesession.proto

## clean: remove compiled binaries and test artifacts
clean:
	$(GO) clean ./...
	rm -rf bin/

## proto-python: regenerate Python protobuf/gRPC stubs from api/proto/*.proto
## Requires: uv with grpcio-tools installed in clients/python venv
proto-python:
	@mkdir -p clients/python/game_engine_core/proto
	cd clients/python && uv run python -m grpc_tools.protoc \
		--proto_path=../../api/proto \
		--python_out=game_engine_core/proto \
		--grpc_python_out=game_engine_core/proto \
		../../api/proto/common.proto \
		../../api/proto/matchmaking.proto \
		../../api/proto/gamesession.proto
	@touch clients/python/game_engine_core/proto/__init__.py
	@# Fix bare imports in generated _grpc files to use package-relative imports
	@sed -i '' 's/^import common_pb2/from game_engine_core.proto import common_pb2/' \
		clients/python/game_engine_core/proto/gamesession_pb2.py
	@sed -i '' 's/^import common_pb2/from game_engine_core.proto import common_pb2/' \
		clients/python/game_engine_core/proto/gamesession_pb2_grpc.py
	@sed -i '' 's/^import gamesession_pb2/from game_engine_core.proto import gamesession_pb2/' \
		clients/python/game_engine_core/proto/gamesession_pb2_grpc.py
	@sed -i '' 's/^import matchmaking_pb2/from game_engine_core.proto import matchmaking_pb2/' \
		clients/python/game_engine_core/proto/matchmaking_pb2_grpc.py
