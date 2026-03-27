/**
 * Integration tests: TypeScript GameClient ↔ Go gRPC server (cmd/testserver).
 *
 * These tests spin up a real Go server binary and exercise the full stack:
 *   TypeScript proto serialisation → gRPC transport → game logic
 *
 * They are skipped automatically if the 'go' binary is not on PATH, so they
 * never block CI environments without Go installed.
 *
 * Structure mirrors the Python integration tests in
 *   clients/python/tests/integration/test_integration.py
 */

import { execFileSync } from "child_process";
import { GameClient } from "../client.js";
import { drawCard } from "../actions.js";
import type { Action, StateUpdate } from "../proto/common.js";
import {
  startTestServer,
  stopTestServer,
  withTestServer,
  type ServerInfo,
} from "../testing/server.js";

// ---------------------------------------------------------------------------
// Skip the entire suite if 'go' is not on PATH
// ---------------------------------------------------------------------------

function goIsAvailable(): boolean {
  try {
    execFileSync("go", ["version"], { stdio: "ignore" });
    return true;
  } catch {
    return false;
  }
}

const GO_AVAILABLE = goIsAvailable();

// Use describe.skip when 'go' is unavailable so all tests are gracefully skipped.
const describeIntegration = GO_AVAILABLE ? describe : describe.skip;

// ---------------------------------------------------------------------------
// Minimal bot implementations used across tests
// ---------------------------------------------------------------------------

/** Bot that always draws a card — simplest possible agent. */
class AlwaysDrawBot extends GameClient {
  onStateUpdate(_update: StateUpdate): Action {
    return drawCard();
  }
}

/** Bot that records every StateUpdate it receives, then draws. */
class RecordingBot extends GameClient {
  public readonly updates: StateUpdate[] = [];

  onStateUpdate(update: StateUpdate): Action {
    this.updates.push(update);
    return drawCard();
  }
}

// ---------------------------------------------------------------------------
// Shared server — started once for the whole suite with countdownSteps=5
// ---------------------------------------------------------------------------

let sharedServer: ServerInfo;

beforeAll(async () => {
  if (!GO_AVAILABLE) return;
  sharedServer = await startTestServer({ countdownSteps: 5 });
}, 60_000 /* allow time for go build */);

afterAll(() => {
  if (!GO_AVAILABLE || !sharedServer) return;
  stopTestServer(sharedServer);
});

// ---------------------------------------------------------------------------
// Basic connection
// ---------------------------------------------------------------------------

describeIntegration("Basic connection", () => {
  it("run completes without error", async () => {
    const bot = new AlwaysDrawBot(sharedServer.url, "p1");
    try {
      await bot.run();
    } finally {
      bot.close();
    }
  });

  it("bot receives state updates", async () => {
    const bot = new RecordingBot(sharedServer.url, "p1");
    try {
      await bot.run();
    } finally {
      bot.close();
    }
    expect(bot.updates.length).toBeGreaterThan(0);
  });

  it("last update is terminal", async () => {
    const bot = new RecordingBot(sharedServer.url, "p1");
    try {
      await bot.run();
    } finally {
      bot.close();
    }
    const last = bot.updates[bot.updates.length - 1];
    expect(last.isTerminal).toBe(true);
  });

  it("step count matches countdown", async () => {
    const bot = new RecordingBot(sharedServer.url, "p1");
    try {
      await bot.run();
    } finally {
      bot.close();
    }
    const nonTerminal = bot.updates.filter((u) => !u.isTerminal);
    expect(nonTerminal).toHaveLength(sharedServer.countdownSteps);
  });

  it("rewards are non-negative", async () => {
    const bot = new RecordingBot(sharedServer.url, "p1");
    try {
      await bot.run();
    } finally {
      bot.close();
    }
    for (const update of bot.updates) {
      expect(update.rewardDelta).toBeGreaterThanOrEqual(0);
    }
  });

  it("rewards are positive after step 0", async () => {
    const bot = new RecordingBot(sharedServer.url, "p1");
    try {
      await bot.run();
    } finally {
      bot.close();
    }
    const postAction = bot.updates.filter(
      (u) => (u.state?.stepIndex ?? 0) > 0
    );
    expect(postAction.length).toBeGreaterThan(0);
    for (const update of postAction) {
      expect(update.rewardDelta).toBeGreaterThan(0);
    }
  });

  it("step indices are monotonically increasing", async () => {
    const bot = new RecordingBot(sharedServer.url, "p1");
    try {
      await bot.run();
    } finally {
      bot.close();
    }
    const indices = bot.updates.map((u) => u.state?.stepIndex ?? 0);
    for (let i = 1; i < indices.length; i++) {
      expect(indices[i]).toBeGreaterThan(indices[i - 1]);
    }
  });
});

// ---------------------------------------------------------------------------
// State payload
// ---------------------------------------------------------------------------

describeIntegration("State payload", () => {
  it("payload is valid JSON", async () => {
    const bot = new RecordingBot(sharedServer.url, "p1");
    try {
      await bot.run();
    } finally {
      bot.close();
    }
    for (const update of bot.updates) {
      const raw = update.state?.payload;
      if (raw && raw.length > 0) {
        const text = Buffer.from(raw).toString("utf8");
        expect(() => JSON.parse(text)).not.toThrow();
      }
    }
  });

  it("game_id is set", async () => {
    const bot = new RecordingBot(sharedServer.url, "p1");
    try {
      await bot.run();
    } finally {
      bot.close();
    }
    for (const update of bot.updates) {
      expect(update.state?.gameId).toBeTruthy();
    }
  });
});

// ---------------------------------------------------------------------------
// Custom step count
// ---------------------------------------------------------------------------

describeIntegration("Custom step count", () => {
  it(
    "3-step game produces exactly 3 non-terminal updates",
    async () => {
      let updates: StateUpdate[] = [];
      await withTestServer({ countdownSteps: 3 }, async (info) => {
        const bot = new RecordingBot(info.url, "p1");
        try {
          await bot.run();
        } finally {
          bot.close();
        }
        updates = bot.updates;
      });
      const nonTerminal = updates.filter((u) => !u.isTerminal);
      expect(nonTerminal).toHaveLength(3);
    },
    60_000
  );

  it(
    "1-step game produces 1 non-terminal then a terminal",
    async () => {
      let updates: StateUpdate[] = [];
      await withTestServer({ countdownSteps: 1 }, async (info) => {
        const bot = new RecordingBot(info.url, "p1");
        try {
          await bot.run();
        } finally {
          bot.close();
        }
        updates = bot.updates;
      });
      expect(updates.length).toBeGreaterThanOrEqual(1);
      expect(updates[updates.length - 1].isTerminal).toBe(true);
    },
    60_000
  );
});

// ---------------------------------------------------------------------------
// Actor id
// ---------------------------------------------------------------------------

describeIntegration("Actor id", () => {
  it("actor_id matches player_id", async () => {
    const bot = new RecordingBot(sharedServer.url, "integration-tester");
    try {
      await bot.run();
    } finally {
      bot.close();
    }
    for (const update of bot.updates) {
      if (!update.isTerminal) {
        expect(update.actorId).toBe("integration-tester");
      }
    }
  });
});
