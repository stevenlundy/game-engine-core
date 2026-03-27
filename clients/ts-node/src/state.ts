import type { StateUpdate } from "./proto/common.js";

export type { StateUpdate };

/**
 * RichState is the decoded view of a StateUpdate, including a parsed payload.
 */
export interface RichState {
  /** Raw StateUpdate received from the server. */
  raw: StateUpdate;
  /** Game-specific state parsed from state.payload as a JSON object. */
  gameState: Record<string, unknown> | null;
  /** Opaque game id (e.g. session identifier). */
  gameId: string;
  /** Monotonically increasing step counter. */
  stepIndex: number;
  /** Immediate reward for this transition. */
  rewardDelta: number;
  /** Whether this update ends the game. */
  isTerminal: boolean;
  /** The player this update is addressed to. */
  actorId: string;
}

/**
 * parseRichState decodes a StateUpdate into a RichState, attempting to
 * JSON-parse the opaque state payload.
 */
export function parseRichState(update: StateUpdate): RichState {
  let gameState: Record<string, unknown> | null = null;
  if (update.state?.payload && update.state.payload.length > 0) {
    try {
      gameState = JSON.parse(
        Buffer.from(update.state.payload).toString("utf8"),
      ) as Record<string, unknown>;
    } catch {
      gameState = null;
    }
  }

  return {
    raw: update,
    gameState,
    gameId: update.state?.gameId ?? "",
    stepIndex: update.state?.stepIndex ?? 0,
    rewardDelta: update.rewardDelta,
    isTerminal: update.isTerminal,
    actorId: update.actorId,
  };
}
