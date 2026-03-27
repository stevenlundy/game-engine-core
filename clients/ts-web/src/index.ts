// Interactive game client (Phase 11)
export { GameWebClient } from "./client";
export type { Action, StateUpdate } from "./client";

// State helpers
export { parseRichState } from "./state";
export type { RichState } from "./state";

// Passive observer / replay SDK (unchanged)
export { ReplayPlayer } from "./replay";
export { fetchGlog } from "./fetcher";
export type { State } from "./observer";
export { parseStateSnapshot } from "./observer";
