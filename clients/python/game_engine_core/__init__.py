"""
game_engine_core — Python client SDK for game-engine-core gRPC services.

Quick-start::

    from game_engine_core import GameClient, Action
    from game_engine_core.actions import draw_card

    class MyBot(GameClient):
        def on_state_update(self, update):
            return draw_card(actor_id=self.player_id)

    bot = MyBot("localhost:50051", "player-1")
    session_id = bot.join_lobby("crazy-eights")
    bot.run()
    bot.close()
"""

from game_engine_core.client import Action, GameClient, State, StateUpdate

__all__ = ["GameClient", "Action", "State", "StateUpdate"]
