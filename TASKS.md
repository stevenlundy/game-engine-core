# game-engine-core — Implementation Tasks

> This file tracks all work needed to build the `game-engine-core` chassis.
> Phases are ordered by dependency: later phases build on earlier ones.
> Each checkbox is a concrete, atomic unit of work.

---

## Phase 1 — Repo Scaffolding & Go Module Setup

**Goal:** A clean, compilable repository skeleton that every later phase can build into.

### 1.1 Module & Toolchain
- [x] Run `go mod init github.com/game-engine/game-engine-core` and commit `go.mod`
- [x] Pin Go toolchain version in `go.mod` (`go 1.22` or later) and add a `.tool-versions` / `.go-version` file
- [x] Create `go.sum` by running an initial `go mod tidy`
- [x] Add a root `Makefile` with targets: `build`, `test`, `lint`, `proto`, `clean`
- [x] Add a `.gitignore` covering Go binaries, `*.pb.go` generated files, `*.glog`, and IDE folders

### 1.2 Directory Skeleton
- [x] Create `api/proto/` directory with a `.gitkeep`
- [x] Create `pkg/engine/` directory with a `.gitkeep`
- [x] Create `pkg/components/cards/` directory with a `.gitkeep`
- [x] Create `pkg/components/grid/` directory with a `.gitkeep`
- [x] Create `pkg/components/timing/` directory with a `.gitkeep`
- [x] Create `pkg/transport/` directory with a `.gitkeep`
- [x] Create `internal/` directory with a `.gitkeep`
- [x] Create `cmd/server/` directory as the entry-point for the gRPC server binary

### 1.3 Core Dependencies
- [x] Add `google.golang.org/grpc` to `go.mod`
- [x] Add `google.golang.org/protobuf` to `go.mod`
- [x] Add `github.com/klauspost/compress` (or standard `compress/gzip`) for GZIP support
- [x] Add a linter config (`.golangci.yml`) with at minimum `errcheck`, `govet`, `staticcheck`, and `gofmt` enabled
- [x] Verify `go build ./...` passes on the empty skeleton

### 1.4 CI Skeleton
- [x] Add a GitHub Actions workflow (`.github/workflows/ci.yml`) that runs `go vet ./...` and `go test ./...` on every push
- [x] Add a `proto` CI step that regenerates `*.pb.go` and fails if the diff is non-empty (ensures proto files stay in sync)

---

## Phase 2 — Protobuf / gRPC API Definitions

**Goal:** Define the complete wire contract so all other packages can depend on stable generated types.

### 2.1 Shared Message Types (`api/proto/common.proto`)
- [x] Define `JSON` as a `bytes` or `google.protobuf.Struct` wrapper message
- [x] Define `State` message (opaque `bytes payload` + `string game_id` + `int64 step_index`)
- [x] Define `Action` message (`string actor_id`, `bytes payload`, `int64 timestamp_ms`)
- [x] Define `StateUpdate` message (`State state`, `float64 reward_delta`, `bool is_terminal`, `string actor_id`)
- [x] Define `SessionMetadata` message (`string session_id`, `string ruleset_version`, `repeated string player_ids`, `int64 start_time_unix`)
- [x] Add proto package declaration and Go package option to `common.proto`

### 2.2 Matchmaking Service (`api/proto/matchmaking.proto`)
- [x] Define `JoinRequest` message (`string player_id`, `string game_type`, `bytes config`)
- [x] Define `JoinResponse` message (`string session_id`, `string status`, `repeated string player_ids`)
- [x] Define `LobbyStatusUpdate` message (`string session_id`, `repeated string ready_players`, `bool game_starting`)
- [x] Define `Matchmaking` service with `JoinLobby(JoinRequest) returns (stream LobbyStatusUpdate)` RPC
- [x] Define `Matchmaking` service with `CancelJoin(JoinRequest) returns (JoinResponse)` RPC

