# game-engine-core Python SDK

The Python client SDK for `game-engine-core`. Handles all gRPC plumbing so your AI only needs to implement one method: `on_state_update()`.

**PyPI:** https://pypi.org/project/game-engine-core/
**GitHub:** https://github.com/stevenlundy/game-engine-core

---

## Install

```bash
# With uv (recommended)
uv add game-engine-core

# With pip
pip install game-engine-core
```

### Pinning to a specific version

```bash
uv add game-engine-core==0.1.0
pip install game-engine-core==0.1.0
```

### Development (editable install from source)

```bash
# From the repo root
uv add --editable ./clients/python
```

---

## Quickstart

```python
from game_engine_core import GameClient, Action
from game_engine_core.actions import play_card, draw_card

class MyBot(GameClient):
    def on_state_update(self, update) -> Action:
        # update.state       — current game state (payload, game_id, step_index)
        # update.reward_delta — score change from last action
        # update.is_terminal  — True when the game is over
        # update.actor_id     — which player this update is for

        # Example: always draw a card
        return draw_card(actor_id=self.player_id)

# Connect, join a lobby, and play
bot = MyBot("localhost:50051", player_id="my-bot")
session_id = bot.join_lobby("crazy-eights")
bot.run()
bot.close()
```

See [`examples/random_agent.py`](examples/random_agent.py) for a complete runnable example.

---

## API Reference

### `GameClient`

| Method | Description |
|---|---|
| `__init__(server_url, player_id)` | Create a client; opens an insecure gRPC channel |
| `join_lobby(game_type) -> str` | Join a lobby and block until `game_starting=True`; returns `session_id` |
| `run()` | Drive the Play stream until the game ends |
| `on_state_update(update) -> Action` | **Override this** — called for each `StateUpdate`; return the next `Action` |
| `close()` | Shut down the gRPC channel cleanly (idempotent) |

### `Action`

```python
@dataclass
class Action:
    actor_id: str
    payload: bytes = b""
    timestamp_ms: int = <now>
```

### Helper factories (`game_engine_core.actions`)

```python
play_card(rank, suit, declared_suit=None, *, actor_id="") -> Action
draw_card(*, actor_id="") -> Action
```

### `StateUpdate`

```python
@dataclass
class StateUpdate:
    state: State          # payload (bytes), game_id (str), step_index (int)
    reward_delta: float
    is_terminal: bool
    actor_id: str
```

### `RichState` (`game_engine_core.state`)

```python
from game_engine_core.state import parse_rich_state

rich = parse_rich_state(update)  # parses update.state.payload as JSON
# rich.top_card, rich.hand, rich.opponent_hand_sizes, rich.current_suit, rich.raw
```

---

## Regenerating Protobuf Stubs

From the repo root:

```bash
make proto-python
```

This runs `grpc_tools.protoc` over `api/proto/*.proto` and writes the generated `*_pb2.py` / `*_pb2_grpc.py` files to `clients/python/game_engine_core/proto/`.

Requirements: `grpcio-tools` must be installed in the venv (`uv sync` handles this automatically).

---

## Running Tests

```bash
cd clients/python

# Unit tests only (no Go server required)
make test

# All tests including integration (requires 'go' on PATH)
make test-all
```

---

## Dev Tooling

Install all dev dependencies:

```bash
make install
# or: uv sync --extra dev
```

| Command | What it does |
|---|---|
| `make lint` | `ruff check` — linting (unused imports, bugbear, pyupgrade, etc.) |
| `make fmt` | `ruff format` — auto-format all files |
| `make fmt-check` | `ruff format --check` — CI formatting check |
| `make type-check` | `mypy game_engine_core/ tests/` — strict type checking |

### Pre-commit hook

Install once after cloning (from the repo root):

```bash
git config core.hooksPath clients/python/.githooks
```

The hook runs `ruff format --check`, `ruff check`, and `mypy` before every commit.

