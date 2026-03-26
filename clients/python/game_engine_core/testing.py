"""
game_engine_core.testing — reusable pytest fixtures for integration tests.

This module provides a ``game_server`` pytest fixture that:

1. Builds the ``cmd/testserver`` Go binary (once per test session).
2. Spawns it on a free TCP port before each test.
3. Waits until the port is accepting connections.
4. Yields a ``ServerInfo`` with the ``server_url`` and config.
5. Terminates the process after the test.

**Usage in your game repo**

Add ``game-engine-core`` as a test dependency, then in your
``tests/conftest.py``::

    from game_engine_core.testing import game_server  # noqa: F401 — registers fixture

    # The fixture is now available to all tests in this directory.

In your test::

    def test_my_ai_beats_random(game_server):
        bot = MyAI(game_server.url, player_id="my-ai")
        session_id = bot.join_lobby(game_server.game_type)
        bot.run()
        bot.close()

**Configuring the test server**

Pass ``pytest`` markers or override the fixture for custom step counts::

    @pytest.fixture
    def game_server(game_server_factory):
        return game_server_factory(countdown_steps=10, max_players=2)

**How it finds the binary**

The fixture looks for the Go repo root by walking up from ``__file__`` (or
``GAME_ENGINE_CORE_ROOT`` env var) until it finds a ``go.mod`` containing
``game-engine-core``.  It then runs ``go build -o <tmp> ./cmd/testserver/``.
"""
from __future__ import annotations

import os
import socket
import subprocess
import tempfile
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Generator, Optional

import pytest


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _find_repo_root() -> Path:
    """Walk up from this file (or $GAME_ENGINE_CORE_ROOT) to find the Go repo root."""
    env_root = os.environ.get("GAME_ENGINE_CORE_ROOT")
    if env_root:
        p = Path(env_root)
        if (p / "go.mod").exists():
            return p
        raise RuntimeError(
            f"GAME_ENGINE_CORE_ROOT={env_root!r} does not contain a go.mod"
        )

    # Walk up from this file's location.
    candidate = Path(__file__).resolve().parent
    for _ in range(10):
        go_mod = candidate / "go.mod"
        if go_mod.exists():
            text = go_mod.read_text()
            if "game-engine-core" in text:
                return candidate
        parent = candidate.parent
        if parent == candidate:
            break
        candidate = parent

    raise RuntimeError(
        "Cannot find game-engine-core repo root. "
        "Set the GAME_ENGINE_CORE_ROOT environment variable to the repo path."
    )


def _free_port() -> int:
    """Return an unused TCP port on localhost."""
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def _wait_for_port(host: str, port: int, timeout: float = 10.0) -> None:
    """Block until the given TCP port accepts connections or timeout expires."""
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            with socket.create_connection((host, port), timeout=0.2):
                return
        except OSError:
            time.sleep(0.05)
    raise TimeoutError(
        f"Test server did not start on {host}:{port} within {timeout}s"
    )


# ---------------------------------------------------------------------------
# Public API
# ---------------------------------------------------------------------------


@dataclass
class ServerInfo:
    """Metadata about the running test server instance.

    Attributes:
        url: gRPC server address, e.g. ``"localhost:12345"``.
        port: The TCP port in use.
        game_type: The game type the server is configured for.
        countdown_steps: Number of steps before the CountdownGame ends.
        max_players: Number of players required to start a session.
        process: The underlying :class:`subprocess.Popen` handle.
    """

    url: str
    port: int
    game_type: str
    countdown_steps: int
    max_players: int
    process: subprocess.Popen