### 2.3 GameSession Service (`api/proto/gamesession.proto`)
- [x] Define `GameSession` service with `Play(stream Action) returns (stream StateUpdate)` bidirectional streaming RPC
- [x] Define `StartSessionRequest` message (`string session_id`, `string player_id`, `bytes initial_config`)
- [x] Define `EndSessionRequest` / `EndSessionResponse` messages (`string session_id`, `string reason`)
- [x] Add `GetReplay(GetReplayRequest) returns (stream ReplayEntry)` RPC for serving `.glog` data over gRPC
- [x] Define `ReplayEntry` message mirroring the `.glog` schema (`int32 step_index`, `string actor_id`, `bytes action_taken`, `bytes state_snapshot`, `float64 reward_delta`, `bool is_terminal`)

### 2.4 Code Generation
- [x] Install `protoc`, `protoc-gen-go`, and `protoc-gen-go-grpc` and document versions in `README.md`
- [x] Add a `make proto` target that runs `protoc` over all `.proto` files and outputs to `api/proto/gen/`
- [x] Commit the generated `*.pb.go` and `*_grpc.pb.go` files (or document the generation step clearly)
- [x] Write a smoke-test that imports the generated package and instantiates one message of each type to confirm generation is correct

---

## Phase 3 — The `GameLogic` Interface

**Goal:** Define the single Go interface every game implementation must satisfy, with supporting types.

### 3.1 Core Types (`pkg/engine/types.go`)
- [x] Define `type JSON = json.RawMessage` type alias (or thin wrapper) for config/state payloads
- [x] Define `type State struct` with fields: `GameID string`, `StepIndex int64`, `Payload json.RawMessage`
- [x] Define `type Action struct` with fields: `ActorID string`, `Payload json.RawMessage`, `TimestampMs int64`
- [x] Define `type TerminalResult struct` with fields: `IsOver bool`, `WinnerID string`

### 3.2 Interface Definition (`pkg/engine/game_logic.go`)
- [x] Declare the `GameLogic` interface with `GetInitialState(config JSON) (State, error)`
- [x] Add `ValidateAction(state State, action Action) error` to `GameLogic`
- [x] Add `ApplyAction(state State, action Action) (newState State, reward float64, err error)` to `GameLogic`
- [x] Add `IsTerminal(state State) (TerminalResult, error)` to `GameLogic`
- [x] Add `GetRichState(state State) (interface{}, error)` to `GameLogic`
- [x] Add `GetTensorState(state State) ([]float32, error)` to `GameLogic`
- [x] Write GoDoc comments on every method explaining contract, expected errors, and nil safety
- [x] Add a compile-time interface guard pattern (e.g., `var _ GameLogic = (*noopGame)(nil)`) in a `_test.go` file

### 3.3 No-Op / Stub Implementation (`pkg/engine/noop_game.go`)
- [x] Implement `noopGame` struct that satisfies `GameLogic` with minimal valid behaviour (for testing the runner in isolation)
- [x] Write unit tests confirming `noopGame` compiles and each method returns its zero value without panicking

---

## Phase 4 — Component Library

**Goal:** Reusable, well-tested building blocks for game developers.

### 4.1 Cards (`pkg/components/cards/`)

#### Deck
- [x] Define `Card` struct (`Suit string`, `Rank string`, `ID string`, `Meta json.RawMessage`)
- [x] Define `Deck` struct wrapping `[]Card` with a `NewDeck(cards []Card) *Deck` constructor
- [x] Implement `Deck.Size() int`
- [x] Implement `Deck.IsEmpty() bool`
- [x] Implement `Deck.Reset()` to restore the deck to its original ordered state

#### Shuffle
- [x] Implement `Deck.Shuffle(rng *rand.Rand)` using Fisher-Yates in-place algorithm
- [x] Accept an explicit `*rand.Rand` (not the global source) so results are deterministic given a seed
- [x] Write a statistical test confirming uniform distribution of shuffle positions over 10,000 runs

#### Deal
- [x] Implement `Deck.Deal(n int) ([]Card, error)` that removes and returns the top `n` cards
- [x] Return a typed error (`ErrInsufficientCards`) when the deck has fewer than `n` cards remaining
- [x] Implement `Deck.DealTo(hands []*Hand, n int) error` to deal `n` cards to each of several hands in round-robin order

