"""
Tests for GameClient in game_engine_core.client.

These tests mock the gRPC channel so no real server is needed.
"""
from __future__ import annotations

import json
from unittest.mock import MagicMock, patch, call

import grpc
import pytest

from game_engine_core.client import Action, GameClient, State, StateUpdate
from game_engine_core.proto import common_pb2, matchmaking_pb2


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_proto_state_update(
    step_index: int = 0,
    game_id: str = "test",
    payload: bytes = b"{}",
    reward_delta: float = 0.0,
    is_terminal: bool = False,
    actor_id: str = "player-1",
) -> common_pb2.StateUpdate:
    """Build a proto StateUpdate for use in tests."""
    return common_pb2.StateUpdate(
        state=common_pb2.State(
            payload=payload,
            game_id=game_id,
            step_index=step_index,
        ),
        reward_delta=reward_delta,
        is_terminal=is_terminal,
        actor_id=actor_id,
    )


def _make_proto_lobby_update(
    session_id: str = "sess-1",
    game_starting: bool = True,
    ready_players: list | None = None,
) -> matchmaking_pb2.LobbyStatusUpdate:
    return matchmaking_pb2.LobbyStatusUpdate(
        session_id=session_id,
        game_starting=game_starting,
        ready_players=ready_players or [],
    )


# ---------------------------------------------------------------------------
# GameClient.run() tests
# ---------------------------------------------------------------------------


class _TrackingBot(GameClient):
    """Test subclass that records calls and returns a canned Action."""

    def __init__(self, server_url, player_id, responses=None):
        super().__init__(server_url, player_id)
        self.received_updates: list[StateUpdate] = []
        # Pre-configured Action objects to return, one per call.
        self._responses = list(responses or [])

    def on_state_update(self, update: StateUpdate) -> Action:
        self.received_updates.append(update)
        if self._responses:
            return self._responses.pop(0)
        return Action(actor_id=self.player_id, payload=b"default")


class TestRunCallsOnStateUpdate:
    """run() must call on_state_update() once per received StateUpdate."""

    def _build_bot_with_mock_stream(self, proto_updates):
        """
        Construct a _TrackingBot whose gRPC channel is fully mocked.

        The mock stub.Play() is rigged to return `proto_updates` as the
        response iterator.  The actions yielded by the sender generator are
        collected and returned alongside the bot for assertion.
        """
        sent_proto_actions: list[common_pb2.Action] = []

        responses = [
            Action(actor_id="player-1", payload=f"action-{i}".encode())
            for i in range(len(proto_updates))
        ]
        bot = _TrackingBot("fake:50051", "player-1", responses=responses)

        mock_channel = MagicMock(spec=grpc.Channel)
        bot._channel = mock_channel

        # stub.Play(request_iterator) must return an iterable of proto updates.
        # We capture the request_iterator passed to Play so we can consume it
        # and record what actions were sent.
        def _play_side_effect(request_iterator):
            # Consume the initial join action from the generator.
            # We drain the generator only as far as needed per response,
            # mimicking what a real gRPC server would do (receive one action
            # per state update).
            # Collect the initial join action first.
            gen = iter(request_iterator)
            join_action = next(gen)  # initial join action
            sent_proto_actions.append(join_action)
            # Now return the response iterator.  As the caller iterates
            # over the response, they will call on_state_update() and
            # enqueue more actions.  We interleave by pulling one action
            # per yielded response inside the iterator.
            def _response_and_drain():
                for update in proto_updates:
                    yield update
                    # Drain one action from the generator (what was enqueued
                    # by the run() loop in response to this update).
                    try:
                        a = next(gen)
                        sent_proto_actions.append(a)
                    except StopIteration:
                        pass

            return _response_and_drain()

        with patch(
            "game_engine_core.client.gamesession_pb2_grpc.GameSessionStub"
        ) as MockStub:
            mock_stub = MockStub.return_value
            mock_stub.Play.side_effect = _play_side_effect
            bot.run()

        return bot, sent_proto_actions, MockStub

    def test_on_state_update_called_once_per_update(self):
        updates = [
            _make_proto_state_update(step_index=0),
            _make_proto_state_update(step_index=1, is_terminal=True),
        ]
        bot, sent_actions, _ = self._build_bot_with_mock_stream(updates)

        assert len(bot.received_updates) == 2
        assert bot.received_updates[0].state.step_index == 0
        assert bot.received_updates[1].state.step_index == 1
        assert bot.received_updates[1].is_terminal is True

    def test_actions_sent_back_for_each_update(self):
        updates = [
            _make_proto_state_update(step_index=0),
            _make_proto_state_update(step_index=1, is_terminal=True),
        ]
        bot, sent_actions, _ = self._build_bot_with_mock_stream(updates)

        # sent_actions[0] is the initial join action; [1] is the response to
        # the non-terminal update. The terminal update does NOT get a reply.
        assert len(sent_actions) == 2  # join + 1 response (not terminal)
        assert sent_actions[0].actor_id == "player-1"
        assert sent_actions[0].payload == b""
        assert sent_actions[1].payload == b"action-0"

    def test_single_terminal_update(self):
        updates = [_make_proto_state_update(step_index=0, is_terminal=True)]
        bot, sent_actions, _ = self._build_bot_with_mock_stream(updates)

        assert len(bot.received_updates) == 1
        assert bot.received_updates[0].is_terminal is True

    def test_state_update_fields_correctly_wrapped(self):
        payload = json.dumps({"hand": [{"rank": "8", "suit": "hearts"}]}).encode()
        updates = [
            _make_proto_state_update(
                step_index=5,
                game_id="crazy-eights",
                payload=payload,
                reward_delta=1.5,
                is_terminal=True,
                actor_id="player-1",
            )
        ]
        bot, _, _ = self._build_bot_with_mock_stream(updates)

        update = bot.received_updates[0]
        assert update.state.step_index == 5
        assert update.state.game_id == "crazy-eights"
        assert update.state.payload == payload
        assert update.reward_delta == 1.5
        assert update.is_terminal is True
        assert update.actor_id == "player-1"

    def test_on_state_update_not_implemented_raises(self):
        """The base class raises NotImplementedError for on_state_update."""
        bot = GameClient("fake:50051", "player-1")
        dummy_state = State(payload=b"", game_id="x", step_index=0)
        dummy_update = StateUpdate(
            state=dummy_state,
            reward_delta=0.0,
            is_terminal=False,
            actor_id="player-1",
        )
        with pytest.raises(NotImplementedError):
            bot.on_state_update(dummy_update)

    def test_run_propagates_grpc_error(self):
        """run() logs and re-raises gRPC errors from the Play stream."""
        bot = _TrackingBot("fake:50051", "player-1")
        mock_channel = MagicMock(spec=grpc.Channel)
        bot._channel = mock_channel

        def _play_raises(_gen):
            raise grpc.RpcError("stream failed")

        with patch(
            "game_engine_core.client.gamesession_pb2_grpc.GameSessionStub"
        ) as MockStub:
            mock_stub = MockStub.return_value
            mock_stub.Play.side_effect = _play_raises
            with pytest.raises(grpc.RpcError):
                bot.run()


