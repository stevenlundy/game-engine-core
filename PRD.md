🛠️ PRODUCT REQUIREMENTS DOCUMENT: `game-engine-core`
-----------------------------------------------------

### 1. High-Level Goals

-   **Zero-Knowledge Hosting:** The server should manage game sessions, players, and logs without needing to be recompiled for every new game.

-   **LLM-Friendly Implementation:** Use idiomatic, simple Go structures that LLMs can accurately extend and debug.

-   **High-Throughput Simulation:** Optimized for "Headless" mode to run thousands of games per second for the Lab.

-   **Standardized "Exhaust":** Every action must produce a structured log entry for both the Visualizer and the RL Agents.

-   **Multi-Language Client SDKs:** Ship pip-installable (Python) and npm-installable (TypeScript) base clients so AI developers never need to understand gRPC plumbing.

* * * * *

### 2. The Core Interface: `GameLogic`

To build a game, a developer simply imports `core` and implements this Go interface.

Go

```
type GameLogic interface {
    // Initialization
    GetInitialState(config JSON) State

    // Core Loop
    ValidateAction(state State, action Action) error
    ApplyAction(state State, action Action) (newState State, reward float64)

    // Status
    IsTerminal(state State) (isOver bool, winnerID string)

    // ML/AI Hooks
    GetRichState(state State) interface{}    // For humans/heuristics
    GetTensorState(state State) []float32    // For RL Agents
}
```

* * * * *

### 3. System Architecture (The "Chassis")

#### **A. The Coordinator (gRPC Server)**

The server uses **Bidirectional Streaming** to minimize latency.

-   **Service:** `Matchmaking` (Joins players to a lobby).

-   **Service:** `GameSession` (A long-lived stream where the server sends `StateUpdate` and the client responds with `Action`).

#### **B. Modular Components**

The `core` repo houses a `/pkg/components` folder containing pre-built logic.

-   **`cards/`:** Handle Deck shuffling (Fisher-Yates), Dealing, and "Hidden State" (masking cards in a hand from other players).

-   **`grid/`:** 2D/3D coordinate systems, distance math (Manhattan/Euclidean), and occupancy maps.

-   **`timing/`:** Strict timeout enforcement. If an AI doesn't respond in **50ms**, the server auto-folds or picks a random move.

#### **C. Multi-Language Client SDKs (`clients/`)**

`pkg/` is strictly Go server logic. A top-level `clients/` directory houses language-specific SDKs. Each SDK handles the gRPC plumbing so that AI developers only implement `on_state_update()` / `onStateUpdate()`.

| Directory | Language | Package Manager | Target Audience |
|---|---|---|---|
| `clients/python/` | Python 3.11+ | `uv` / PyPI | RL researchers, Lab orchestration |
| `clients/ts-node/` | TypeScript (Node) | `npm` | Server-side AI bots, scripted agents |
| `clients/ts-web/` | TypeScript (Browser) | `npm` | In-browser visualizers, web UIs |

Each client folder is a self-contained package with its own dependency manifest (`pyproject.toml` / `package.json`) and its own generated protobuf stubs compiled from `api/proto/`.

#### **D. The Developer Experience**

A game repo developer imports the SDK and overrides a single method:

**Python (`game-crazy-eights`):**
```python
from game_engine_core import GameClient

class MyCrazyEightsAI(GameClient):
    def on_state_update(self, state):
        # Inspect state.rich_state, return an Action
        return self.play_card("8", "S", declared_suit="H")
```

**TypeScript (Node):**
```typescript
import { GameClient } from "game-engine-core";

class MyCrazyEightsAI extends GameClient {
  onStateUpdate(state: StateUpdate): Action {
    return { drawCard: {} };
  }
}
```

* * * * *

### 4. The Standardized Replay & Telemetry System

The "Replay Log" is the primary data output of the engine. Its purpose is to decouple **execution** from **analysis** and **visualization**.