#### Hand & Hidden-State Masking
- [x] Define `Hand` struct (`OwnerID string`, `Cards []Card`)
- [x] Implement `Hand.Add(cards ...Card)`
- [x] Implement `Hand.Remove(cardID string) (Card, error)`
- [x] Implement `Hand.MaskFor(viewerID string) Hand` — returns a copy where all cards not owned by `viewerID` have their `Suit` and `Rank` zeroed out and `ID` replaced with `"hidden"`
- [x] Write unit tests for `MaskFor` confirming the owner sees all cards and all other viewers see only `"hidden"` entries

### 4.2 Grid (`pkg/components/grid/`)

#### 2D Grid
- [x] Define `Vec2` struct (`X, Y int`)
- [x] Define `Grid2D[T any]` struct backed by a flat `[]T` slice with `Width, Height int`
- [x] Implement `NewGrid2D[T](width, height int) *Grid2D[T]`
- [x] Implement `Grid2D.Get(pos Vec2) (T, error)` with bounds checking
- [x] Implement `Grid2D.Set(pos Vec2, val T) error` with bounds checking
- [x] Implement `Grid2D.InBounds(pos Vec2) bool`
- [x] Implement `Grid2D.Neighbors4(pos Vec2) []Vec2` (cardinal directions only)
- [x] Implement `Grid2D.Neighbors8(pos Vec2) []Vec2` (cardinal + diagonal)

#### 3D Grid
- [x] Define `Vec3` struct (`X, Y, Z int`)
- [x] Define `Grid3D[T any]` struct backed by a flat `[]T` slice with `Width, Height, Depth int`
- [x] Implement `NewGrid3D[T](width, height, depth int) *Grid3D[T]`
- [x] Implement `Grid3D.Get(pos Vec3) (T, error)` and `Grid3D.Set(pos Vec3, val T) error` with bounds checking
- [x] Implement `Grid3D.InBounds(pos Vec3) bool`

#### Distance Math
- [x] Implement `ManhattanDistance2D(a, b Vec2) int`
- [x] Implement `EuclideanDistance2D(a, b Vec2) float64`
- [x] Implement `ManhattanDistance3D(a, b Vec3) int`
- [x] Implement `EuclideanDistance3D(a, b Vec3) float64`
- [x] Implement `ChebyshevDistance2D(a, b Vec2) int` (chessboard distance, useful for grid games)

#### Occupancy Map
- [x] Define `OccupancyMap` as a type alias or thin wrapper over `Grid2D[string]` where `""` means empty
- [x] Implement `OccupancyMap.Occupy(pos Vec2, entityID string) error` (returns error if already occupied)
- [x] Implement `OccupancyMap.Vacate(pos Vec2) error` (returns error if already empty)
- [x] Implement `OccupancyMap.IsOccupied(pos Vec2) bool`
- [x] Implement `OccupancyMap.OccupiedBy(pos Vec2) (string, bool)`
- [x] Implement `OccupancyMap.AllOccupied() []Vec2` returning positions of all non-empty cells
- [x] Write table-driven unit tests covering boundary conditions and double-occupy / double-vacate errors

### 4.3 Timing (`pkg/components/timing/`)
- [x] Define `TurnTimer` struct with configurable `Timeout time.Duration` and a channel-based signal
- [x] Implement `TurnTimer.Start(ctx context.Context)` that begins a countdown
- [x] Implement `TurnTimer.Stop()` that cancels the countdown cleanly
- [x] Implement `TurnTimer.Expired() <-chan struct{}` returning a channel that fires when the deadline passes
- [x] Implement the **50ms AI timeout** as a package-level constant `DefaultAITimeout = 50 * time.Millisecond`
- [x] Implement `TurnTimer.ElapsedMs() int64` for telemetry logging
- [x] Write a unit test that starts a timer with a 10ms timeout and confirms the `Expired()` channel fires within ±5ms
- [x] Write a unit test that starts and then immediately stops a timer and confirms the `Expired()` channel never fires

---

## Phase 5 — Game Runner & Session Management

**Goal:** The core execution loop that drives `GameLogic`, enforces timeouts, and orchestrates Live vs. Headless modes.

### 5.1 Session Model (`pkg/engine/session.go`)
- [x] Define `SessionConfig` struct (`SessionID string`, `GameType string`, `PlayerIDs []string`, `InitialConfig JSON`, `Mode RunMode`, `AITimeout time.Duration`, `ReplayPath string`)
- [x] Define `RunMode` type with constants `RunModeLive` and `RunModeHeadless`
- [x] Define `Session` struct holding `Config SessionConfig`, `State State`, `Logic GameLogic`, `Log *ReplayLog`, and internal step counter
- [x] Implement `NewSession(cfg SessionConfig, logic GameLogic) (*Session, error)`

