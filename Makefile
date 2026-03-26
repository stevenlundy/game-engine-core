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
## TODO(phase2): uncomment once protoc, protoc-gen-go, and protoc-gen-go-grpc are installed.
## proto:
## 	protoc \
## 		--go_out=api/proto/gen --go_opt=paths=source_relative \
## 		--go-grpc_out=api/proto/gen --go-grpc_opt=paths=source_relative \
## 		api/proto/*.proto
proto:
	@echo "TODO: protoc not yet installed — see Phase 2 of TASKS.md"

## clean: remove compiled binaries and test artifacts
clean:
	$(GO) clean ./...
	rm -rf bin/
