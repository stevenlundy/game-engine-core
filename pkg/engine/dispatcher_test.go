package engine

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Test helpers shared across dispatcher_test.go, runner_test.go, etc.
// ─────────────────────────────────────────────────────────────────────────────

// stubTerminalGame is a [GameLogic] implementation that terminates after
// exactly maxSteps successful ApplyAction calls. It is used for deterministic
// runner integration tests.
type stubTerminalGame struct {
	maxSteps int64
}

func (g *stubTerminalGame) GetInitialState(_ JSON) (State, error) {
	return State{GameID: "stub", StepIndex: 0, Payload: json.RawMessage(`{"step":0}`)}, nil
}

func (g *stubTerminalGame) ValidateAction(_ State, a Action) error {
	if a.Payload == nil {
		return nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(a.Payload, &m); err != nil {
		return nil // null payload is OK
	}
	if reject, _ := m["reject"].(bool); reject {
		return errors.New("stub: action rejected by test")
	}
	return nil
}

func (g *stubTerminalGame) ApplyAction(state State, _ Action) (State, float64, error) {
	next := State{
		GameID:    state.GameID,
		StepIndex: state.StepIndex + 1,
		Payload:   json.RawMessage(`{"step":` + string(rune('0'+state.StepIndex+1)) + `}`),
	}
	return next, 1.0, nil
}

func (g *stubTerminalGame) IsTerminal(state State) (TerminalResult, error) {
	if state.StepIndex >= g.maxSteps {
		return TerminalResult{IsOver: true, WinnerID: "p1"}, nil
	}
	return TerminalResult{}, nil
}

func (g *stubTerminalGame) GetRichState(_ State) (interface{}, error) { return nil, nil }
func (g *stubTerminalGame) GetTensorState(_ State) ([]float32, error) { return nil, nil }

// slowAdapter is a PlayerAdapter that sleeps for d before returning.
type slowAdapter struct {
	delay  time.Duration
	called int
}

func (s *slowAdapter) RequestAction(ctx context.Context, update StateUpdate) (Action, error) {
	s.called++
	select {
	case <-time.After(s.delay):
		return Action{ActorID: update.ActorID, Payload: json.RawMessage("null")}, nil
	case <-ctx.Done():
		return Action{}, ctx.Err() //nolint:wrapcheck // ctx.Err() returns sentinels; wrapping breaks errors.Is
	}
}

// rejectOnceAdapter returns a rejectable action on the first call, then a
// valid null action on subsequent calls.
type rejectOnceAdapter struct {
	calls int
}

func (r *rejectOnceAdapter) RequestAction(_ context.Context, update StateUpdate) (Action, error) {
	r.calls++
	if r.calls == 1 {
		return Action{
			ActorID: update.ActorID,
			Payload: json.RawMessage(`{"reject":true}`),
		}, nil
	}
	return Action{ActorID: update.ActorID, Payload: json.RawMessage("null")}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Dispatcher tests
// ─────────────────────────────────────────────────────────────────────────────

// TestRandomFallbackAdapter_ReturnsImmediately checks that RequestAction never
// blocks and returns an action with the correct ActorID.
func TestRandomFallbackAdapter_ReturnsImmediately(t *testing.T) {
	t.Parallel()
	fb := NewRandomFallbackAdapter()
	update := StateUpdate{ActorID: "player1"}

	start := time.Now()
	action, err := fb.RequestAction(context.Background(), update)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action.ActorID != "player1" {
		t.Errorf("ActorID = %q, want %q", action.ActorID, "player1")
	}
	if elapsed > 5*time.Millisecond {
		t.Errorf("RequestAction took %v, expected near-instant", elapsed)
	}
	if string(action.Payload) != "null" {
		t.Errorf("Payload = %s, want null", action.Payload)
	}
}

// TestTimeoutAdapter_InnerRespondsInTime verifies that the inner adapter's
// action is forwarded when it returns before the deadline.
func TestTimeoutAdapter_InnerRespondsInTime(t *testing.T) {
	t.Parallel()
	slow := &slowAdapter{delay: 5 * time.Millisecond}
	fb := NewRandomFallbackAdapter()
	ta := NewTimeoutAdapter(slow, fb, 100*time.Millisecond)

	update := StateUpdate{ActorID: "ai"}
	action, err := ta.RequestAction(context.Background(), update)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action.ActorID != "ai" {
		t.Errorf("expected actor %q, got %q", "ai", action.ActorID)
	}
	if slow.called != 1 {
		t.Errorf("inner adapter called %d times, want 1", slow.called)
	}
}

// TestTimeoutAdapter_FallbackOnExpiry verifies that the fallback is used when
// the inner adapter is slower than the timeout.
func TestTimeoutAdapter_FallbackOnExpiry(t *testing.T) {
	t.Parallel()
	slow := &slowAdapter{delay: 200 * time.Millisecond}
	fb := NewRandomFallbackAdapter()
	ta := NewTimeoutAdapter(slow, fb, 20*time.Millisecond)

	update := StateUpdate{ActorID: "ai"}
	start := time.Now()
	action, err := ta.RequestAction(context.Background(), update)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action.ActorID != "ai" {
		t.Errorf("ActorID = %q, want %q", "ai", action.ActorID)
	}
	// Should have returned in roughly the timeout duration, not the slow delay.
	if elapsed > 100*time.Millisecond {
		t.Errorf("took %v, expected fallback within ~20ms", elapsed)
	}
}

// TestTimeoutAdapter_ContextCancellation verifies that cancelling the parent
// context propagates the error instead of using the fallback.
func TestTimeoutAdapter_ContextCancellation(t *testing.T) {
	t.Parallel()
	slow := &slowAdapter{delay: 500 * time.Millisecond}
	fb := NewRandomFallbackAdapter()
	ta := NewTimeoutAdapter(slow, fb, 200*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := ta.RequestAction(ctx, StateUpdate{ActorID: "ai"})
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
}

// TestTimeoutAdapter_PanicsOnNilInner checks the constructor panics.
func TestTimeoutAdapter_PanicsOnNilInner(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil inner, but did not panic")
		}
	}()
	NewTimeoutAdapter(nil, NewRandomFallbackAdapter(), time.Second)
}

// TestTimeoutAdapter_PanicsOnNilFallback checks the constructor panics.
func TestTimeoutAdapter_PanicsOnNilFallback(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for nil fallback, but did not panic")
		}
	}()
	NewTimeoutAdapter(NewRandomFallbackAdapter(), nil, time.Second)
}
