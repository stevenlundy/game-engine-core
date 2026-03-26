"""
State parsing helpers for the game-engine-core Python SDK.

The raw :class:`~game_engine_core.client.StateUpdate` carries opaque bytes in
``state.payload``.  :func:`parse_rich_state` unmarshals that JSON payload into
a :class:`RichState` dataclass that exposes the most commonly needed fields.

Example::

    from game_engine_core.state import parse_rich_state

    def on_state_update(self, update):
        rich = parse_rich_state(update)
        print(rich.hand, rich.top_card)
        return draw_card(actor_id=self.player_id)
"""
from __future__ import annotations

import json
import logging
from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional

from game_engine_core.client import StateUpdate

logger = logging.getLogger(__name__)


@dataclass
class RichState:
    """Parsed, human-friendly representation of a game state payload.

    The fields here reflect the conventions used by game implementations
    built on game-engine-core.  Fields whose keys are absent from the
    payload are left as their default values.

    Attributes:
        game_id: Identifier of the game type or session.
        step_index: Monotonically increasing step counter.
        hand: The cards currently in this player's hand (list of dicts
            with ``rank`` and ``suit`` keys).
        top_card: The top card of the discard pile, if present.
        current_player: The player ID whose turn it is.
        scores: A mapping from player ID to current score.
        is_terminal: Whether the game has ended.
        raw: The full, unparsed JSON dict for fields not covered above.
    """

    game_id: str = ""
    step_index: int = 0
    hand: List[Dict[str, str]] = field(default_factory=list)
    top_card: Optional[Dict[str, str]] = None
    current_player: str = ""
    scores: Dict[str, float] = field(default_factory=dict)
    is_terminal: bool = False
    raw: Dict[str, Any] = field(default_factory=dict)


def parse_rich_state(state_update: StateUpdate) -> RichState:
    """Unmarshal the JSON payload from a :class:`StateUpdate` into a :class:`RichState`.

    The ``state.payload`` bytes are expected to be UTF-8 encoded JSON.  If the
    payload is empty or cannot be decoded, a :class:`RichState` is returned
    with only the ``game_id``, ``step_index``, and ``is_terminal`` fields set.

    Args:
        state_update: A typed :class:`~game_engine_core.client.StateUpdate`.

    Returns:
        A :class:`RichState` instance populated from the payload.
    """
    rich = RichState(
        game_id=state_update.state.game_id,
        step_index=state_update.state.step_index,
        is_terminal=state_update.is_terminal,
    )

    payload_bytes = state_update.state.payload
    if not payload_bytes:
        return rich

    try:
        data: Dict[str, Any] = json.loads(payload_bytes.decode("utf-8"))
    except (json.JSONDecodeError, UnicodeDecodeError) as exc:
        logger.warning("Failed to parse state payload as JSON: %s", exc)
        return rich

    rich.raw = data
    rich.hand = data.get("hand", [])
    rich.top_card = data.get("top_card")
    rich.current_player = data.get("current_player", "")
    rich.scores = data.get("scores", {})
    return rich
