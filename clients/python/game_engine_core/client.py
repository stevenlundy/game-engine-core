"""
GameClient base class for game-engine-core Python SDK.

Subclass GameClient and override on_state_update() to implement your bot.
All gRPC plumbing is handled internally; callers only work with the typed
StateUpdate and Action wrappers defined in this module.
"""

from __future__ import annotations

import logging
import queue
import time
from dataclasses import dataclass, field
from typing import TYPE_CHECKING

import grpc

if TYPE_CHECKING:
    from collections.abc import Generator

from game_engine_core.proto import (
    common_pb2,
    gamesession_pb2_grpc,
    matchmaking_pb2,
    matchmaking_pb2_grpc,
)

logger = logging.getLogger(__name__)

# ---------------------------------------------------------------------------
# Typed wrappers — callers never import raw proto objects directly
# ---------------------------------------------------------------------------


@dataclass
class State:
    """Typed wrapper around the proto State message."""

    payload: bytes
    game_id: str
    step_index: int


@dataclass
class StateUpdate:
    """Typed wrapper around the proto StateUpdate message.

    Attributes:
        state: The new game state after the action was applied.
        reward_delta: Immediate reward / score change for the actor.
        is_terminal: True when this update ends the game.
        actor_id: Which player this update is addressed to.
    """

    state: State
    reward_delta: float
    is_terminal: bool
    actor_id: str

    @classmethod
    def from_proto(cls, proto_msg: common_pb2.StateUpdate) -> StateUpdate:  # type: ignore[name-defined]
        """Convert a raw proto StateUpdate into the typed wrapper."""
        s = proto_msg.state
        return cls(
            state=State(
                payload=s.payload,
                game_id=s.game_id,
                step_index=s.step_index,
            ),
            reward_delta=proto_msg.reward_delta,
            is_terminal=proto_msg.is_terminal,
            actor_id=proto_msg.actor_id,
        )


@dataclass
class Action:
    """Typed wrapper around the proto Action message.

    Attributes:
        actor_id: The player or AI that produced this action.
        payload: Opaque, game-specific encoded action bytes.
        timestamp_ms: Wall-clock time when the action was recorded (Unix ms).
    """

    actor_id: str
    payload: bytes = field(default=b"")
    timestamp_ms: int = field(default_factory=lambda: int(time.time() * 1000))

    def to_proto(self) -> common_pb2.Action:  # type: ignore[name-defined]
        """Convert to a raw proto Action for transmission."""
        return common_pb2.Action(  # type: ignore[attr-defined]
            actor_id=self.actor_id,
            payload=self.payload,
            timestamp_ms=self.timestamp_ms,
        )


# Sentinel used to signal the action-sending thread to stop.
_STOP = object()


# ---------------------------------------------------------------------------
# GameClient
# ---------------------------------------------------------------------------


