# game-engine-core-web

Browser client SDK for [game-engine-core](../../README.md).

This package provides **two things**:

1. **`GameWebClient`** — an interactive abstract base class for writing
   browser-based AI agents that play live games against the engine (Phase 11).
2. **`ReplayPlayer` + `fetchGlog`** — a passive observer SDK for replaying
   finished game sessions in the browser (unchanged from earlier phases).

---

## Transport strategy: Option A (one long-lived stream)

The `GameWebClient` opens **a single bidirectional `Play` stream** for the
entire game.  Because the game is turn-based the protocol is naturally
sequential:

```
open stream
  ← recv StateUpdate  (server sends first state)
  if terminal → close, done
  → call onStateUpdate(update)  (subclass decides action)
  → send Action
  ← recv StateUpdate
  ...repeat until terminal...
```

This avoids the overhead of opening a new HTTP request per turn and keeps the
implementation identical to the ts-node client.

### Envoy gRPC-Web proxy

Browsers cannot speak raw HTTP/2 gRPC.  You need an **Envoy** proxy in front
of the game-engine server to translate gRPC-Web (HTTP/1.1) to gRPC (HTTP/2).

Start one with Docker:

```bash
docker run --rm -p 8080:8080 \
  -v "$(pwd)/docker/envoy.yaml:/etc/envoy/envoy.yaml:ro" \
  envoyproxy/envoy:v1.29-latest \
  -c /etc/envoy/envoy.yaml
```

The configuration in [`docker/envoy.yaml`](./docker/envoy.yaml) listens on
port **8080** and proxies to `host.docker.internal:50051` (where the
game-engine gRPC server runs).

---

## Quickstart

### 1 — Install

```bash
npm install game-engine-core-web
```

### 2 — Subclass `GameWebClient`

```typescript
import { GameWebClient } from "game-engine-core-web";
import type { Action, StateUpdate } from "game-engine-core-web";

class MyAgent extends GameWebClient {
  onStateUpdate(update: StateUpdate): Action {
    // Parse the state and decide what to do
    const payload = JSON.parse(Buffer.from(update.state!.payload).toString());
    // ... your logic here ...
    return {
      actorId: "",                              // stamped automatically by run()
      payload: Buffer.from(JSON.stringify({ type: "draw_card" })),
      timestampMs: Date.now(),
    };
  }
}

// Connect through the Envoy proxy on port 8080
const agent = new MyAgent("http://localhost:8080", "player-1");

const sessionId = await agent.joinLobby("crazy-eights");
console.log("session:", sessionId);

await agent.run();
agent.close();
```

See [`examples/randomAgent.ts`](./examples/randomAgent.ts) for a complete
working example.

### 3 — Replay viewer

```typescript
import { fetchGlog, ReplayPlayer } from "game-engine-core-web";

const player = await fetchGlog("https://your-server/sessions/abc123.glog");

player.onEntry = (entry, index) => {
  console.log(`Step ${index}: actor=${entry.actorId} terminal=${entry.isTerminal}`);
};
player.onComplete = () => console.log("Done!");
player.play(500); // 500 ms per step
```

---

## Migration from ts-node

To swap the ts-node `GameClient` for the browser `GameWebClient`:

```diff
-import { GameClient } from "game-engine-core-node";
+import { GameWebClient as GameClient } from "game-engine-core-web";
```

That's it — the public API (`constructor`, `joinLobby`, `run`, `onStateUpdate`,
`close`) is **identical**.

---

## API reference

### `GameWebClient` (abstract)

| Member | Description |
|--------|-------------|
| `constructor(serverUrl, playerId)` | `serverUrl` is the Envoy proxy URL (e.g. `"http://localhost:8080"`). |
| `joinLobby(gameType): Promise<string>` | Join a lobby; resolves with `sessionId` when the game starts. |
| `run(): Promise<void>` | Open one stream, loop until terminal. |
| `abstract onStateUpdate(update): Action \| Promise<Action>` | Override to implement your agent logic. |
| `close(): void` | Shut down gRPC channels. |
| `protected createMatchmakingClient()` | Override for testing or custom transports. |
| `protected createGameSessionClient()` | Override for testing or custom transports. |

### `RichState` / `parseRichState`

```typescript
import { parseRichState } from "game-engine-core-web";

const rich = parseRichState(update);
console.log(rich.gameState);   // parsed JSON payload
console.log(rich.stepIndex);
console.log(rich.isTerminal);
```

---

## Development

```bash
# Install deps
npm install

# Regenerate proto stubs (requires protoc in PATH)
npm run proto

# Run tests
npm test

# Type-check without emitting
npx tsc --noEmit
```
