import type { ReplayEntry } from "./proto/gamesession";
import { ReplayPlayer } from "./replay";

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function makeEntry(stepIndex: number, isTerminal = false): ReplayEntry {
  return {
    stepIndex,
    actorId: `player-${stepIndex}`,
    actionTaken: Buffer.alloc(0),
    stateSnapshot: Buffer.alloc(0),
    rewardDelta: stepIndex * 1.0,
    isTerminal,
  };
}

function makeEntries(count: number): ReplayEntry[] {
  return Array.from({ length: count }, (_, i) => makeEntry(i, i === count - 1));
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("ReplayPlayer", () => {
  beforeEach(() => {
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
  });

  it("emits entries in order", () => {
    const entries = makeEntries(3);
    const player = new ReplayPlayer(entries);

    const received: Array<{ entry: ReplayEntry; index: number }> = [];
    player.onEntry = (entry, index) => received.push({ entry, index });

    player.play(100);

    // advance through all three ticks
    jest.advanceTimersByTime(300);

    expect(received).toHaveLength(3);
    const [r0, r1, r2] = received;
    expect(r0?.index).toBe(0);
    expect(r1?.index).toBe(1);
    expect(r2?.index).toBe(2);
    expect(r0?.entry.stepIndex).toBe(0);
    expect(r1?.entry.stepIndex).toBe(1);
    expect(r2?.entry.stepIndex).toBe(2);
  });

  it("stop() halts playback mid-stream", () => {
    const entries = makeEntries(5);
    const player = new ReplayPlayer(entries);

    const received: number[] = [];
    player.onEntry = (_, index) => received.push(index);

    player.play(100);

    // let two ticks fire, then stop before the rest
    jest.advanceTimersByTime(200);
    player.stop();

    // advance further — nothing more should arrive
    jest.advanceTimersByTime(1000);

    expect(received).toHaveLength(2);
    expect(received).toEqual([0, 1]);
  });

  it("onComplete fires after the last entry", () => {
    const entries = makeEntries(3);
    const player = new ReplayPlayer(entries);

    const completedAt: number[] = [];
    const received: number[] = [];

    player.onEntry = (_, index) => received.push(index);
    player.onComplete = () => completedAt.push(received.length);

    player.play(100);
    jest.advanceTimersByTime(300);

    expect(completedAt).toHaveLength(1);
    // onComplete fires after all 3 entries have been emitted
    expect(completedAt[0]).toBe(3);
  });

  it("onComplete fires immediately for an empty entry list", () => {
    const player = new ReplayPlayer([]);

    let completed = false;
    player.onComplete = () => {
      completed = true;
    };

    player.play(100);

    expect(completed).toBe(true);
  });

  it("fromJsonLines parses a valid JSON-L string correctly", () => {
    const lines = [
      JSON.stringify({
        step_index: 0,
        actor_id: "agent-1",
        action_taken: "move_left",
        state_snapshot: '{"board":"..."}',
        reward_delta: 1.5,
        is_terminal: false,
      }),
      JSON.stringify({
        step_index: 1,
        actor_id: "agent-2",
        action_taken: "move_right",
        state_snapshot: '{"board":"!!!"}',
        reward_delta: -0.5,
        is_terminal: true,
      }),
    ].join("\n");

    const player = ReplayPlayer.fromJsonLines(lines);

    // play and collect
    jest.useFakeTimers();
    const received: ReplayEntry[] = [];
    player.onEntry = (e) => received.push(e);
    player.play(50);
    jest.advanceTimersByTime(100);

    expect(received).toHaveLength(2);
    const [e0, e1] = received;
    expect(e0?.stepIndex).toBe(0);
    expect(e0?.actorId).toBe("agent-1");
    expect(e0?.rewardDelta).toBe(1.5);
    expect(e0?.isTerminal).toBe(false);

    expect(e1?.stepIndex).toBe(1);
    expect(e1?.actorId).toBe("agent-2");
    expect(e1?.rewardDelta).toBe(-0.5);
    expect(e1?.isTerminal).toBe(true);
  });

  it("fromJsonLines skips blank lines", () => {
    const text =
      "\n\n" + JSON.stringify({ step_index: 0, actor_id: "x" }) + "\n\n";
    const player = ReplayPlayer.fromJsonLines(text);

    jest.useFakeTimers();
    const received: ReplayEntry[] = [];
    player.onEntry = (e) => received.push(e);
    player.play(50);
    jest.advanceTimersByTime(50);

    expect(received).toHaveLength(1);
    expect(received[0]?.stepIndex).toBe(0);
  });
});