### 5.2 Action Dispatcher (`pkg/engine/dispatcher.go`)
- [x] Define `PlayerAdapter` interface with `RequestAction(ctx context.Context, update StateUpdate) (Action, error)` — the abstraction over gRPC client, in-process AI, or random fallback
- [x] Implement `RandomFallbackAdapter` that returns a randomly-chosen valid action (used when the AI timer expires)
- [x] Implement `TimeoutAdapter` wrapper that wraps any `PlayerAdapter`, starts a `TurnTimer`, and calls the fallback if the inner adapter does not respond within `AITimeout`

### 5.3 Game Loop (`pkg/engine/runner.go`)
- [x] Implement `Runner.Run(ctx context.Context, session *Session, players map[string]PlayerAdapter) error`
- [x] In the loop: call `Logic.IsTerminal` → if not terminal, determine active player, send `StateUpdate` via `PlayerAdapter`, receive `Action`
- [x] Call `Logic.ValidateAction`; on error, either reject and re-prompt (Live) or apply fallback (Headless)
- [x] Call `Logic.ApplyAction` and update `session.State`
- [x] Write each transition to `session.Log` (see Phase 6)
- [x] Increment step counter and loop
- [x] On terminal state: write final log entry with `is_terminal: true`, flush and close the `ReplayLog`
- [x] Emit a structured `slog` / `log/slog` log line for each step in Live mode; suppress in Headless mode

### 5.4 Live Mode Specifics
- [x] Implement configurable per-human move timeout (default 30s) separately from the 50ms AI timeout
- [x] Implement graceful disconnect handling: if a gRPC stream drops mid-game, mark that player's actions as forfeit and continue
- [x] Emit `StateUpdate` to all connected spectator streams after each action (broadcast)

### 5.5 Headless Mode Specifics
- [x] Suppress all `slog` output in Headless mode (use a `DiscardHandler`)
- [x] Ensure no blocking I/O occurs during the game loop (all log writes go through the buffer — see Phase 6)
- [x] Add a `BatchRunner` that accepts a slice of `SessionConfig` and runs them concurrently using a worker pool with configurable parallelism
- [x] Implement `BatchRunner.RunAll(ctx context.Context, configs []SessionConfig) ([]BatchResult, error)` returning per-session outcomes
- [x] Benchmark `BatchRunner` with the `noopGame` and document achievable games/second in `README.md`

---

## Phase 6 — Replay Log System

**Goal:** A standardized, efficient `.glog` file format for every game session.

### 6.1 Schema Types (`pkg/engine/replay_types.go`)
- [x] Define `SessionMetadataEntry` struct (`SessionID`, `RulesetVersion`, `PlayerIDs []string`, `StartTimeUnix int64`, `Mode string`) with `json` struct tags
- [x] Define `ReplayEntry` struct with fields: `StepIndex int`, `ActorID string`, `ActionTaken json.RawMessage`, `StateSnapshot json.RawMessage`, `RewardDelta float64`, `IsTerminal bool` — all with `json` struct tags matching the PRD schema
- [x] Define a `ReplayRecord` union type (or tagged struct with `Type string` field) to encode both metadata and step entries in a single JSON-L stream
- [x] Implement `ReplayRecord.MarshalJSON()` and `ReplayRecord.UnmarshalJSON()` for the union type

### 6.2 Writer (`pkg/engine/replay_writer.go`)
- [x] Define `ReplayLog` struct holding an `io.Writer`, a `bufio.Writer` for buffering, an optional `gzip.Writer`, and a `sync.Mutex`
- [x] Implement `NewReplayLog(path string, mode RunMode) (*ReplayLog, error)` — opens the file, wraps in `bufio.Writer` (64 KB buffer), and wraps in `gzip.Writer` when `mode == RunModeHeadless`
- [x] Implement `ReplayLog.WriteMetadata(meta SessionMetadataEntry) error` — writes the header record as the first JSON-L line
- [x] Implement `ReplayLog.WriteEntry(entry ReplayEntry) error` — serializes to JSON and writes a `\n`-terminated line; must be goroutine-safe via the mutex
- [x] Implement `ReplayLog.Flush() error` — flushes the `bufio.Writer` (and `gzip.Writer` if active)
- [x] Implement `ReplayLog.Close() error` — flushes, closes `gzip.Writer` if active, then closes the underlying file