class GameClient:
    """Base class for all game-engine-core bots.

    Subclass this and implement :meth:`on_state_update` to drive your agent.

    Example::

        class MyBot(GameClient):
            def on_state_update(self, update: StateUpdate) -> Action:
                return draw_card(actor_id=self.player_id)

        bot = MyBot("localhost:50051", "player-1")
        session_id = bot.join_lobby("crazy-eights")
        bot.run()
        bot.close()

    Args:
        server_url: gRPC server address, e.g. ``"localhost:50051"``.
        player_id: Unique identifier for this player / agent.
    """

    def __init__(self, server_url: str, player_id: str) -> None:
        self.server_url = server_url
        self.player_id = player_id
        self._channel: grpc.Channel | None = grpc.insecure_channel(server_url)
        self._session_id: str | None = None
        logger.debug("GameClient created for player=%s at %s", player_id, server_url)

    # ------------------------------------------------------------------
    # Public API
    # ------------------------------------------------------------------

    def join_lobby(self, game_type: str) -> str:
        """Join a lobby and wait until the game is starting.

        Calls ``Matchmaking.JoinLobby`` and blocks until the server emits a
        ``LobbyStatusUpdate`` with ``game_starting=True``.

        Args:
            game_type: The game type to request (e.g. ``"crazy-eights"``).

        Returns:
            The ``session_id`` assigned by the server.

        Raises:
            grpc.RpcError: If the stream terminates before ``game_starting``.
            RuntimeError: If the stream ends without a ``game_starting`` update.
        """
        stub = matchmaking_pb2_grpc.MatchmakingStub(self._channel)  # type: ignore[no-untyped-call]
        request = matchmaking_pb2.JoinRequest(  # type: ignore[attr-defined]
            player_id=self.player_id,
            game_type=game_type,
        )
        logger.info("Joining lobby: game_type=%s player_id=%s", game_type, self.player_id)
        try:
            for update in stub.JoinLobby(request):
                logger.debug(
                    "LobbyStatusUpdate: session_id=%s ready=%s starting=%s",
                    update.session_id,
                    list(update.ready_players),
                    update.game_starting,
                )
                if update.game_starting:
                    self._session_id = update.session_id
                    logger.info("Game starting: session_id=%s", self._session_id)
                    return self._session_id
        except grpc.RpcError as exc:
            logger.error("JoinLobby stream error: %s", exc)
            raise

        raise RuntimeError("JoinLobby stream ended without a game_starting=True update")

    def run(self) -> None:
        """Drive the bidirectional Play stream until the game ends.

        Opens the ``GameSession.Play`` stream, sends an initial join
        :class:`Action` with this player's ID, then loops:

        1. Receive a :class:`StateUpdate` from the server.
        2. Call :meth:`on_state_update` (implemented by the subclass).
        3. Send back the returned :class:`Action`.

        The loop exits when the server sends a terminal update
        (``is_terminal=True``) or closes the stream.

        Raises:
            grpc.RpcError: On stream-level errors (logged then re-raised).
        """
        stub = gamesession_pb2_grpc.GameSessionStub(self._channel)  # type: ignore[no-untyped-call]

        # A queue through which the main thread pushes proto Actions for the
        # sender generator to yield to gRPC.
        action_queue: queue.Queue[object] = queue.Queue()

        def _sender() -> Generator[object, None, None]:
            """Generator that yields Actions from action_queue to the stream."""
            # Initial join action
            join_action = Action(actor_id=self.player_id)
            logger.debug("Sending initial join action for player=%s", self.player_id)
            yield join_action.to_proto()
            while True:
                item = action_queue.get()
                if item is _STOP:
                    return
                yield item

        try:
            response_iter = stub.Play(_sender())
            for proto_update in response_iter:
                update = StateUpdate.from_proto(proto_update)
                logger.debug(
                    "Received StateUpdate: step=%d terminal=%s",
                    update.state.step_index,
                    update.is_terminal,
                )
                action = self.on_state_update(update)
                logger.debug(
                    "Sending Action: actor=%s payload_len=%d",
                    action.actor_id,
                    len(action.payload),
                )
                action_queue.put(action.to_proto())
                if update.is_terminal:
                    logger.info("Game terminal at step=%d", update.state.step_index)
                    action_queue.put(_STOP)
                    break
        except grpc.RpcError as exc:
            logger.error("Play stream error: %s", exc)
            action_queue.put(_STOP)
            raise

    def on_state_update(self, update: StateUpdate) -> Action:
        """Handle an incoming state update and return the next action.

        Subclasses **must** override this method.

        Args:
            update: The latest :class:`StateUpdate` from the server.

        Returns:
            The :class:`Action` to send back to the server.

        Raises:
            NotImplementedError: Always — subclasses must override this.
        """
        raise NotImplementedError(
            "Subclasses must implement on_state_update(self, update: StateUpdate) -> Action"
        )

    def close(self) -> None:
        """Shut down the gRPC channel cleanly.

        Safe to call multiple times; subsequent calls are no-ops.
        """
        if self._channel is not None:
            logger.debug("Closing gRPC channel for player=%s", self.player_id)
            self._channel.close()
            self._channel = None
