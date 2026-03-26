package engine

// GameLogic is the single interface every game implementation must satisfy.
// The engine chassis calls these methods in a well-defined order (see the runner
// in Phase 5) and never inspects the raw JSON payloads itself — all game-specific
// reasoning lives exclusively inside the GameLogic implementation.
//
// Implementations must be safe for sequential use by a single goroutine; the
// runner serialises all calls for a given session. Implementations must not
// retain references to State or Action values after a method returns, because
// the engine may reuse or mutate the underlying byte slices.
type GameLogic interface {
	// GetInitialState creates and returns the starting State for a new game
	// session. config is a JSON-encoded object whose schema is defined by the
	// concrete game (number of players, board size, rule variants, etc.).
	//
	// Returns a non-nil error if config is malformed or contains values that
	// violate the game's constraints. On success the returned State must have a
	// non-empty GameID and StepIndex equal to 0.
	GetInitialState(config JSON) (State, error)

	// ValidateAction checks whether action is a legal move given the current
	// state. It must be a pure read-only operation: state must not be mutated,
	// and the method must produce no side-effects.
	//
	// Returns nil if the action is legal. Returns a descriptive non-nil error
	// (e.g. "square already occupied", "actor not the active player") if the
	// action is illegal. Returning an error does NOT automatically terminate the
	// session; the runner decides how to handle invalid actions (re-prompt or
	// apply fallback).
	ValidateAction(state State, action Action) error

	// ApplyAction applies a validated action to state and returns the resulting
	// new state, the immediate reward signal for the acting player, and any
	// error. It is the caller's responsibility to call ValidateAction first;
	// ApplyAction behaviour is undefined for invalid actions.
	//
	// newState must be a freshly-allocated value; it must not alias state.
	// reward may be zero, positive, or negative depending on game semantics.
	// Returns a non-nil error only for unexpected internal failures (e.g. JSON
	// serialisation errors); legal game transitions must not return errors.
	ApplyAction(state State, action Action) (newState State, reward float64, err error)

	// IsTerminal inspects state and reports whether the game has ended. It must
	// be a pure read-only operation with no side-effects.
	//
	// When the returned TerminalResult.IsOver is true, WinnerID must be set to
	// the winning actor's ID, or left empty to represent a draw. Returns a
	// non-nil error only if state is structurally invalid and cannot be decoded.
	IsTerminal(state State) (TerminalResult, error)

	// GetRichState returns a human-readable or UI-friendly representation of
	// state. The return type is interface{} so that each game may use its own
	// concrete struct; callers typically JSON-encode the result for display or
	// streaming to spectators.
	//
	// Must not mutate state. Returns a non-nil error if state cannot be decoded.
	// Returning nil, nil is valid for games that have no rich representation.
	GetRichState(state State) (interface{}, error)

	// GetTensorState returns a flat float32 slice encoding state as a numeric
	// feature vector suitable for consumption by machine-learning agents. The
	// length and semantics of the slice are defined by the concrete game and
	// must remain stable across versions for a given GameID.
	//
	// Must not mutate state. Returns a non-nil error if state cannot be decoded.
	// Returns a nil slice (not an error) when the game does not support tensor
	// encoding.
	GetTensorState(state State) ([]float32, error)
}