### 6.3 Reader / Parser (`pkg/engine/replay_reader.go`)
- [x] Implement `OpenReplayLog(path string) (*ReplayReader, error)` — auto-detects GZIP magic bytes and transparently wraps with `gzip.Reader`
- [x] Implement `ReplayReader.ReadMetadata() (SessionMetadataEntry, error)` — reads and parses the first line
- [x] Implement `ReplayReader.Next() (ReplayEntry, error)` — advances one line, returns `io.EOF` when exhausted
- [x] Implement `ReplayReader.Close() error`
- [x] Write an integration test: write 1,000 entries with `ReplayLog`, re-read with `ReplayReader`, and assert round-trip fidelity for all fields including `RewardDelta` float precision

### 6.4 GZIP & Performance
- [x] Write a benchmark (`BenchmarkReplayLog`) comparing throughput of plain vs. GZIP writers at 10,000 entries
- [x] Document the compression ratio achieved on a sample `.glog` in `README.md`
- [x] Implement a CLI utility `cmd/glogtool/main.go` with subcommands `inspect` (prints metadata) and `dump` (pretty-prints all entries) for local debugging

---

## Phase 7 — gRPC Transport Layer

**Goal:** Wire the engine to the network using the proto definitions from Phase 2.

### 7.1 Server (`pkg/transport/server.go`)
- [x] Implement `MatchmakingServer` struct satisfying the generated `MatchmakingServer` gRPC interface
- [x] Implement `MatchmakingServer.JoinLobby` — adds the player to an in-memory lobby, streams `LobbyStatusUpdate` until the lobby is full, then triggers session creation
- [x] Implement `MatchmakingServer.CancelJoin` — removes a player from the pending lobby
- [x] Implement `GameSessionServer` struct satisfying the generated `GameSessionServer` gRPC interface
- [x] Implement `GameSessionServer.Play` — receives the bidirectional stream, adapts it to a `PlayerAdapter`, hands off to the `Runner`, and streams `StateUpdate` back to the client
- [x] Implement `GameSessionServer.GetReplay` — opens the `.glog` for a given `session_id` and streams `ReplayEntry` messages
- [x] Add server-side interceptors for: request logging, panic recovery, and context-deadline propagation
- [x] Implement `NewGRPCServer(logic GameLogic, opts ServerOptions) *grpc.Server` as the public constructor

### 7.2 Client Boilerplate (`pkg/transport/client.go`)
- [x] Implement `MatchmakingClient` wrapper with `Join(ctx context.Context, req JoinRequest) (<-chan LobbyStatusUpdate, error)` convenience method
- [x] Implement `GameClient` wrapper with `Play(ctx context.Context) (ActionSender, StateUpdateReceiver, error)` that hides stream management boilerplate
- [x] Define `ActionSender` interface (`Send(Action) error`, `Close() error`) and `StateUpdateReceiver` interface (`Recv() (StateUpdate, error)`)
- [x] Implement a `GRPCPlayerAdapter` that wraps a `GameClient` stream to satisfy the `PlayerAdapter` interface from Phase 5
- [x] Add client-side interceptors for: retry with exponential back-off on transient errors, and deadline injection

### 7.3 Server Entry Point (`cmd/server/main.go`)
- [x] Implement `main.go` that reads config from env vars (`PORT`, `GAME_TYPE`, `HEADLESS`, `LOG_DIR`)
- [x] Registers `MatchmakingServer` and `GameSessionServer` with the gRPC server
- [x] Enables gRPC server reflection (for `grpcurl` / `evans` tooling)
- [x] Handles `SIGTERM` / `SIGINT` for graceful shutdown (drain in-flight sessions, flush all open `ReplayLog` writers)

