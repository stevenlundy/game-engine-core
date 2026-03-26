// Package engine defines the core types and interfaces for the game engine chassis.
// All game implementations must satisfy the [GameLogic] interface defined here.
package engine

import "encoding/json"

// JSON is a type alias for [json.RawMessage], used to represent opaque JSON payloads
// for game configs, state snapshots, and action payloads. Using a type alias (rather
// than a new named type) preserves full compatibility with the standard library's
// json.Marshaler / json.Unmarshaler semantics.
type JSON = json.RawMessage

// State is the canonical game-state envelope passed between the engine and every
// GameLogic implementation. The Payload field carries game-specific data encoded as
// raw JSON so that the engine chassis remains agnostic to the concrete game schema.
type State struct {
	// GameID uniquely identifies the game type that owns this state
	// (e.g. "chess", "tic-tac-toe"). It must never be empty.
	GameID string `json:"game_id"`

	// StepIndex is a monotonically-increasing counter incremented by the runner
	// after every successful ApplyAction call. Starts at 0 for the initial state.
	StepIndex int64 `json:"step_index"`

	// Payload is the opaque, game-specific state serialised as a JSON object.
	// GameLogic implementations own the schema of this field.
	Payload json.RawMessage `json:"payload"`
}

// Action represents a single move or decision made by one actor (human player,
// AI agent, or the environment itself). The Payload field is intentionally opaque
// so that the chassis can forward it without understanding the game schema.
type Action struct {
	// ActorID identifies the player or agent that produced this action.
	// Must match one of the player IDs registered in the session.
	ActorID string `json:"actor_id"`

	// Payload is the game-specific move data serialised as a JSON object.
	Payload json.RawMessage `json:"payload"`

	// TimestampMs is the wall-clock time at which the action was submitted,
	// expressed as milliseconds since the Unix epoch. Used for replay fidelity
	// and latency telemetry; does not affect game logic.
	TimestampMs int64 `json:"timestamp_ms"`
}

// TerminalResult is the outcome returned by [GameLogic.IsTerminal]. When IsOver
// is false the game is still in progress and WinnerID should be ignored.
type TerminalResult struct {
	// IsOver is true when the game has reached a terminal state (win, draw, or
	// any other ending condition defined by the game rules).
	IsOver bool `json:"is_over"`

	// WinnerID is the ActorID of the winning player, or an empty string for a
	// draw / no winner. This field is only meaningful when IsOver is true.
	WinnerID string `json:"winner_id,omitempty"`
}
