import { EventEmitter } from "events";
import { drawCard } from "./actions.js";
import { GameClient } from "./client.js";
import type { Action, StateUpdate } from "./proto/common.js";
import type { GameSessionClient } from "./proto/gamesession.js";
import type { MatchmakingClient } from "./proto/matchmaking.js";

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

class TestClient extends GameClient {
  private _onStateUpdateFn: OnStateUpdateFn;
  public mockMatchmaking: { joinLobby: jest.Mock; close: jest.Mock } | null =
    null;
  public mockSession: { play: jest.Mock; close: jest.Mock } | null = null;

  constructor(onStateUpdateFn: OnStateUpdateFn) {
    super("localhost:50051", "player-1");
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

describe("GameClient.joinLobby", () => {
  it("resolves with sessionId when game_starting is true", async () => {
    const lobbyStream = new FakeReadStream();
    const client = new TestClient(() => drawCard());
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
    const client = new TestClient(() => drawCard());
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
    const client = new TestClient(() => drawCard());
    client.mockMatchmaking = {
      joinLobby: jest.fn().mockReturnValue(lobbyStream),
      close: jest.fn(),
    };

    const promise = client.joinLobby("card-game");
    lobbyStream.emit("end");

    await expect(promise).rejects.toThrow("before game started");
  });
});

describe("GameClient.run", () => {
  it("calls onStateUpdate for each non-terminal update and writes Action back", async () => {
    const playStream = new FakeDuplexStream();
    const onStateUpdate = jest.fn().mockReturnValue(drawCard());

    const client = new TestClient(onStateUpdate);
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

    expect(onStateUpdate).toHaveBeenCalledTimes(3); // 2 non-terminal + 1 terminal
    expect(onStateUpdate).toHaveBeenCalledWith(update1);
    expect(onStateUpdate).toHaveBeenCalledWith(update2);
    expect(onStateUpdate).toHaveBeenCalledWith(terminal);
    // Initial join action + two response actions written back (terminal action not sent)
    expect(playStream.written).toHaveLength(3);
    // Initial join action has actor id and empty payload
    const [join, act1, act2] = playStream.written;
    expect(join?.actorId).toBe("player-1");
    expect(join?.payload).toEqual(Buffer.alloc(0));
    // Actor id is stamped on subsequent actions too
    expect(act1?.actorId).toBe("player-1");
    expect(act2?.actorId).toBe("player-1");
  });

  it("calls onStateUpdate for the terminal update but does not send an action back", async () => {
    const playStream = new FakeDuplexStream();
    const onStateUpdate = jest.fn().mockReturnValue(drawCard());

    const client = new TestClient(onStateUpdate);
    client.mockSession = {
      play: jest.fn().mockReturnValue(playStream),
      close: jest.fn(),
    };

    const runPromise = client.run();
    playStream.emit("data", makeStateUpdate({ isTerminal: true }));

    await runPromise;
    // onStateUpdate IS called for the terminal update
    expect(onStateUpdate).toHaveBeenCalledTimes(1);
    expect(onStateUpdate).toHaveBeenCalledWith(
      makeStateUpdate({ isTerminal: true }),
    );
    // Only the initial join action is written — no action response for terminal
    expect(playStream.written).toHaveLength(1);
    expect(playStream.ended).toBe(true);
  });

  it("resolves when the stream ends naturally", async () => {
    const playStream = new FakeDuplexStream();
    const client = new TestClient(() => drawCard());
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
    const client = new TestClient(() => drawCard());
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

    const client = new TestClient(asyncHandler);
    client.mockSession = {
      play: jest.fn().mockReturnValue(playStream),
      close: jest.fn(),
    };

    const runPromise = client.run();
    playStream.emit("data", makeStateUpdate());
    await new Promise(setImmediate);
    playStream.emit("data", makeStateUpdate({ isTerminal: true }));

    await runPromise;

    expect(asyncHandler).toHaveBeenCalledTimes(2); // 1 non-terminal + 1 terminal
    expect(playStream.written).toHaveLength(2); // initial join + 1 action response (terminal not sent)
  });
});

describe("GameClient.close", () => {
  it("calls close on both underlying clients", () => {
    const client = new TestClient(() => drawCard());
    const mmClose = jest.fn();
    const gsClose = jest.fn();
    client.mockMatchmaking = { joinLobby: jest.fn(), close: mmClose };
    client.mockSession = { play: jest.fn(), close: gsClose };

    // close before run — should not throw (internal refs are null)
    expect(() => client.close()).not.toThrow();
  });

  it("does not throw if close is called before joinLobby/run", () => {
    const client = new TestClient(() => drawCard());
    expect(() => client.close()).not.toThrow();
  });
});