### 7.4 TLS & Auth (Internal)
- [x] Add `internal/auth/` package with a token-based gRPC interceptor (validates a shared secret via metadata)
- [x] Add `internal/tls/` helper that loads a TLS cert/key pair and returns a `credentials.TransportCredentials`
- [x] Document how to generate a self-signed cert for local development in `README.md`

---

## Phase 8 — Testing & Integration

**Goal:** Confidence that every component works correctly in isolation and together end-to-end.

### 8.1 Unit Tests (per package)
- [x] `pkg/components/cards`: ≥90% coverage — deck, shuffle distribution, deal edge cases, hand masking
- [x] `pkg/components/grid`: ≥90% coverage — bounds checking for 2D and 3D, all distance functions, occupancy map operations
- [x] `pkg/components/timing`: timer fires on time, timer cancelled cleanly, elapsed time accuracy
- [x] `pkg/engine` types & interface: `noopGame` satisfies `GameLogic`, `State`/`Action` marshal round-trip
- [x] `pkg/engine` replay: metadata + entry write/read round-trip (plain and GZIP), `Close` without `Flush` is safe

### 8.2 Runner Integration Tests
- [x] Write a deterministic `countdownGame` (counts down from N to 0) that implements `GameLogic` and terminates after exactly N steps
- [x] Test `Runner.Run` in Live mode with two `RandomFallbackAdapter` players and confirm the game terminates and writes a valid `.glog`
- [x] Test `Runner.Run` in Headless mode and confirm GZIP `.glog` is produced and passes the reader round-trip test
- [x] Test that the 50ms AI timeout fires: use a `PlayerAdapter` that sleeps 200ms and confirm `RandomFallbackAdapter` was used instead
- [x] Test graceful handling of `ValidateAction` returning an error (runner does not crash, fallback is applied)

### 8.3 gRPC End-to-End Tests
- [x] Use `google.golang.org/grpc/test/bufconn` (in-memory listener) to stand up a real gRPC server in tests without a port
- [x] Test `JoinLobby`: two clients join the same game type; confirm both receive a `game_starting: true` update
- [x] Test `Play`: full two-player game of `countdownGame` over gRPC streams; confirm all `StateUpdate` messages arrive and the game terminates correctly
- [x] Test `GetReplay`: after a completed session, stream the replay and confirm entry count matches step count

### 8.4 Benchmark Suite
- [x] `BenchmarkShuffle` — Fisher-Yates on a 52-card deck
- [x] `BenchmarkGrid2DSetGet` — random reads/writes on a 100×100 grid
- [x] `BenchmarkRunnerHeadless` — `countdownGame` with N=100 steps, measure ns/game
- [x] `BenchmarkBatchRunner` — 1,000 concurrent headless games, measure total wall time and games/second
- [x] `BenchmarkReplayLogWrite` — 10,000 entry writes, compare plain vs. GZIP throughput

### 8.5 Documentation & Handoff
- [x] Write `README.md` covering: purpose, quick-start (clone → `make proto` → `make build`), directory map, how to implement a new game, how to run in headless mode, and how to parse a `.glog`
- [x] Add `CONTRIBUTING.md` covering: branch naming, PR checklist (tests, lint, proto regen), and code style rules
- [x] Add Go example file `examples/minimal_game/main.go` showing the smallest possible `GameLogic` implementation wired to the runner
- [x] Confirm `go doc ./...` produces clean output (no unexported symbols leaking into docs)
- [x] Tag `v0.1.0` once all Phase 1–7 checkboxes are complete and CI is green

---

---

## Phase 9 — Python Client SDK (`clients/python/`)

**Goal:** A pip/uv-installable Python package that handles all gRPC plumbing so AI developers only implement `on_state_update()`.

### 9.1 Project Setup
- [x] Create `clients/python/pyproject.toml` using `uv` conventions — name `game-engine-core`, version `0.1.0`, requires Python ≥ 3.11
- [x] Add `grpcio`, `grpcio-tools`, and `protobuf` as dependencies in `pyproject.toml`
- [x] Create `clients/python/game_engine_core/__init__.py` exporting `GameClient` and `Action`
- [x] Add a `Makefile` target `proto-python` that runs `python -m grpc_tools.protoc` to compile `api/proto/*.proto` into `clients/python/game_engine_core/proto/`
- [x] Commit the generated `*_pb2.py` and `*_pb2_grpc.py` stubs (or document the generation step)
- [x] Add `.python-version` pinning Python `3.11` and a `uv.lock` file

