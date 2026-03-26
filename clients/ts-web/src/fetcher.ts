import { ReplayPlayer } from "./replay";

/**
 * Fetch a .glog file from the given URL, parse its newline-delimited JSON
 * content, and return a configured ReplayPlayer.
 *
 * Uses the browser Fetch API — no Node.js dependencies.
 *
 * @param url - Absolute or relative URL pointing to a .glog resource.
 * @throws {Error} if the HTTP response status is not OK (2xx).
 */
export async function fetchGlog(url: string): Promise<ReplayPlayer> {
  const response = await fetch(url);

  if (!response.ok) {
    throw new Error(
      `fetchGlog: request failed with status ${response.status} ${response.statusText}`
    );
  }

  const text = await response.text();
  return ReplayPlayer.fromJsonLines(text);
}