1.  **Post-Game Visualization:** A separate renderer can "play back" the file without needing the game logic.

2.  **Scientific Analysis:** The `game-engine-lab` parses these logs to calculate win rates, balance metrics, and strategy effectiveness.

3.  **Heuristic Discovery:** LLMs or data models can ingest these logs to find patterns.

#### **A. The Log Format (.glog)**

| **Field** | **Type** | **Justification** |
| --- | --- | --- |
| `session_metadata` | `object` | Records the ruleset version, player IDs, and timestamps. |
| `step_index` | `int` | Chronological order of moves. |
| `actor_id` | `string` | Identifies which player performed the action. |
| `action_taken` | `object` | The raw input provided by the client. |
| `state_snapshot` | `object` | The "Rich State" immediately *after* the action. |
| `reward_delta` | `float` | Immediate change in score. Crucial for RL training. |
| `is_terminal` | `boolean` | Flag indicating if this move ended the game. |

#### **B. Storage & Efficiency**

-   **Buffer Management:** Log entries are streamed to a buffer and flushed on game completion.

-   **Compression:** Headless mode supports GZIP compression.

* * * * *

### 5. Repository Structure

```
game-engine-core/
├── api/
│   └── proto/             # Master .proto definitions (Source of Truth)
│       └── gen/           # Generated Go stubs (committed)
├── clients/               # Language-agnostic SDKs
│   ├── python/            # Python base client (pip/uv installable)
│   │   ├── pyproject.toml
│   │   ├── game_engine_core/
│   │   │   ├── __init__.py
│   │   │   ├── client.py      # GameClient base class
│   │   │   └── proto/         # Generated Python stubs
│   │   └── tests/
│   ├── ts-node/           # TypeScript (Node.js) base client
│   │   ├── package.json
│   │   ├── tsconfig.json
│   │   ├── src/
│   │   │   ├── client.ts      # GameClient base class
│   │   │   └── proto/         # Generated TS stubs
│   │   └── tests/
│   └── ts-web/            # TypeScript (Browser) base client
│       ├── package.json
│       ├── tsconfig.json
│       ├── src/
│       │   ├── client.ts      # WebSocket/gRPC-Web bridge
│       │   └── proto/         # Generated TS stubs (grpc-web)
│       └── tests/
├── pkg/                   # GO-ONLY SERVER LOGIC
│   ├── engine/            # Rule execution & session management
│   ├── components/        # Cards, Grids, Dice, Math
│   └── transport/         # Server-side gRPC & matchmaking
├── internal/              # Non-exported utilities (Auth, TLS)
├── cmd/
│   ├── server/            # gRPC server binary entry point
│   └── glogtool/          # .glog inspection CLI
├── examples/
│   └── minimal_game/      # Smallest possible GameLogic example
├── go.mod
└── README.md
```

* * * * *

### 6. Protobuf Strategy

The `api/proto/` folder is the **single source of truth** for all language SDKs.

-   **Go server:** `make proto` → compiles to `api/proto/gen/` via `protoc-gen-go` + `protoc-gen-go-grpc`.

-   **Python client:** `make proto-python` → compiles to `clients/python/game_engine_core/proto/` via `grpc_tools.protoc`.

-   **TypeScript Node client:** `make proto-ts-node` → compiles to `clients/ts-node/src/proto/` via `ts-proto`.

-   **TypeScript Web client:** `make proto-ts-web` → compiles to `clients/ts-web/src/proto/` via `ts-proto` (gRPC-Web mode).

Recommended: adopt **Buf** (`buf.build`) for cross-language protobuf generation management.

* * * * *

### 7. "Headless" vs. "Live" Mode

-   **Live Mode:** The server runs at "human speed" (e.g., waiting 30s for a move) and streams data to connected clients.

-   **Headless Mode:** The server runs as fast as the CPU allows. All I/O is suppressed except for the final Replay Log. This is what the **Lab** triggers for experiments.
