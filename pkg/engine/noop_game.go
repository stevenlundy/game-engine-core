package engine

// noopGame is a minimal stub implementation of [GameLogic] used to exercise the
// runner and related infrastructure without any real game rules. Every method
// returns its zero value and a nil error, satisfying the interface contract while
// performing no actual computation.
type noopGame struct{}

// GetInitialState returns a zero-value State and no error.
func (n *noopGame) GetInitialState(_ JSON) (State, error) {
	return State{}, nil
}

// ValidateAction always considers the action valid and returns nil.
func (n *noopGame) ValidateAction(_ State, _ Action) error {
	return nil
}

// ApplyAction returns a zero-value State, zero reward, and no error.
func (n *noopGame) ApplyAction(_ State, _ Action) (State, float64, error) {
	return State{}, 0, nil
}

// IsTerminal always reports that the game is not over.
func (n *noopGame) IsTerminal(_ State) (TerminalResult, error) {
	return TerminalResult{}, nil
}

// GetRichState returns nil and no error.
func (n *noopGame) GetRichState(_ State) (interface{}, error) {
	return nil, nil
}

// GetTensorState returns a nil slice and no error.
func (n *noopGame) GetTensorState(_ State) ([]float32, error) {
	return nil, nil
}
