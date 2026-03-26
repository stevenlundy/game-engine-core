import { Action } from "./proto/common.js";

/**
 * Build a "play card" Action payload.
 *
 * @param rank  Card rank, e.g. "A", "K", "10"
 * @param suit  Card suit, e.g. "hearts", "spades"
 * @param declaredSuit  Optional declared suit (e.g. for wild-card / crazy-eights rules)
 */
export function playCard(
  rank: string,
  suit: string,
  declaredSuit?: string,
): Action {
  const payload = JSON.stringify({ type: "play_card", rank, suit, ...(declaredSuit ? { declared_suit: declaredSuit } : {}) });
  return {
    actorId: "",
    payload: Buffer.from(payload, "utf8"),
    timestampMs: Date.now(),
  };
}

/**
 * Build a "draw card" Action payload.
 */
export function drawCard(): Action {
  const payload = JSON.stringify({ type: "draw_card" });
  return {
    actorId: "",
    payload: Buffer.from(payload, "utf8"),
    timestampMs: Date.now(),
  };
}
