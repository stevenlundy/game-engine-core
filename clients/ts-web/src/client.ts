import * as grpc from "@grpc/grpc-js";
import type { Action, StateUpdate } from "./proto/common";
import { GameSessionClient } from "./proto/gamesession";
import { MatchmakingClient } from "./proto/matchmaking";

export type { Action, StateUpdate };

/**
 * GameWebClient is an abstract base class for browser-based game agents.
 * It handles the full lifecycle of connecting to the game engine via a
 * grpc-web proxy: joining a lobby, running the game loop over a single
 * long-lived stream, and dispatching state updates to the subclass.
 *
 * Transport: Option A — one bidirectional Play stream for the entire game.
 * The stream is opened once in run(), the join Action is sent first, then
 * the loop is: recv StateUpdate → if terminal resolve → call onStateUpdate
 * → send Action → repeat.
 *
 * The public API is identical to the ts-node GameClient so developers can
 * swap the import and nothing else changes.
 */
export abstract class GameWebClient {
  protected readonly playerId: string;
  private readonly serverUrl: string;
  private readonly channelCredentials: grpc.ChannelCredentials;

  private matchmakingClient: MatchmakingClient | null = null;
  private gameSessionClient: GameSessionClient | null = null;

  constructor(serverUrl: string, playerId: string) {
    this.serverUrl = serverUrl;
    this.playerId = playerId;
    // grpc-web proxy typically listens on plain HTTP — use insecure credentials.
    // For TLS-enabled proxies, override createMatchmakingClient /
    // createGameSessionClient and pass grpc.credentials.createSsl() instead.
    this.channelCredentials = grpc.credentials.createInsecure();
  }

  // ---------------------------------------------------------------------------
  // Overridable factory methods — tests can swap these out via subclass
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
    const matchmakingClient = this.matchmakingClient;

    return new Promise<string>((resolve, reject) => {
      const stream = matchmakingClient.joinLobby({
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
   * run opens a single bidirectional Play stream and loops until the server
   * sends a terminal StateUpdate or the stream ends.  For each non-terminal
   * update it calls `onStateUpdate` and sends the returned Action back.
   *
   * Option A transport: ONE stream for the entire game — server only sends
   * a StateUpdate after receiving an Action (turn-based protocol).
   */
  async run(): Promise<void> {
    this.gameSessionClient = this.createGameSessionClient();
    const gameSessionClient = this.gameSessionClient;

    return new Promise<void>((resolve, reject) => {
      const stream = gameSessionClient.play();

      const cleanup = () => {
        try {
          stream.end();
        } catch {
          // ignore
        }
      };

      stream.on("data", async (update: StateUpdate) => {
        try {
          if (update.isTerminal) {
            cleanup();
            resolve();
            return;
          }

          const action = await Promise.resolve(this.onStateUpdate(update));
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
