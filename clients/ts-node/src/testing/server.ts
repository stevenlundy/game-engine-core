/**
 * game-engine-core testing helpers for integration tests.
 *
 * Provides utilities to build and start the `cmd/testserver` Go binary,
 * wait until it is ready, and stop it cleanly — mirroring the Python
 * `game_engine_core.testing` module.
 *
 * Usage:
 *
 *   import { startTestServer, stopTestServer } from 'game-engine-core-node/testing/server.js'
 *
 *   let server: ServerInfo
 *   beforeAll(async () => { server = await startTestServer({ countdownSteps: 5 }) })
 *   afterAll(() => stopTestServer(server))
 *
 *   test('bot plays a full game', async () => {
 *     const bot = new MyBot(server.url, 'p1')
 *     await bot.run()
 *     bot.close()
 *   })
 */

import { type ChildProcess, execFileSync, spawn } from "child_process";
import * as fs from "fs";
import * as net from "net";
import * as os from "os";
import * as path from "path";

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Metadata about a running test server instance. */
export interface ServerInfo {
  /** gRPC server address, e.g. "localhost:12345" */
  url: string;
  /** The TCP port in use */
  port: number;
  /** The game type the server is configured for */
  gameType: string;
  /** Number of steps before the CountdownGame ends */
  countdownSteps: number;
  /** Number of players required to start a session */
  maxPlayers: number;
  /** The underlying child process handle */
  process: ChildProcess;
}

/** Options for startTestServer */
export interface StartOptions {
  /** Path to the game-engine-core repo root. Autodetected if omitted. */
  repoRoot?: string;
  /** Game type string (default: "countdown") */
  gameType?: string;
  /** Steps before CountdownGame ends (default: 5) */
  countdownSteps?: number;
  /** Players required to start a session (default: 1) */
  maxPlayers?: number;
  /** Directory for .glog replay files. Omit to disable replay writing. */
  logDir?: string;
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

/**
 * Walk up from `startDir` (or GAME_ENGINE_CORE_ROOT env var) until we find
 * a directory containing a go.mod that mentions "game-engine-core".
 */
function findRepoRoot(startDir?: string): string {
  const envRoot = process.env["GAME_ENGINE_CORE_ROOT"];
  if (envRoot) {
    const goMod = path.join(envRoot, "go.mod");
    if (fs.existsSync(goMod)) {
      return envRoot;
    }
    throw new Error(
      `GAME_ENGINE_CORE_ROOT=${envRoot} does not contain a go.mod`,
    );
  }

  // Walk up from the provided dir (or this file's location)
  let candidate = startDir ?? __dirname;
  for (let i = 0; i < 15; i++) {
    const goMod = path.join(candidate, "go.mod");
    if (fs.existsSync(goMod)) {
      const content = fs.readFileSync(goMod, "utf8");
      if (content.includes("game-engine-core")) {
        return candidate;
      }
    }
    const parent = path.dirname(candidate);
    if (parent === candidate) break;
    candidate = parent;
  }

  throw new Error(
    "Cannot find game-engine-core repo root. " +
      "Set the GAME_ENGINE_CORE_ROOT environment variable to the repo path.",
  );
}

/** Return a free TCP port by binding to port 0 and reading the assigned port. */
function findFreePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const srv = net.createServer();
    srv.listen(0, "127.0.0.1", () => {
      const addr = srv.address();
      if (!addr || typeof addr === "string") {
        srv.close();
        reject(new Error("Could not determine free port"));
        return;
      }
      const port = addr.port;
      srv.close(() => resolve(port));
    });
    srv.on("error", reject);
  });
}

/** Poll until the TCP port accepts connections or `timeoutMs` expires. */
function waitForPort(port: number, timeoutMs = 10_000): Promise<void> {
  const deadline = Date.now() + timeoutMs;

  return new Promise((resolve, reject) => {
    function attempt() {
      if (Date.now() > deadline) {
        reject(
          new Error(
            `Test server did not start on port ${port} within ${timeoutMs}ms`,
          ),
        );
        return;
      }

      const socket = net.createConnection({ host: "127.0.0.1", port });
      socket.on("connect", () => {
        socket.destroy();
        resolve();
      });
      socket.on("error", () => {
        socket.destroy();
        setTimeout(attempt, 50);
      });
    }

    attempt();
  });
}

// ---------------------------------------------------------------------------
// Build cache — binary is built once per process lifetime
// ---------------------------------------------------------------------------

let _cachedBinary: string | null = null;

/**
 * Build `cmd/testserver` and return the path to the compiled binary.
 *
 * The result is cached for the lifetime of the current process — subsequent
 * calls return the same path without re-building.
 *
 * @param repoRoot  Optional path to the game-engine-core repo root.
 *                  Autodetected via `findRepoRoot()` if omitted.
 */
export function buildTestServer(repoRoot?: string): string {
  if (_cachedBinary && fs.existsSync(_cachedBinary)) {
    return _cachedBinary;
  }

  const root = repoRoot ?? findRepoRoot();
  const tmpDir = fs.mkdtempSync(
    path.join(os.tmpdir(), "game-engine-core-testserver-"),
  );
  const binary = path.join(tmpDir, "testserver");

  execFileSync("go", ["build", "-o", binary, "./cmd/testserver/"], {
    cwd: root,
    stdio: ["ignore", "ignore", "pipe"],
  });

  _cachedBinary = binary;
  return binary;
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/**
 * Build (if needed) and start the test server.
 *
 * Equivalent to the Python `start_testserver()` helper.
 *
 * @param opts  Optional configuration; all fields have sensible defaults.
 * @returns     A `ServerInfo` describing the running server.
 */
export async function startTestServer(
  opts: StartOptions = {},
): Promise<ServerInfo> {
  const {
    repoRoot,
    gameType = "countdown",
    countdownSteps = 5,
    maxPlayers = 1,
    logDir,
  } = opts;

  const binary = buildTestServer(repoRoot);
  const port = await findFreePort();

  const env: NodeJS.ProcessEnv = {
    ...process.env,
    PORT: String(port),
    GAME_TYPE: gameType,
    COUNTDOWN_STEPS: String(countdownSteps),
    MAX_PLAYERS: String(maxPlayers),
  };
  if (logDir) {
    env["LOG_DIR"] = logDir;
  }

  const proc = spawn(binary, [], {
    env,
    stdio: ["ignore", "ignore", "ignore"],
    detached: false,
  });

  try {
    await waitForPort(port);
  } catch (err) {
    proc.kill("SIGTERM");
    throw err;
  }

  return {
    url: `localhost:${port}`,
    port,
    gameType,
    countdownSteps,
    maxPlayers,
    process: proc,
  };
}

/**
 * Terminate a running test server.
 *
 * Equivalent to the Python `stop_testserver()` helper.
 *
 * @param info  The `ServerInfo` returned by `startTestServer`.
 */
export function stopTestServer(info: ServerInfo): void {
  info.process.kill("SIGTERM");
}

/**
 * Convenience wrapper: start the server, call `fn(info)`, stop the server.
 *
 * Suitable for use inline or inside beforeAll/afterAll hooks.
 *
 * @example
 *   await withTestServer({ countdownSteps: 3 }, async (info) => {
 *     const bot = new MyBot(info.url, 'p1')
 *     await bot.run()
 *     bot.close()
 *   })
 */
export async function withTestServer<T>(
  opts: StartOptions,
  fn: (info: ServerInfo) => Promise<T>,
): Promise<T> {
  const info = await startTestServer(opts);
  try {
    return await fn(info);
  } finally {
    stopTestServer(info);
  }
}