def build_testserver(repo_root: Optional[Path] = None) -> Path:
    """Build ``cmd/testserver`` and return the path to the binary.

    The binary is written to a temporary directory and cached for the
    current process (subsequent calls return the same path if the binary
    still exists).

    Args:
        repo_root: Path to the ``game-engine-core`` repository root.
            Autodetected if ``None``.

    Returns:
        Path to the compiled ``testserver`` binary.

    Raises:
        subprocess.CalledProcessError: If ``go build`` fails.
    """
    if repo_root is None:
        repo_root = _find_repo_root()

    # Re-use the cached binary for the lifetime of the test session.
    cache_attr = "_testserver_binary"
    cached: Optional[Path] = getattr(build_testserver, cache_attr, None)
    if cached is not None and cached.exists():
        return cached

    tmp_dir = Path(tempfile.mkdtemp(prefix="game-engine-core-testserver-"))
    binary = tmp_dir / "testserver"

    result = subprocess.run(
        ["go", "build", "-o", str(binary), "./cmd/testserver/"],
        cwd=str(repo_root),
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        raise subprocess.CalledProcessError(
            result.returncode,
            result.args,
            output=result.stdout,
            stderr=result.stderr,
        )

    setattr(build_testserver, cache_attr, binary)
    return binary


def start_testserver(
    *,
    repo_root: Optional[Path] = None,
    game_type: str = "countdown",
    countdown_steps: int = 5,
    max_players: int = 1,
    log_dir: Optional[str] = None,
) -> ServerInfo:
    """Build (if needed) and start the test server.

    Args:
        repo_root: Path to the ``game-engine-core`` repo root. Autodetected.
        game_type: Game type string passed to the server.
        countdown_steps: Steps before ``CountdownGame`` ends.
        max_players: Players required to start a session.
        log_dir: Directory for ``.glog`` files. ``None`` disables replay writing.

    Returns:
        A :class:`ServerInfo` with ``url``, ``port``, and ``process``.
    """
    binary = build_testserver(repo_root)
    port = _free_port()

    env = os.environ.copy()
    env["PORT"] = str(port)
    env["GAME_TYPE"] = game_type
    env["COUNTDOWN_STEPS"] = str(countdown_steps)
    env["MAX_PLAYERS"] = str(max_players)
    if log_dir:
        env["LOG_DIR"] = log_dir

    proc = subprocess.Popen(
        [str(binary)],
        env=env,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )

    try:
        _wait_for_port("127.0.0.1", port)
    except TimeoutError:
        proc.terminate()
        proc.wait(timeout=5)
        raise

    return ServerInfo(
        url=f"localhost:{port}",
        port=port,
        game_type=game_type,
        countdown_steps=countdown_steps,
        max_players=max_players,
        process=proc,
    )


def stop_testserver(info: ServerInfo, timeout: float = 5.0) -> None:
    """Terminate the test server process.

    Args:
        info: The :class:`ServerInfo` returned by :func:`start_testserver`.
        timeout: Seconds to wait for graceful termination before killing.
    """
    info.process.terminate()
    try:
        info.process.wait(timeout=timeout)
    except subprocess.TimeoutExpired:
        info.process.kill()
        info.process.wait()


# ---------------------------------------------------------------------------
# Pytest fixtures
# ---------------------------------------------------------------------------


@pytest.fixture(scope="function")
def game_server() -> Generator[ServerInfo, None, None]:
    """Pytest fixture: start a CountdownGame test server, yield ServerInfo, stop it.

    Default configuration: 5 steps, 1 player, game_type="countdown".

    To customise, use :func:`game_server_factory` instead::

        def test_two_player(game_server_factory):
            with game_server_factory(countdown_steps=10, max_players=2) as srv:
                ...
    """
    info = start_testserver()
    try:
        yield info
    finally:
        stop_testserver(info)


@pytest.fixture(scope="function")
def game_server_factory() -> Generator:
    """Pytest fixture: factory for customised test server instances.

    Usage::

        def test_long_game(game_server_factory):
            with game_server_factory(countdown_steps=20) as srv:
                bot = MyBot(srv.url, "p1")
                ...

    The server is stopped automatically when the ``with`` block exits.
    """
    import contextlib

    @contextlib.contextmanager
    def _factory(
        game_type: str = "countdown",
        countdown_steps: int = 5,
        max_players: int = 1,
        log_dir: Optional[str] = None,
    ):
        info = start_testserver(
            game_type=game_type,
            countdown_steps=countdown_steps,
            max_players=max_players,
            log_dir=log_dir,
        )
        try:
            yield info
        finally:
            stop_testserver(info)

    yield _factory
