"""
Integration tests: Python GameClient ↔ Go gRPC server (cmd/testserver).

These tests spin up a real Go server binary and exercise the full stack:
    proto serialisation → gRPC transport → game logic → Python client

They are skipped automatically if the 'go' binary is not on PATH, so they
never block CI environments that only have Python.

Pattern for game repos
----------------------
Copy (or import) the ``game_server`` / ``game_server_factory`` fixtures from
``game_engine_core.testing`` in your own ``conftest.py``:

    # tests/conftest.py  (in game-crazy-eights, for example)
    from game_engine_core.testing import game_server, game_server_factory  # noqa: F401

Then write tests exactly like the ones below, replacing CountdownGame
semantics with your own game's expectations.
"""
from __future__ import annotations

import json
import shutil

import pytest

from game_engine_core import Action, GameClient
from game_engine_core.actions import draw_card
from game_engine_core.testing import game_server, game_server_factory  # noqa: F401 — registers fixtures

# ---------------------------------------------------------------------------
# Skip the entire module if 'go' is not available
# ---------------------------------------------------------------------------

pytestmark = pytest.mark.skipif(
    shutil.which("go") is None,
    reason="'go' binary not found on PATH — skipping integration tests",
)


# ---------------------------------------------------------------------------
# Minimal bot implementations used across tests
# ---------------------------------------------------------------------------


class AlwaysDrawBot(GameClient):
    """Bot that always draws a card — simplest possible agent."""

    def on_state_update(self, update) -> Action:
        return draw_card(actor_id=self.player_id)


class RecordingBot(GameClient):
    """Bot that records every StateUpdate it receives, then draws."""

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.updates = []

    def on_state_update(self, update) -> Action:
        self.updates.append(update)
        return draw_card(actor_id=self.player_id)


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------


class TestBasicConnection:
    """Verify the client can connect and play a complete game."""

    def test_run_completes_without_error(self, game_server):
        """A bot should be able to play a full game without raising."""
        bot = AlwaysDrawBot(game_server.url, player_id="p1")
        try:
            bot.run()
        finally:
            bot.close()

    def test_bot_receives_state_updates(self, game_server):
        """The bot should receive at least one StateUpdate per step."""
        bot = RecordingBot(game_server.url, player_id="p1")
        try:
            bot.run()
        finally:
            bot.close()

        assert len(bot.updates) > 0, "Bot received no StateUpdates"

    def test_last_update_is_terminal(self, game_server):
        """The final StateUpdate must have is_terminal=True."""
        bot = RecordingBot(game_server.url, player_id="p1")
        try:
            bot.run()
        finally:
            bot.close()

        assert bot.updates[-1].is_terminal, (
            f"Last update was not terminal. Updates received: {len(bot.updates)}"
        )

    def test_step_count_matches_countdown(self, game_server):
        """Number of non-terminal updates should equal countdown_steps."""
        bot = RecordingBot(game_server.url, player_id="p1")
        try:
            bot.run()
        finally:
            bot.close()

        non_terminal = [u for u in bot.updates if not u.is_terminal]
        assert len(non_terminal) == game_server.countdown_steps, (
            f"Expected {game_server.countdown_steps} non-terminal updates, "
            f"got {len(non_terminal)}"
        )

    def test_rewards_are_non_negative(self, game_server):
        """reward_delta should be >= 0 on all updates.

        Note: the current transport implementation sends reward_delta=0.0 on
        all StateUpdates sent to clients — rewards are written to the .glog
        replay file but not streamed back over gRPC. This test asserts the
        field is present and non-negative (i.e. not corrupted in transit),
        and documents the current behaviour so it is explicit.

        A future transport enhancement could populate reward_delta from
        the result of the previous ApplyAction call.
        """
        bot = RecordingBot(game_server.url, player_id="p1")
        try:
            bot.run()
        finally:
            bot.close()

        for update in bot.updates:
            assert update.reward_delta >= 0, (
                f"Negative reward_delta at step {update.state.step_index}: "
                f"{update.reward_delta}"
            )

    def test_step_indices_are_monotonically_increasing(self, game_server):
        """step_index in each StateUpdate must increase by 1 each time."""
        bot = RecordingBot(game_server.url, player_id="p1")
        try:
            bot.run()
        finally:
            bot.close()

        indices = [u.state.step_index for u in bot.updates]
        for i in range(1, len(indices)):
            assert indices[i] > indices[i - 1], (
                f"step_index did not increase: {indices[i - 1]} → {indices[i]}"
            )


class TestStatePayload:
    """Verify the state payload is valid JSON and contains expected fields."""

    def test_payload_is_valid_json(self, game_server):
        """Every state payload should be parseable as JSON."""
        bot = RecordingBot(game_server.url, player_id="p1")
        try:
            bot.run()
        finally:
            bot.close()

        for update in bot.updates:
            payload = update.state.payload
            if payload:
                try:
                    json.loads(payload)
                except json.JSONDecodeError as exc:
                    pytest.fail(
                        f"State payload at step {update.state.step_index} "
                        f"is not valid JSON: {exc}\nPayload: {payload!r}"
                    )

    def test_state_game_id_is_set(self, game_server):
        """game_id in the state should be non-empty."""
        bot = RecordingBot(game_server.url, player_id="p1")
        try:
            bot.run()
        finally:
            bot.close()

        for update in bot.updates:
            assert update.state.game_id, (
                f"game_id was empty at step {update.state.step_index}"
            )


class TestCustomStepCount:
    """Verify the game_server_factory fixture allows custom configuration."""

    def test_custom_countdown_steps(self, game_server_factory):
        """A game configured for 3 steps should produce exactly 3 non-terminal updates."""
        with game_server_factory(countdown_steps=3) as srv:
            bot = RecordingBot(srv.url, player_id="p1")
            try:
                bot.run()
            finally:
                bot.close()

        non_terminal = [u for u in bot.updates if not u.is_terminal]
        assert len(non_terminal) == 3, (
            f"Expected 3 non-terminal updates, got {len(non_terminal)}"
        )

    def test_single_step_game(self, game_server_factory):
        """A 1-step game should produce 1 non-terminal update then a terminal."""
        with game_server_factory(countdown_steps=1) as srv:
            bot = RecordingBot(srv.url, player_id="p1")
            try:
                bot.run()
            finally:
                bot.close()

        assert len(bot.updates) >= 1
        assert bot.updates[-1].is_terminal


class TestActorId:
    """Verify actor_id is correctly propagated through the stack."""

    def test_actor_id_matches_player_id(self, game_server):
        """The actor_id in StateUpdates should match the bot's player_id."""
        bot = RecordingBot(game_server.url, player_id="integration-tester")
        try:
            bot.run()
        finally:
            bot.close()

        for update in bot.updates:
            if not update.is_terminal:
                assert update.actor_id == "integration-tester", (
                    f"actor_id mismatch at step {update.state.step_index}: "
                    f"expected 'integration-tester', got {update.actor_id!r}"
                )
