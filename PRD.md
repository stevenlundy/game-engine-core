🛠️ PRODUCT REQUIREMENTS DOCUMENT: `game-engine-core`
-----------------------------------------------------

### 1. High-Level Goals

-   **Zero-Knowledge Hosting:** The server should manage game sessions, players, and logs without needing to be recompiled for every new game.

-   **LLM-Friendly Implementation:** Use idiomatic, simple Go structures that LLMs can accurately extend and debug.

-   **High-Throughput Simulation:** Optimized for "Headless" mode to run thousands of games per second for the Lab.

-   **Standardized "Exhaust":** Every action must produce a structured log entry for both the Visualizer and the RL Agents.

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

The `core` repo will house a `/pkg/components` folder containing pre-built logic.

-   **`cards/`:** Handle Deck shuffling (Fisher-Yates), Dealing, and "Hidden State" (masking cards in a hand from other players).

-   **`grid/`:** 2D/3D coordinate systems, distance math (Manhattan/Euclidean), and occupancy maps.

-   **`timing/`:** Strict timeout enforcement. If an AI doesn't respond in **50ms**, the server auto-folds or picks a random move.

* * * * *
### 4. The Standardized Replay & Telemetry System

The "Replay Log" is the primary data output of the engine. Its purpose is to decouple the **execution** of a game from its **analysis** and **visualization**. By recording every state transition in a standardized format, the engine enables three critical workflows:

1.  **Post-Game Visualization:** A separate renderer can "play back" the file to create visuals for YouTube or UI debugging without needing the game logic.

2.  **Scientific Analysis:** The `game-engine-lab` can parse these logs to calculate win rates, balance metrics, and strategy effectiveness.

3.  **Heuristic Discovery:** LLMs or data models can ingest these logs to find patterns (e.g., "Players who control the center of the board win 70% of the time").

#### **A. The Log Format (.glog)**

To ensure maximum compatibility, the engine will produce a structured JSON-L (JSON Lines) or Protobuf-serialized file containing the following schema:

| **Field** | **Type** | **Justification** |
| --- | --- | --- |
| `session_metadata` | `object` | Records the ruleset version, player IDs, and timestamps to ensure the analysis is context-aware. |
| `step_index` | `int` | An incremental counter (tick) to maintain the chronological order of moves. |
| `actor_id` | `string` | Identifies which player (Human or AI) performed the action. |
| `action_taken` | `object` | The raw input provided by the client (e.g., `card_played: "Ace"`, `coord: [2,2]`). |
| `state_snapshot` | `object` | The "Rich State" of the game immediately *after* the action was applied. |
| `reward_delta` | `float` | The immediate change in score or utility. Crucial for Reinforcement Learning (RL) training. |
| `is_terminal` | `boolean` | Flag indicating if this specific move ended the game. |

#### **B. Storage & Efficiency**

-   **Buffer Management:** The engine will stream log entries to a buffer during gameplay and flush to disk upon game completion to avoid I/O bottlenecks during high-speed simulations.

-   **Compression:** In "Headless" mode (where millions of games are run), the engine will support GZIP compression of the logs to save storage space.

* * * * *

### 5. Repository Structure

Plaintext

```
game-engine-core/
├── api/
│   └── proto/             # .proto definitions for Matchmaking & Gameplay
├── pkg/
│   ├── engine/            # The Runner (Loops, Timeouts, Logging)
│   ├── components/        # Cards, Grids, Dice, Math
│   └── transport/         # gRPC server & client boilerplate
├── internal/              # Non-exported utilities (Auth, Net-scaling)
├── go.mod                 # Core dependencies (grpc, protobuf)
└── README.md

```

* * * * *

### 6. "Headless" vs. "Live" Mode

-   **Live Mode:** The server runs at "human speed" (e.g., waiting 30s for a move) and streams data to the gRPC visualizer.

-   **Headless Mode:** The server runs as fast as the CPU allows. All I/O is suppressed except for the final Replay Log. This is what the **Lab** will trigger for its experiments.