/**
 * randomAgent.ts — minimal GameWebClient subclass that picks a random action
 * each turn.  Mirrors ts-node's randomAgent.ts.
 *
 * Usage (Node.js with Envoy running on localhost:8080):
 *
 *   SERVER_URL=http://localhost:8080 PLAYER_ID=bot-1 ts-node examples/randomAgent.ts
 */

import { GameWebClient } from "../src/client";
import { Action, StateUpdate } from "../src/proto/common";

// ---------------------------------------------------------------------------
// Action helpers (inline so the example has no extra imports)
// ---------------------------------------------------------------------------

function playCard(rank: string, suit: string, declaredSuit?: string): Action {
  const payload = JSON.stringify({
    type: "play_card",
    rank,
    suit,
    ...(declaredSuit ? { declared_suit: declaredSuit } : {}),
  });
  return {
    actorId: "",
    payload: Buffer.from(payload, "utf8"),
    timestampMs: Date.now(),
  };
}

function drawCard(): Action {
  const payload = JSON.stringify({ type: "draw_card" });
  return {
    actorId: "",
    payload: Buffer.from(payload, "utf8"),
    timestampMs: Date.now(),
  };
}

// ---------------------------------------------------------------------------
// RandomAgent
// ---------------------------------------------------------------------------

/**
 * RandomAgent picks a random action from a fixed repertoire each turn.
 *
 * Usage:
 *   const agent = new RandomAgent("http://localhost:8080", "bot-1");
 *   const sessionId = await agent.joinLobby("crazy-eights");
 *   await agent.run();
 *   agent.close();
 */
export class RandomAgent extends GameWebClient {
  constructor(serverUrl: string, playerId: string) {
    super(serverUrl, playerId);
  }

  onStateUpdate(_state: StateUpdate): Action {
    // 50% chance to play a card, 50% to draw
    if (Math.random() < 0.5) {
      const ranks = ["A", "2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K"];
      const suits = ["hearts", "diamonds", "clubs", "spades"];
      const rank = ranks[Math.floor(Math.random() * ranks.length)];
      const suit = suits[Math.floor(Math.random() * suits.length)];
      return playCard(rank, suit);
    }
    return drawCard();
  }
}

// Run directly: ts-node examples/randomAgent.ts
if (require.main === module) {
  const serverUrl = process.env["SERVER_URL"] ?? "http://localhost:8080";
  const playerId = process.env["PLAYER_ID"] ?? "bot-random";

  const agent = new RandomAgent(serverUrl, playerId);

  (async () => {
    try {
      console.log(`Joining lobby on ${serverUrl} as ${playerId}…`);
      const sessionId = await agent.joinLobby("crazy-eights");
      console.log(`Game starting — session: ${sessionId}`);
      await agent.run();
      console.log("Game finished.");
    } catch (err) {
      console.error("Error:", err);
    } finally {
      agent.close();
    }
  })();
}
