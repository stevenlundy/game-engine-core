"""
Factory functions for constructing common game Actions.

These helpers create typed :class:`~game_engine_core.client.Action` objects
so callers never need to construct raw proto messages directly.

Example::

    from game_engine_core.actions import play_card, draw_card

    action = play_card("8", "hearts", declared_suit="spades")
    action = draw_card()
"""

from __future__ import annotations

import json
import time

from game_engine_core.client import Action


def play_card(
    rank: str,
    suit: str,
    declared_suit: str | None = None,
    *,
    actor_id: str = "",
    timestamp_ms: int | None = None,
) -> Action:
    """Create an action that plays a card from the player's hand.

    Args:
        rank: Card rank, e.g. ``"8"``, ``"A"``, ``"K"``.
        suit: Card suit, e.g. ``"hearts"``, ``"spades"``.
        declared_suit: For wild cards (e.g. 8s in Crazy Eights), the suit
            the player declares.  ``None`` if not applicable.
        actor_id: The player ID.  Defaults to empty string; override when
            the session already knows the player.
        timestamp_ms: Action timestamp in Unix milliseconds.  Defaults to
            the current wall-clock time.

    Returns:
        A typed :class:`~game_engine_core.client.Action` ready to send.
    """
    payload: dict[str, str] = {"type": "play_card", "rank": rank, "suit": suit}
    if declared_suit is not None:
        payload["declared_suit"] = declared_suit
    return Action(
        actor_id=actor_id,
        payload=json.dumps(payload).encode(),
        timestamp_ms=timestamp_ms if timestamp_ms is not None else int(time.time() * 1000),
    )


def draw_card(
    *,
    actor_id: str = "",
    timestamp_ms: int | None = None,
) -> Action:
    """Create an action that draws the top card from the draw pile.

    Args:
        actor_id: The player ID.  Defaults to empty string.
        timestamp_ms: Action timestamp in Unix milliseconds.  Defaults to
            the current wall-clock time.

    Returns:
        A typed :class:`~game_engine_core.client.Action` ready to send.
    """
    payload: dict[str, str] = {"type": "draw_card"}
    return Action(
        actor_id=actor_id,
        payload=json.dumps(payload).encode(),
        timestamp_ms=timestamp_ms if timestamp_ms is not None else int(time.time() * 1000),
    )
