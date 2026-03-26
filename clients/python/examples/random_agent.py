"""
random_agent.py — minimal GameClient subclass that plays randomly.

Run against a local game-engine-core server:

    uv run python examples/random_agent.py --server localhost:50051 \
        --player my-bot --game crazy-eights
"""
from __future__ import annotations

import argparse
import json
import logging
import random

from game_engine_core import GameClient, Action
from game_engine_core.actions import play_card, draw_card
from game_engine_core.state import parse_rich_state

logging.basicConfig(level=logging.INFO)


class RandomAgent(GameClient):
    """An agent that plays a random legal card, or draws if none available."""

    def on_state_update(self, update) -> Action:
        if update.is_terminal:
            # Terminal updates don't need a response, but the base class
            # won't call us again after this — just return anything.
            return draw_card(actor_id=self.player_id)

        rich = parse_rich_state(update)

        # If we have cards in hand, randomly decide to play or draw.
        if rich.hand and random.random() < 0.7:
            card = random.choice(rich.hand)
            declared = random.choice(["S", "H", "D", "C"]) if card.get("rank") == "8" else None
            return play_card(
                rank=card["rank"],
                suit=card["suit"],
                declared_suit=declared,
                actor_id=self.player_id,
            )

        return draw_card(actor_id=self.player_id)


def main() -> None:
    parser = argparse.ArgumentParser(description="Random Crazy Eights agent")
    parser.add_argument("--server", default="localhost:50051")
    parser.add_argument("--player", default="random-bot")
    parser.add_argument("--game", default="crazy-eights")
    args = parser.parse_args()

    agent = RandomAgent(args.server, player_id=args.player)
    try:
        print(f"Joining lobby for game '{args.game}' as '{args.player}'...")
        session_id = agent.join_lobby(args.game)
        print(f"Game starting! session_id={session_id}")
        agent.run()
        print("Game complete.")
    finally:
        agent.close()


if __name__ == "__main__":
    main()
