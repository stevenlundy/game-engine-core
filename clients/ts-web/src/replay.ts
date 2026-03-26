import { ReplayEntry } from "./proto/gamesession";

/**
 * ReplayPlayer plays back a sequence of ReplayEntry objects, emitting each
 * via the onEntry callback at a configurable interval.
 */
export class ReplayPlayer {
  private entries: ReplayEntry[];
  private timerId: ReturnType<typeof setTimeout> | null = null;

  /** Called for each entry as it is played. */
  onEntry: ((entry: ReplayEntry, index: number) => void) | null = null;

  /** Called once after the last entry has been emitted. */
  onComplete: (() => void) | null = null;

  constructor(entries: ReplayEntry[]) {
    this.entries = entries;
  }

  /**
   * Begin emitting entries at the given interval (default 500 ms per step).
   * Calling play() while already playing restarts from the beginning.
   */
  play(speedMs = 500): void {
    this.stop();

    if (this.entries.length === 0) {
      this.onComplete?.();
      return;
    }

    let index = 0;

    const tick = () => {
      if (this.timerId === null) {
        // stop() was called mid-flight
        return;
      }

      const entry = this.entries[index];
      this.onEntry?.(entry, index);
      index++;

      if (index < this.entries.length) {
        this.timerId = setTimeout(tick, speedMs);
      } else {
        this.timerId = null;
        this.onComplete?.();
      }
    };

    this.timerId = setTimeout(tick, speedMs);
  }

  /** Stop playback immediately. Any in-progress interval is cleared. */
  stop(): void {
    if (this.timerId !== null) {
      clearTimeout(this.timerId);
      this.timerId = null;
    }
  }

  /**
   * Parse a newline-delimited JSON (.glog) text where each line is a JSON
   * object that maps to a ReplayEntry and return a ready-to-use ReplayPlayer.
   */
  static fromJsonLines(text: string): ReplayPlayer {
    const entries: ReplayEntry[] = text
      .split("\n")
      .map((line) => line.trim())
      .filter((line) => line.length > 0)
      .map((line) => {
        const obj = JSON.parse(line) as Record<string, unknown>;
        return {
          stepIndex: (obj["step_index"] as number | undefined) ?? 0,
          actorId: (obj["actor_id"] as string | undefined) ?? "",
          actionTaken:
            obj["action_taken"] != null
              ? typeof obj["action_taken"] === "string"
                ? new TextEncoder().encode(obj["action_taken"] as string)
                : new Uint8Array(obj["action_taken"] as number[])
              : new Uint8Array(0),
          stateSnapshot:
            obj["state_snapshot"] != null
              ? typeof obj["state_snapshot"] === "string"
                ? new TextEncoder().encode(obj["state_snapshot"] as string)
                : new Uint8Array(obj["state_snapshot"] as number[])
              : new Uint8Array(0),
          rewardDelta: (obj["reward_delta"] as number | undefined) ?? 0,
          isTerminal: (obj["is_terminal"] as boolean | undefined) ?? false,
        } satisfies ReplayEntry;
      });

    return new ReplayPlayer(entries);
  }
}