# ---------------------------------------------------------------------------
# GameClient.join_lobby() tests
# ---------------------------------------------------------------------------


class TestJoinLobby:
    """join_lobby() must return session_id on game_starting=True."""

    def _make_bot(self):
        bot = GameClient("fake:50051", "player-1")
        bot._channel = MagicMock(spec=grpc.Channel)
        return bot

    def test_returns_session_id_when_game_starting(self):
        bot = self._make_bot()
        updates = [
            _make_proto_lobby_update(session_id="sess-abc", game_starting=True)
        ]
        with patch(
            "game_engine_core.client.matchmaking_pb2_grpc.MatchmakingStub"
        ) as MockStub:
            mock_stub = MockStub.return_value
            mock_stub.JoinLobby.return_value = iter(updates)
            result = bot.join_lobby("crazy-eights")

        assert result == "sess-abc"
        assert bot._session_id == "sess-abc"

    def test_blocks_until_game_starting(self):
        """join_lobby iterates past non-starting updates."""
        bot = self._make_bot()
        updates = [
            _make_proto_lobby_update(session_id="sess-1", game_starting=False),
            _make_proto_lobby_update(session_id="sess-1", game_starting=False),
            _make_proto_lobby_update(session_id="sess-1", game_starting=True),
        ]
        with patch(
            "game_engine_core.client.matchmaking_pb2_grpc.MatchmakingStub"
        ) as MockStub:
            mock_stub = MockStub.return_value
            mock_stub.JoinLobby.return_value = iter(updates)
            result = bot.join_lobby("crazy-eights")

        assert result == "sess-1"

    def test_raises_runtime_error_if_stream_ends_without_game_starting(self):
        """Stream closes without game_starting=True → RuntimeError."""
        bot = self._make_bot()
        updates = [
            _make_proto_lobby_update(session_id="sess-1", game_starting=False),
        ]
        with patch(
            "game_engine_core.client.matchmaking_pb2_grpc.MatchmakingStub"
        ) as MockStub:
            mock_stub = MockStub.return_value
            mock_stub.JoinLobby.return_value = iter(updates)
            with pytest.raises(RuntimeError, match="game_starting=True"):
                bot.join_lobby("crazy-eights")

    def test_raises_grpc_error_on_stream_error(self):
        """join_lobby propagates gRPC stream errors."""
        bot = self._make_bot()

        def _raising_iter(_req):
            raise grpc.RpcError("network failure")

        with patch(
            "game_engine_core.client.matchmaking_pb2_grpc.MatchmakingStub"
        ) as MockStub:
            mock_stub = MockStub.return_value
            mock_stub.JoinLobby.side_effect = _raising_iter
            with pytest.raises(grpc.RpcError):
                bot.join_lobby("crazy-eights")

    def test_raises_grpc_error_during_iteration(self):
        """join_lobby propagates gRPC errors raised during stream iteration."""
        bot = self._make_bot()

        def _error_iter():
            raise grpc.RpcError("mid-stream failure")
            yield  # make it a generator

        with patch(
            "game_engine_core.client.matchmaking_pb2_grpc.MatchmakingStub"
        ) as MockStub:
            mock_stub = MockStub.return_value
            mock_stub.JoinLobby.return_value = _error_iter()
            with pytest.raises(grpc.RpcError):
                bot.join_lobby("crazy-eights")


# ---------------------------------------------------------------------------
# GameClient.close() tests
# ---------------------------------------------------------------------------


class TestClose:
    def test_close_calls_channel_close(self):
        bot = GameClient("fake:50051", "player-1")
        mock_channel = MagicMock(spec=grpc.Channel)
        bot._channel = mock_channel
        bot.close()
        mock_channel.close.assert_called_once()

    def test_close_is_idempotent(self):
        bot = GameClient("fake:50051", "player-1")
        mock_channel = MagicMock(spec=grpc.Channel)
        bot._channel = mock_channel
        bot.close()
        bot.close()  # second call should not raise
        mock_channel.close.assert_called_once()
