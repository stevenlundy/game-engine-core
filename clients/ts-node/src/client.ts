import * as grpc from "@grpc/grpc-js";
import { MatchmakingClient } from "./proto/matchmaking.js";
import { GameSessionClient } from "./proto/gamesession.js";
import { Action, StateUpdate } from "./proto/common.js";

export type { Action, StateUpdate };

/**
 * GameClient is an abstract base class that handles the full lifecycle of
 * connecting to the game engine: joining a lobby, running the game loop, and
 * dispatching state updates to the subclass.
 */
export abstract class GameClient {
  protected readonly playerId: string;
  private readonly serverUrl: string;
  private readonly channelCredentials: grpc.ChannelCredentials;

  private matchmakingClient: MatchmakingClient | null = null;
  private gameSessionClient: GameSessionClient | null = null;

  constructor(serverUrl: string, playerId: string) {
    this.serverUrl = serverUrl;
    this.playerId = playerId;
    this.channelCredentials = grpc.credentials.createInsecure();
  }

  // ---------------------------------------------------------------------------
  // Overridable factory methods — tests can swap these out via subclass or spy
  // ---------------------------------------------------------------------------

  /** @internal — exposed for testing */
  protected createMatchmakingClient(): MatchmakingClient {
    return new MatchmakingClient(this.serverUrl, this.channelCredentials);
  }

  /** @internal — exposed for testing */
  protected createGameSessionClient(): GameSessionClient {
    return new GameSessionClient(this.serverUrl, this.channelCredentials);
  }

  // ---------------------------------------------------------------------------
  // Public API
  // ---------------------------------------------------------------------------

  /**
   * joinLobby streams LobbyStatusUpdates until the server signals
   * `game_starting = true`, then resolves with the session_id.
   */
  async joinLobby(gameType: string): Promise<string> {
    this.matchmakingClient = this.createMatchmakingClient();

    return new Promise<string>((resolve, reject) => {
      const stream = this.matchmakingClient!.joinLobby({
        playerId: this.playerId,
        gameType,
        config: Buffer.alloc(0),
      });

      stream.on("data", (update) => {
        if (update.gameStarting) {
          stream.destroy();
          resolve(update.sessionId as string);
        }
      });

      stream.on("error", (err: Error) => reject(err));
      stream.on("end", () => {
        // Server closed the stream without game_starting — treat as error
        reject(new Error("Lobby stream ended before game started"));
      });
    });
  }

  /**
   * run opens a bidirectional Play stream and loops until the server sends a
   * terminal StateUpdate or the stream ends.  For each non-terminal update it
   * calls `onStateUpdate` and sends the returned Action back.
   *
   * The caller should have already called `joinLobby` to obtain a session_id
   * and have it available via the `sessionId` property, **or** override
   * `createGameSessionClient` to supply a pre-configured client.
   */
  async run(): Promise<void> {
    this.gameSessionClient = this.createGameSessionClient();

    return new Promise<void>((resolve, reject) => {
      const stream = this.gameSessionClient!.play();

      const cleanup = () => {
        try {
          stream.end();
        } catch {
          // ignore
        }
      };

      // Send the initial join action so the server knows which player this is.
      // The server requires the first message on the Play stream to have
      // actor_id set to the player's id (and an optional JSON session_id in
      // the payload for multi-player sessions).
      const joinAction: Action = {
        actorId: this.playerId,
        payload: Buffer.alloc(0),
        timestampMs: Date.now(),
      };
      stream.write(joinAction);

      stream.on("data", async (update: StateUpdate) => {
        try {
          // Call onStateUpdate for every update — including the terminal one —
          // so that subclasses can observe the final state. This mirrors the
          // Python client's behaviour.
          const action = await Promise.resolve(this.onStateUpdate(update));

          if (update.isTerminal) {
            // The game is over; don't send the action back, just clean up.
            cleanup();
            resolve();
            return;
          }

          // Stamp the actor id before sending
          const toSend: Action = { ...action, actorId: this.playerId };
          stream.write(toSend);
        } catch (err) {
          cleanup();
          reject(err as Error);
        }
      });

      stream.on("error", (err: Error) => {
        reject(err);
      });

      stream.on("end", () => {
        resolve();
      });
    });
  }

  /**
   * Called by `run()` for every non-terminal StateUpdate.
   * Subclasses must implement this to return the next Action.
   */
  abstract onStateUpdate(state: StateUpdate): Action | Promise<Action>;

  /**
   * Close all open gRPC channels.
   */
  close(): void {
    try {
      this.matchmakingClient?.close();
    } catch {
      // ignore
    }
    try {
      this.gameSessionClient?.close();
    } catch {
      // ignore
    }
    this.matchmakingClient = null;
    this.gameSessionClient = null;
  }
}
