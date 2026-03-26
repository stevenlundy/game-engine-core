.PHONY: build test lint proto clean

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
