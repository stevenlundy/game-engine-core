# game-engine-core

The **Chassis** — a high-performance, language-agnostic foundation for game theory exploration and reinforcement learning. It provides a gRPC server, a standardised replay log (`.glog`), and reusable components (cards, grid, timing) so that any game can be plugged in by implementing a single Go interface.

**GitHub:** https://github.com/stevenlundy/game-engine-core

---

## Using game-engine-core in a Game Repo

Each released version publishes four packages simultaneously, so your game always uses a consistent set of server + client SDKs.

### Go (server / game logic)

```bash
go get github.com/stevenlundy/game-engine-core@v0.1.0
```

```go
import "github.com/stevenlundy/game-engine-core/pkg/engine"
import "github.com/stevenlundy/game-engine-core/pkg/components/cards"
```

### Python (AI clients)

```bash
uv add game-engine-core==0.1.0
# or: pip install game-engine-core==0.1.0
```

```python
from game_engine_core import GameClient, Action
```

### TypeScript — Node.js (AI clients / bots)

```bash
npm install game-engine-core-node@0.1.0
```

```typescript
import { GameClient, Action, StateUpdate } from "game-engine-core-node";
```

### TypeScript — Browser (web UI / interactive player)

```bash
npm install game-engine-core-web@0.1.0
```

```typescript
import { GameWebClient, ReplayPlayer } from "game-engine-core-web";
```

> **Note:** The web client requires an Envoy proxy in front of the Go server for gRPC-Web transport. See [`clients/ts-web/README.md`](clients/ts-web/README.md).

---

## Quick Start

```bash
# 1. Clone
git clone https://github.com/stevenlundy/game-engine-core
cd game-engine-core

# 2. Regenerate protobuf (requires protoc + plugins — see below)
make proto

# 3. Build all binaries
make build

# 4. Run tests
make test
```

### Protobuf prerequisites

| Tool | Install |
|---|---|
| `protoc` 34.x | `brew install protobuf` |
| `protoc-gen-go` v1.36+ | `go install google.golang.org/protobuf/cmd/protoc-gen-go@latest` |
| `protoc-gen-go-grpc` v1.6+ | `go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest` |

Make sure `$(go env GOPATH)/bin` is on your `PATH`.

---

## Directory Map

```
game-engine-core/
├── api/proto/             # .proto source files
│   └── gen/               # Generated *.pb.go / *_grpc.pb.go (committed)
├── cmd/
│   ├── server/            # gRPC server binary entry point
│   └── glogtool/          # CLI for inspecting .glog replay files
├── pkg/
│   ├── engine/            # GameLogic interface, Runner, Session, ReplayLog
│   └── components/
│       ├── cards/         # Deck, shuffle, deal, hidden-state masking
│       ├── grid/          # 2D/3D grids, distance math, occupancy maps
│       └── timing/        # TurnTimer, 50 ms AI timeout constant
├── internal/
│   ├── auth/              # Token-based gRPC interceptor
│   └── tls/               # TLS credential helpers
└── examples/
    └── minimal_game/      # Smallest possible GameLogic wired to the Runner
```

---

## Implementing a New Game

Implement the `engine.GameLogic` interface (six methods) and pass it to the server:

```go
package mygame

import "github.com/game-engine/game-engine-core/pkg/engine"

type MyGame struct{}

func (g *MyGame) GetInitialState(config engine.JSON) (engine.State, error)          { ... }
func (g *MyGame) ValidateAction(s engine.State, a engine.Action) error              { ... }
func (g *MyGame) ApplyAction(s engine.State, a engine.Action) (engine.State, float64, error) { ... }
func (g *MyGame) IsTerminal(s engine.State) (engine.TerminalResult, error)          { ... }
func (g *MyGame) GetRichState(s engine.State) (interface{}, error)                  { ... }
func (g *MyGame) GetTensorState(s engine.State) ([]float32, error)                  { ... }
```

See [`examples/minimal_game/main.go`](examples/minimal_game/main.go) for a runnable example.

---

## Running in Headless Mode

Headless mode suppresses all logging and writes GZIP-compressed `.glog` files — ideal for bulk simulation:

```go
import "github.com/game-engine/game-engine-core/pkg/engine"

factory := func() engine.GameLogic { return &MyGame{} }
br := engine.NewBatchRunner(8 /* parallelism */, factory)

configs := []engine.SessionConfig{
    {SessionID: "run-1", PlayerIDs: []string{"ai-a", "ai-b"}, Mode: engine.RunModeHeadless, LogDir: "./replays"},
    // ...
}

results, err := br.RunAll(context.Background(), configs)
```

---

## Parsing a `.glog` File

```go
import "github.com/game-engine/game-engine-core/pkg/engine"

r, err := engine.OpenReplayLog("session-123.glog") // auto-detects GZIP
if err != nil { log.Fatal(err) }
defer r.Close()

meta, _ := r.ReadMetadata()
fmt.Println("session:", meta.SessionID, "players:", meta.PlayerIDs)

for {
    entry, err := r.Next()
    if err == io.EOF { break }
    fmt.Printf("step %d actor=%s reward=%.2f terminal=%v\n",
        entry.StepIndex, entry.ActorID, entry.RewardDelta, entry.IsTerminal)
}
```

Or use the CLI tool:

```bash
# Inspect metadata
./glogtool inspect session-123.glog

# Pretty-print all entries
./glogtool dump session-123.glog
```

---

## TLS / Auth

### Generating a self-signed certificate for local development

```bash
openssl req -x509 -newkey rsa:4096 -keyout key.pem -out cert.pem \
  -days 365 -nodes -subj '/CN=localhost'
```

Then start the server with:

```bash
TLS_CERT=cert.pem TLS_KEY=key.pem PORT=50051 ./server
```

### Token auth

Set `AUTH_TOKEN` on the server. Clients must include the token in gRPC metadata:

```
authorization: bearer <token>
```

---

## Protobuf Tool Versions

| Tool | Version used |
|---|---|
| `protoc` | 34.1 |
| `protoc-gen-go` | v1.36.11 |
| `protoc-gen-go-grpc` | v1.6.1 |

Regenerate with `make proto`.
