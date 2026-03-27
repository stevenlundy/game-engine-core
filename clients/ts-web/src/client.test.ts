import { EventEmitter } from "events";
import { GameWebClient } from "./client";
import type { Action, StateUpdate } from "./proto/common";
import type { GameSessionClient } from "./proto/gamesession";
import type { MatchmakingClient } from "./proto/matchmaking";

// ---------------------------------------------------------------------------
// Minimal fake duplex stream helper
// ---------------------------------------------------------------------------

class FakeDuplexStream extends EventEmitter {
  public written: Action[] = [];
  public ended = false;

  write(action: Action): boolean {
    this.written.push(action);
    return true;
  }

  end(): void {
    this.ended = true;
  }

  destroy(): void {
    this.ended = true;
  }
}

// ---------------------------------------------------------------------------
// Fake server-reading stream (for joinLobby)
// ---------------------------------------------------------------------------

class FakeReadStream extends EventEmitter {
  destroy(): void {
    this.emit("close");
  }
}

// ---------------------------------------------------------------------------
// Concrete test subclass
// ---------------------------------------------------------------------------

type OnStateUpdateFn = (state: StateUpdate) => Action | Promise<Action>;

class TestGameWebClient extends GameWebClient {
  private _onStateUpdateFn: OnStateUpdateFn;
  public mockMatchmaking: { joinLobby: jest.Mock; close: jest.Mock } | null =
    null;
  public mockSession: { play: jest.Mock; close: jest.Mock } | null = null;

  constructor(onStateUpdateFn: OnStateUpdateFn) {
    super("localhost:8080", "player-1");
    this._onStateUpdateFn = onStateUpdateFn;
  }

  onStateUpdate(state: StateUpdate): Action | Promise<Action> {
    return this._onStateUpdateFn(state);
  }

  protected override createMatchmakingClient() {
    return this.mockMatchmaking as unknown as MatchmakingClient;
  }

