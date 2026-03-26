package engine

import (
	"testing"
)

// Compile-time interface guard: this line fails to compile if *noopGame does not
// fully implement GameLogic, giving an immediate, descriptive error at build time.
var _ GameLogic = (*noopGame)(nil)

// assertZeroState is a helper that fails t if s is not the zero State.
func assertZeroState(t *testing.T, label string, s State) {
	t.Helper()
	if s.GameID != "" {
		t.Errorf("%s: expected empty GameID, got %q", label, s.GameID)
	}
	if s.StepIndex != 0 {
		t.Errorf("%s: expected StepIndex 0, got %d", label, s.StepIndex)
	}
	if s.Payload != nil {
		t.Errorf("%s: expected nil Payload, got %v", label, s.Payload)
	}
}

// TestNoopGame_GetInitialState confirms the method returns a zero-value State
// and does not panic.
func TestNoopGame_GetInitialState(t *testing.T) {
	t.Parallel()
	g := &noopGame{}
	state, err := g.GetInitialState(nil)
	if err != nil {
		t.Fatalf("GetInitialState returned unexpected error: %v", err)
	}
	assertZeroState(t, "GetInitialState", state)
}

// TestNoopGame_ValidateAction confirms the method returns nil and does not panic.
func TestNoopGame_ValidateAction(t *testing.T) {
	t.Parallel()
	g := &noopGame{}
	err := g.ValidateAction(State{}, Action{})
	if err != nil {
		t.Fatalf("ValidateAction returned unexpected error: %v", err)
	}
}

// TestNoopGame_ApplyAction confirms the method returns zero-value outputs and
// does not panic.
func TestNoopGame_ApplyAction(t *testing.T) {
	t.Parallel()
	g := &noopGame{}
	newState, reward, err := g.ApplyAction(State{}, Action{})
	if err != nil {
		t.Fatalf("ApplyAction returned unexpected error: %v", err)
	}
	assertZeroState(t, "ApplyAction", newState)
	if reward != 0 {
		t.Errorf("ApplyAction returned non-zero reward: %v", reward)
	}
}

// TestNoopGame_IsTerminal confirms the method reports the game is not over and
// does not panic.
func TestNoopGame_IsTerminal(t *testing.T) {
	t.Parallel()
	g := &noopGame{}
	result, err := g.IsTerminal(State{})
	if err != nil {
		t.Fatalf("IsTerminal returned unexpected error: %v", err)
	}
	if result.IsOver {
		t.Error("IsTerminal: IsOver should be false for noopGame")
	}
	if result.WinnerID != "" {
		t.Errorf("IsTerminal: WinnerID should be empty, got %q", result.WinnerID)
	}
}

// TestNoopGame_GetRichState confirms the method returns (nil, nil) and does not
// panic.
func TestNoopGame_GetRichState(t *testing.T) {
	t.Parallel()
	g := &noopGame{}
	rich, err := g.GetRichState(State{})
	if err != nil {
		t.Fatalf("GetRichState returned unexpected error: %v", err)
	}
	if rich != nil {
		t.Errorf("GetRichState returned non-nil value: %v", rich)
	}
}

// TestNoopGame_GetTensorState confirms the method returns (nil, nil) and does
// not panic.
func TestNoopGame_GetTensorState(t *testing.T) {
	t.Parallel()
	g := &noopGame{}
	tensor, err := g.GetTensorState(State{})
	if err != nil {
		t.Fatalf("GetTensorState returned unexpected error: %v", err)
	}
	if tensor != nil {
		t.Errorf("GetTensorState returned non-nil slice: %v", tensor)
	}
}
