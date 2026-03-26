import { GameClient } from "../src/client.js";
import { Action, StateUpdate } from "../src/proto/common.js";
import { drawCard, playCard } from "../src/actions.js";

/**
 * RandomAgent picks a random action from a fixed repertoire each turn.
 *
 * Usage:
 *   const agent = new RandomAgent("localhost:50051", "bot-1");
 *   const sessionId = await agent.joinLobby("crazy-eights");
 *   await agent.run();
 *   agent.close();
 */
export class RandomAgent extends GameClient {
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
  const serverUrl = process.env["SERVER_URL"] ?? "localhost:50051";
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