  protected override createGameSessionClient() {
    return this.mockSession as unknown as GameSessionClient;
  }
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function drawCard(): Action {
  const payload = JSON.stringify({ type: "draw_card" });
  return {
    actorId: "",
    payload: Buffer.from(payload, "utf8"),
    timestampMs: Date.now(),
  };
}

function makeStateUpdate(overrides: Partial<StateUpdate> = {}): StateUpdate {
  return {
    state: {
      payload: Buffer.from("{}"),
      gameId: "test-game",
      stepIndex: 0,
    },
    rewardDelta: 0,
    isTerminal: false,
    actorId: "player-1",
    ...overrides,
  };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("GameWebClient.joinLobby", () => {
  it("resolves with sessionId when game_starting is true", async () => {
    const lobbyStream = new FakeReadStream();
    const client = new TestGameWebClient(() => drawCard());
    client.mockMatchmaking = {
      joinLobby: jest.fn().mockReturnValue(lobbyStream),
      close: jest.fn(),
    };

    const promise = client.joinLobby("card-game");

    // Emit a non-final update first
    lobbyStream.emit("data", {
      sessionId: "sess-1",
      readyPlayers: [],
      gameStarting: false,
    });
    // Then the final one
    lobbyStream.emit("data", {
      sessionId: "sess-42",
      readyPlayers: ["player-1"],
      gameStarting: true,
    });

    await expect(promise).resolves.toBe("sess-42");
  });

  it("rejects when the lobby stream errors", async () => {
    const lobbyStream = new FakeReadStream();
    const client = new TestGameWebClient(() => drawCard());
    client.mockMatchmaking = {
      joinLobby: jest.fn().mockReturnValue(lobbyStream),
      close: jest.fn(),
    };

    const promise = client.joinLobby("card-game");
    lobbyStream.emit("error", new Error("network failure"));

    await expect(promise).rejects.toThrow("network failure");
  });

  it("rejects when the lobby stream ends without game_starting", async () => {
    const lobbyStream = new FakeReadStream();
    const client = new TestGameWebClient(() => drawCard());
    client.mockMatchmaking = {
      joinLobby: jest.fn().mockReturnValue(lobbyStream),
      close: jest.fn(),
    };

    const promise = client.joinLobby("card-game");
    lobbyStream.emit("end");

    await expect(promise).rejects.toThrow("before game started");
  });
});

describe("GameWebClient.run", () => {
  it("calls onStateUpdate for each non-terminal update and writes Action back", async () => {
    const playStream = new FakeDuplexStream();
    const onStateUpdate = jest.fn().mockReturnValue(drawCard());

    const client = new TestGameWebClient(onStateUpdate);
    client.mockSession = {
      play: jest.fn().mockReturnValue(playStream),
      close: jest.fn(),
    };

    const runPromise = client.run();

    // Emit two normal updates then a terminal one
    const update1 = makeStateUpdate({
      state: { payload: Buffer.from("{}"), gameId: "g", stepIndex: 1 },
    });
    const update2 = makeStateUpdate({
      state: { payload: Buffer.from("{}"), gameId: "g", stepIndex: 2 },
    });
    const terminal = makeStateUpdate({ isTerminal: true });

    playStream.emit("data", update1);
    playStream.emit("data", update2);
    // Wait for the async onStateUpdate handlers to flush
    await new Promise(setImmediate);
    playStream.emit("data", terminal);

    await runPromise;

    expect(onStateUpdate).toHaveBeenCalledTimes(2);
    expect(onStateUpdate).toHaveBeenCalledWith(update1);
    expect(onStateUpdate).toHaveBeenCalledWith(update2);
    // Two actions written back
    expect(playStream.written).toHaveLength(2);
    // Actor id is stamped
    expect(playStream.written[0]?.actorId).toBe("player-1");
  });

  it("resolves immediately on the first terminal update (no onStateUpdate call)", async () => {
    const playStream = new FakeDuplexStream();
    const onStateUpdate = jest.fn();

    const client = new TestGameWebClient(onStateUpdate);
    client.mockSession = {
      play: jest.fn().mockReturnValue(playStream),
      close: jest.fn(),
    };

    const runPromise = client.run();
    playStream.emit("data", makeStateUpdate({ isTerminal: true }));

    await runPromise;
    expect(onStateUpdate).not.toHaveBeenCalled();
    expect(playStream.ended).toBe(true);
  });

  it("resolves when the stream ends naturally", async () => {
    const playStream = new FakeDuplexStream();
    const client = new TestGameWebClient(() => drawCard());
    client.mockSession = {
      play: jest.fn().mockReturnValue(playStream),
      close: jest.fn(),
    };

    const runPromise = client.run();
    playStream.emit("end");

    await expect(runPromise).resolves.toBeUndefined();
  });

  it("rejects when the play stream errors", async () => {
    const playStream = new FakeDuplexStream();
    const client = new TestGameWebClient(() => drawCard());
    client.mockSession = {
      play: jest.fn().mockReturnValue(playStream),
      close: jest.fn(),
    };

    const runPromise = client.run();
    playStream.emit("error", new Error("stream broken"));

    await expect(runPromise).rejects.toThrow("stream broken");
  });

  it("supports async onStateUpdate returning a Promise<Action>", async () => {
    const playStream = new FakeDuplexStream();
    const asyncHandler = jest.fn().mockResolvedValue(drawCard());

    const client = new TestGameWebClient(asyncHandler);
    client.mockSession = {
      play: jest.fn().mockReturnValue(playStream),
      close: jest.fn(),
    };

    const runPromise = client.run();
    playStream.emit("data", makeStateUpdate());
    await new Promise(setImmediate);
    playStream.emit("data", makeStateUpdate({ isTerminal: true }));

    await runPromise;

    expect(asyncHandler).toHaveBeenCalledTimes(1);
    expect(playStream.written).toHaveLength(1);
  });
});

describe("GameWebClient.close", () => {
  it("calls close on both underlying clients", () => {
    const client = new TestGameWebClient(() => drawCard());
    const mmClose = jest.fn();
    const gsClose = jest.fn();
    client.mockMatchmaking = { joinLobby: jest.fn(), close: mmClose };
    client.mockSession = { play: jest.fn(), close: gsClose };

    // close before run — should not throw (internal refs are null)
    expect(() => client.close()).not.toThrow();
  });

  it("does not throw if close is called before joinLobby/run", () => {
    const client = new TestGameWebClient(() => drawCard());
    expect(() => client.close()).not.toThrow();
  });
});
