/**
 * replayViewer.ts — example showing how to use ReplayPlayer in a browser
 * environment to log each step of a game session to the console.
 *
 * Usage (browser script tag or bundled entry point):
 *
 *   import { fetchGlog } from "game-engine-core-web";
 *   import { startReplayViewer } from "./replayViewer";
 *
 *   startReplayViewer("https://your-server/sessions/abc123.glog");
 */

import { fetchGlog } from "../src/fetcher";
import { ReplayEntry } from "../src/proto/gamesession";

/**
 * Fetch a .glog file and replay it to the browser console at 1 step/second.
 *
 * @param url     - URL of the .glog replay file.
 * @param speedMs - Milliseconds between steps (default 1000).
 */
export async function startReplayViewer(
  url: string,
  speedMs = 1000
): Promise<void> {
  console.log(`[ReplayViewer] Fetching replay from ${url} …`);

  const player = await fetchGlog(url);

  player.onEntry = (entry: ReplayEntry, index: number) => {
    console.log(
      `[ReplayViewer] Step ${index} | actor=${entry.actorId} | reward=${entry.rewardDelta} | terminal=${entry.isTerminal}`
    );
  };

  player.onComplete = () => {
    console.log("[ReplayViewer] Replay complete.");
  };

  player.play(speedMs);
}