### 9.2 `GameClient` Base Class (`game_engine_core/client.py`)
- [x] Define `GameClient` with `__init__(self, server_url: str, player_id: str)` that creates an insecure gRPC channel and a `GameSessionStub`
- [x] Implement `join_lobby(self, game_type: str)` that calls `MatchmakingStub.JoinLobby` and blocks until `game_starting=True`, returning the `session_id`
- [x] Implement `run(self)` that opens the bidirectional `Play` stream, sends the initial join action, then loops: receives `StateUpdate`, calls `on_state_update()`, sends back the returned `Action`
- [x] Define `on_state_update(self, state: StateUpdate) -> Action` as `raise NotImplementedError` — the single method subclasses override
- [x] Implement `close(self)` for clean channel shutdown
- [x] Handle stream errors and disconnection gracefully (log and re-raise)
- [x] Add type hints throughout; export a `StateUpdate` and `Action` dataclass or typed wrapper so callers never import raw proto objects

### 9.3 Helper Utilities
- [x] Add `game_engine_core/actions.py` with factory functions `play_card(rank, suit, declared_suit=None) -> Action` and `draw_card() -> Action`
- [x] Add `game_engine_core/state.py` with a `RichState` dataclass and a `parse_rich_state(state_update) -> RichState` helper that unmarshals the JSON payload

### 9.4 Tests
- [x] Write `tests/test_client.py` using `pytest` — mock the gRPC channel to verify `run()` calls `on_state_update()` for each `StateUpdate` and sends back the returned `Action`
- [x] Write a test that confirms `join_lobby` raises an exception on a non-`game_starting` stream error
- [x] Run `uv run pytest` and confirm all tests pass

### 9.5 Documentation
- [x] Add `clients/python/README.md` covering: install (`uv add git+...` or `pip install .`), quickstart subclassing example, `proto-python` regeneration step
- [x] Add a `clients/python/examples/random_agent.py` showing the minimal subclass

---

## Phase 10 — TypeScript Node Client SDK (`clients/ts-node/`)

**Goal:** An npm-installable TypeScript package for server-side Node.js AI bots using `@grpc/grpc-js`.

### 10.1 Project Setup
- [x] Create `clients/ts-node/package.json` — name `game-engine-core-node`, version `0.1.0`, `main: "dist/index.js"`, `types: "dist/index.d.ts"`
- [x] Add dependencies: `@grpc/grpc-js`, `@grpc/proto-loader`, `google-protobuf`; add dev dependencies: `typescript`, `ts-proto`, `@types/node`, `jest`, `ts-jest`
- [x] Create `clients/ts-node/tsconfig.json` targeting `ES2022`, `moduleResolution: node`, `strict: true`, output to `dist/`
- [x] Add a `Makefile` target `proto-ts-node` that runs `ts-proto`'s `protoc` plugin to compile `api/proto/*.proto` into `clients/ts-node/src/proto/`
- [x] Add `.nvmrc` pinning Node `20 LTS`
- [x] Add `build`, `test`, `proto` npm scripts to `package.json`

### 10.2 `GameClient` Base Class (`src/client.ts`)
- [x] Define `GameClient` class with `constructor(serverUrl: string, playerId: string)` that creates a `@grpc/grpc-js` channel
- [x] Implement `joinLobby(gameType: string): Promise<string>` that calls `Matchmaking.JoinLobby` and resolves with `session_id` when `game_starting=true`
- [x] Implement `run(): Promise<void>` that opens the `GameSession.Play` bidi stream, sends the initial join action, then loops: receives `StateUpdate`, calls `onStateUpdate()`, sends back the returned `Action`
- [x] Define `abstract onStateUpdate(state: StateUpdate): Action | Promise<Action>` — the single method subclasses override
- [x] Implement `close(): void` for clean channel shutdown
- [x] Export `StateUpdate`, `Action`, and `GameClient` from `src/index.ts`

### 10.3 Helper Utilities
- [x] Add `src/actions.ts` with `playCard(rank: string, suit: string, declaredSuit?: string): Action` and `drawCard(): Action` factory functions
- [x] Add `src/state.ts` with a `RichState` interface and `parseRichState(update: StateUpdate): RichState` helper

