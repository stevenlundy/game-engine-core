# game-engine-core-web

> **Browser-only** observer SDK for [game-engine-core](https://github.com/game-engine/game-engine-core).  
> Watch and replay game sessions — no Node.js, no bidi streaming, no action sending.

---

## Purpose

This package lets a browser application:

1. **Fetch & replay** recorded game sessions (`.glog` newline-delimited JSON files) via `fetchGlog` + `ReplayPlayer`.
2. **Decode state snapshots** from replay entries via `parseStateSnapshot`.
3. (Advanced) **Observe live sessions** by connecting through an Envoy gRPC-Web proxy — see the [gRPC-Web proxy note](#grpc-web-proxy-note) below.

---

## Install

```bash
npm install game-engine-core-web
```

Or, from source:

```bash
cd clients/ts-web
npm install
npm run build
```

---

## Quick Start

### Replay a recorded session

```typescript
import { fetchGlog } from "game-engine-core-web";

const player = await fetchGlog("https://your-server/sessions/abc123.glog");

player.onEntry = (entry, index) => {
  console.log(`Step ${index}`, entry);
};

player.onComplete = () => {
  console.log("Replay complete!");
};

// Play at 500 ms per step (default)
player.play();

// Or at a custom speed
player.play(200); // 200 ms per step

// Stop at any time
player.stop();
```

### Parse a `.glog` file you already have in memory

```typescript
import { ReplayPlayer } from "game-engine-core-web";

const text = await someFile.text(); // e.g. from a <input type="file">
const player = ReplayPlayer.fromJsonLines(text);
player.onEntry = (entry) => console.log(entry);
player.play(500);
```

### Decode a state snapshot payload

```typescript
import { parseStateSnapshot } from "game-engine-core-web";

// entry.stateSnapshot is a Uint8Array of UTF-8 JSON
const state = parseStateSnapshot(entry.stateSnapshot);
console.log(state); // { board: "...", score: 42, ... }
```

---

## API Reference

### `fetchGlog(url: string): Promise<ReplayPlayer>`

Fetches a newline-delimited JSON `.glog` file from `url` using the browser
Fetch API, parses it, and returns a `ReplayPlayer`.

Throws if the HTTP response is not `2xx`.

---

### `class ReplayPlayer`

| Member | Description |
|---|---|
| `constructor(entries: ReplayEntry[])` | Create from a pre-parsed array. |
| `onEntry` | `((entry, index) => void) \| null` — called for each step. |
| `onComplete` | `(() => void) \| null` — called after the last step. |
| `play(speedMs = 500)` | Start (or restart) playback at the given interval. |
| `stop()` | Halt playback immediately. |
| `static fromJsonLines(text)` | Parse a `.glog` text string and return a `ReplayPlayer`. |

---

### `parseStateSnapshot(payload: Uint8Array | string): Record<string, unknown>`

Decode an opaque state-snapshot payload.  Tries UTF-8 → JSON; on failure
returns `{ raw: "<original text>" }`.

---

### `interface StateUpdate`

TypeScript mirror of the `StateUpdate` proto message (see `src/observer.ts`).

---

## gRPC-Web Proxy Note

The browser cannot make raw HTTP/2 gRPC calls.  Real-time observation of live
sessions requires an **Envoy** (or compatible) gRPC-Web proxy:

```
Browser ──(gRPC-Web/HTTP1.1)──▶ Envoy Proxy ──(gRPC/HTTP2)──▶ game-engine server
```

Envoy configuration reference:
<https://www.envoyproxy.io/docs/envoy/latest/configuration/http/http_filters/grpc_web_filter>

For replay-only use (this package's primary purpose) **no proxy is needed** —
only standard HTTP `fetch` is used.

---

## Proto Regeneration

Generated files live in `src/proto/`.  To regenerate after editing `.proto`
sources in `api/proto/`:

```bash
cd clients/ts-web
npm install          # ensures protoc-gen-ts_proto is present
npm run proto
```

This runs:

```bash
protoc \
  --plugin=protoc-gen-ts_proto=./node_modules/.bin/protoc-gen-ts_proto \
  --ts_proto_out=src/proto \
  --ts_proto_opt=env=browser,outputServices=false,esModuleInterop=true \
  --proto_path=../../api/proto \
  ../../api/proto/common.proto \
  ../../api/proto/matchmaking.proto \
  ../../api/proto/gamesession.proto
```

Requires `protoc` ≥ 3 to be on your `PATH`.

---

## Development

```bash
npm run build   # compile TypeScript → dist/
npm test        # run Jest tests (jsdom environment)
npm run clean   # remove dist/
```
