// Interactive game client (Phase 11)

export type { Action, StateUpdate } from "./client";
export { GameWebClient } from "./client";
export { fetchGlog } from "./fetcher";
export type { State } from "./observer";
export { parseStateSnapshot } from "./observer";
// Passive observer / replay SDK (unchanged)
export { ReplayPlayer } from "./replay";
export type { RichState } from "./state";
// State helpers
export { parseRichState } from "./state";
