# game-engine-core

A zero-knowledge game hosting chassis in Go. The server manages sessions,
players, and replay logs without being recompiled for every new game type.

---

## Quick-Start

```bash
git clone https://github.com/game-engine/game-engine-core.git
cd game-engine-core
make proto   # regenerate *.pb.go from .proto files
make build   # go build ./...
make test    # go test ./...
```

---

## Directory Map

```
game-engine-core/
├── api/
│   └── proto/             # .proto definitions (common, matchmaking, gamesession)
│       └── gen/           # generated *.pb.go and *_grpc.pb.go files
├── cmd/
│   └── server/            # gRPC server entry point (Phase 7)
├── pkg/
│   ├── engine/            # Runner, session model, replay log
│   ├── components/
│   │   ├── cards/         # Deck, shuffle, deal, hand masking
│   │   ├── grid/          # 2D/3D grids, distance math, occupancy maps
│   │   └── timing/        # TurnTimer, AI timeout enforcement
│   └── transport/         # gRPC server & client boilerplate
├── internal/              # Non-exported utilities (auth, TLS)
├── tools/                 # Build-time tool dependencies
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## Protobuf Code Generation

### Tool Versions

| Tool               | Version   |
|--------------------|-----------|
| `protoc`           | 34.1      |
| `protoc-gen-go`    | v1.36.11  |
| `protoc-gen-go-grpc` | v1.6.1  |

### Installing the Plugins

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

Make sure `$(go env GOPATH)/bin` (typically `~/go/bin`) is on your `PATH`.

### Running Generation

```bash
make proto
```

This runs:

```bash
protoc \
  --proto_path=api/proto \
  --go_out=api/proto/gen --go_opt=paths=source_relative \
  --go-grpc_out=api/proto/gen --go-grpc_opt=paths=source_relative \
  api/proto/common.proto \
  api/proto/matchmaking.proto \
  api/proto/gamesession.proto
```

The generated files (`*.pb.go`, `*_grpc.pb.go`) are placed in `api/proto/gen/`.
They are excluded from version control via `.gitignore`; run `make proto` after
cloning to regenerate them.

---

## Implementing a New Game

> Full example: `examples/minimal_game/main.go` (Phase 8)

Implement the `GameLogic` interface from `pkg/engine/`:

```go
type GameLogic interface {
    GetInitialState(config JSON) (State, error)
    ValidateAction(state State, action Action) error
    ApplyAction(state State, action Action) (State, float64, error)
    IsTerminal(state State) (TerminalResult, error)
    GetRichState(state State) (interface{}, error)
    GetTensorState(state State) ([]float32, error)
}
```

---

## Running in Headless Mode

Set the `HEADLESS=true` environment variable (or pass `RunModeHeadless` to
`SessionConfig`) to suppress all slog output and enable GZIP-compressed replay
logs. Use `BatchRunner` to run thousands of concurrent games.

---

## Parsing a `.glog` File

```go
r, err := engine.OpenReplayLog("path/to/session.glog")
if err != nil { /* handle */ }
defer r.Close()

meta, err := r.ReadMetadata()
for {
    entry, err := r.Next()
    if err == io.EOF { break }
    // process entry ...
}
```

The reader auto-detects GZIP-compressed files.