### 10.4 Tests
- [x] Write `tests/client.test.ts` using `jest` — mock the gRPC channel to verify `run()` calls `onStateUpdate()` for each `StateUpdate` and sends back the returned `Action`
- [x] Confirm `npm test` passes with zero failures

### 10.5 Documentation
- [x] Add `clients/ts-node/README.md` covering: install (`npm install`), quickstart subclassing example, `proto` regeneration step
- [x] Add `clients/ts-node/examples/randomAgent.ts` showing the minimal subclass

---

## Phase 11 — TypeScript Web Client SDK (`clients/ts-web/`)

**Goal:** A browser-compatible TypeScript package using `grpc-web` for in-browser visualizers and web UIs.

### 11.1 Project Setup
- [x] Create `clients/ts-web/package.json` — name `game-engine-core-web`, version `0.1.0`, `main: "dist/index.js"`, `types: "dist/index.d.ts"`
- [x] Add dependencies: `grpc-web`, `google-protobuf`; add dev dependencies: `typescript`, `ts-proto`, `webpack` (or `vite`), `jest`, `ts-jest`
- [x] Create `clients/ts-web/tsconfig.json` targeting `ES2020`, `lib: ["ES2020", "DOM"]`, `strict: true`, output to `dist/`
- [x] Add a `Makefile` target `proto-ts-web` that compiles `api/proto/*.proto` into `clients/ts-web/src/proto/` using `ts-proto` in `grpc-web` mode (unary + server-streaming only — no bidi in browsers)
- [x] Add `.nvmrc` pinning Node `20 LTS`
- [x] Add `build`, `test`, `proto` npm scripts to `package.json`

### 11.2 `GameWebClient` Base Class (`src/client.ts`)
- [x] Define `GameWebClient` class with `constructor(serverUrl: string, playerId: string)` using a `grpc-web` client
- [x] Note: browsers do not support bidi streaming — implement a **polling / server-streaming** model: `watchSession(sessionId: string): void` that subscribes to `StateUpdate` events via `GetReplay` stream or a dedicated watch RPC
- [x] Implement `onStateUpdate(state: StateUpdate): void` as the override point (web clients observe state; actions are submitted via a separate unary `SubmitAction` call if a suitable RPC exists, or noted as out-of-scope)
- [x] Export `GameWebClient`, `StateUpdate` from `src/index.ts`

### 11.3 Helper Utilities
- [x] Add `src/state.ts` with `RichState` interface and `parseRichState` helper (same shape as ts-node)
- [x] Add `src/replay.ts` with `ReplayPlayer` class that reads a `.glog` (fetched as JSON-L text) and emits entries at configurable speed — useful for post-game visualization

### 11.4 Tests
- [x] Write `tests/client.test.ts` using `jest` with `jsdom` environment — mock `grpc-web` transport and confirm `onStateUpdate` is called for each emitted `StateUpdate`
- [x] Write tests for `ReplayPlayer` — confirm entries are emitted in order and `stop()` halts playback
- [x] Confirm `npm test` passes with zero failures

### 11.5 Documentation
- [x] Add `clients/ts-web/README.md` covering: install, browser compatibility notes, gRPC-Web proxy requirement (Envoy), quickstart observer example
- [x] Add `clients/ts-web/examples/replayViewer.ts` showing `ReplayPlayer` usage

---

## Dependency Order (Quick Reference)

```
Phase 1 (Scaffold)
    └── Phase 2 (Proto / gRPC types)
            └── Phase 3 (GameLogic interface)
                    ├── Phase 4 (Components — independent of 2 & 3, can parallelise)
                    ├── Phase 5 (Runner — depends on 3, 4)
                    │       └── Phase 6 (Replay Log — depends on 5 types)
                    └── Phase 7 (Transport — depends on 2, 3, 5, 6)
                            └── Phase 8 (Testing — depends on all above)
                                    └── Phases 9, 10, 11 (Client SDKs — depend on Phase 2 protos)
```

Phases 4, 5, and 6 can be developed in parallel once Phase 3 is complete.
Phases 9, 10, and 11 (Client SDKs) can be developed in parallel with each other once Phase 2 is complete.
