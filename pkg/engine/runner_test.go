package engine

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Runner integration tests
// ─────────────────────────────────────────────────────────────────────────────

func newHeadlessSession(t *testing.T, logic GameLogic, playerIDs []string) *Session {
	t.Helper()
	cfg := SessionConfig{
		SessionID: "test-" + t.Name(),
		PlayerIDs: playerIDs,
		Mode:      RunModeHeadless,
	}
	s, err := NewSession(cfg, logic)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	return s
}

// TestRunner_TerminatesAfterNSteps runs a stubTerminalGame with N=3 steps and
// confirms the runner exits cleanly.
func TestRunner_TerminatesAfterNSteps(t *testing.T) {
	t.Parallel()
	const N = 3
	logic := &stubTerminalGame{maxSteps: N}
	session := newHeadlessSession(t, logic, []string{"p1"})

	fb := NewRandomFallbackAdapter()
	players := map[string]PlayerAdapter{"p1": fb}

	runner := NewRunner()
	if err := runner.Run(context.Background(), session, players); err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}
	if session.step != N {
		t.Errorf("step = %d, want %d", session.step, N)
	}
}

// TestRunner_TerminatesImmediately tests a game that is terminal from the start.
func TestRunner_TerminatesImmediately(t *testing.T) {
	t.Parallel()
	logic := &stubTerminalGame{maxSteps: 0}
	session := newHeadlessSession(t, logic, []string{"p1"})

	players := map[string]PlayerAdapter{"p1": NewRandomFallbackAdapter()}
	runner := NewRunner()
	if err := runner.Run(context.Background(), session, players); err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}
	if session.step != 0 {
		t.Errorf("step = %d, want 0", session.step)
	}
}

// TestRunner_MissingAdapter verifies Run returns an error when a player has no
// adapter.
func TestRunner_MissingAdapter(t *testing.T) {
	t.Parallel()
	logic := &stubTerminalGame{maxSteps: 5}
	session := newHeadlessSession(t, logic, []string{"p1", "p2"})

	// Only provide adapter for p1, not p2.
	players := map[string]PlayerAdapter{"p1": NewRandomFallbackAdapter()}
	runner := NewRunner()
	err := runner.Run(context.Background(), session, players)
	if err == nil {
		t.Fatal("expected error for missing adapter, got nil")
	}
}

// TestRunner_NilSession verifies that passing nil returns an error.
func TestRunner_NilSession(t *testing.T) {
	t.Parallel()
	runner := NewRunner()
	err := runner.Run(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error for nil session, got nil")
	}
}

// TestRunner_ContextCancellation verifies that cancelling the context stops
// the runner.
func TestRunner_ContextCancellation(t *testing.T) {
	t.Parallel()
	// Game that never terminates.
	logic := &stubTerminalGame{maxSteps: 1_000_000}
	session := newHeadlessSession(t, logic, []string{"p1"})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	players := map[string]PlayerAdapter{"p1": NewRandomFallbackAdapter()}
	runner := NewRunner()
	err := runner.Run(ctx, session, players)
	if err == nil {
		t.Fatal("expected error from cancelled context, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("want context.DeadlineExceeded, got %v", err)
	}
}

// TestRunner_InvalidActionFallback verifies that an invalid action on the
// first call is recovered from via fallback (Headless mode).
func TestRunner_InvalidActionFallback(t *testing.T) {
	t.Parallel()
	const N = 2
	logic := &stubTerminalGame{maxSteps: N}
	session := newHeadlessSession(t, logic, []string{"p1"})

	// rejectOnceAdapter sends a rejectable action on the first call.
	players := map[string]PlayerAdapter{"p1": &rejectOnceAdapter{}}
	runner := NewRunner()
	if err := runner.Run(context.Background(), session, players); err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}
	if session.step != N {
		t.Errorf("step = %d, want %d", session.step, N)
	}
}

// TestRunner_AITimeout verifies that wrapping a slow adapter with TimeoutAdapter
// causes the fallback to be used and the game still terminates.
func TestRunner_AITimeout(t *testing.T) {
	t.Parallel()
	const N = 2
	logic := &stubTerminalGame{maxSteps: N}
	session := newHeadlessSession(t, logic, []string{"p1"})

	// Slow adapter that would take 500ms per action.
	slowInner := &slowAdapter{delay: 500 * time.Millisecond}
	fb := NewRandomFallbackAdapter()
	// 10ms timeout — should always fall back.
	wrappedAdapter := NewTimeoutAdapter(slowInner, fb, 10*time.Millisecond)

	players := map[string]PlayerAdapter{"p1": wrappedAdapter}
	runner := NewRunner()

	start := time.Now()
	if err := runner.Run(context.Background(), session, players); err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}
	elapsed := time.Since(start)
	// N steps × ~10ms timeout each → should be well under 500ms × N.
	if elapsed > time.Duration(N)*100*time.Millisecond {
		t.Errorf("Run took %v, expected fallback to keep it under %v", elapsed, time.Duration(N)*100*time.Millisecond)
	}
	if session.step != N {
		t.Errorf("step = %d, want %d", session.step, N)
	}
}

// TestRunner_TwoPlayers verifies round-robin turn assignment for two players.
func TestRunner_TwoPlayers(t *testing.T) {
	t.Parallel()
	const N = 4
	logic := &stubTerminalGame{maxSteps: N}
	session := newHeadlessSession(t, logic, []string{"p1", "p2"})

	players := map[string]PlayerAdapter{
		"p1": NewRandomFallbackAdapter(),
		"p2": NewRandomFallbackAdapter(),
	}
	runner := NewRunner()
	if err := runner.Run(context.Background(), session, players); err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}
	if session.step != N {
		t.Errorf("step = %d, want %d", session.step, N)
	}
}

// TestRunner_LiveMode verifies that the runner works in Live mode (logging
// enabled but should not panic or error).
func TestRunner_LiveMode(t *testing.T) {
	t.Parallel()
	const N = 2
	logic := &stubTerminalGame{maxSteps: N}
	cfg := SessionConfig{
		SessionID: "live-test",
		PlayerIDs: []string{"p1"},
		Mode:      RunModeLive,
	}
	session, err := NewSession(cfg, logic)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	players := map[string]PlayerAdapter{"p1": NewRandomFallbackAdapter()}
	runner := NewRunner()
	if err := runner.Run(context.Background(), session, players); err != nil {
		t.Fatalf("Run returned unexpected error: %v", err)
	}
	if session.step != N {
		t.Errorf("step = %d, want %d", session.step, N)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// errLogicGame — a GameLogic that returns errors on specific calls for error
// path testing
// ─────────────────────────────────────────────────────────────────────────────

type errLogicGame struct {
	isTerminalErr error
	applyErr      error
}

func (e *errLogicGame) GetInitialState(_ JSON) (State, error) {
	return State{GameID: "err-game"}, nil
}
func (e *errLogicGame) ValidateAction(_ State, _ Action) error { return nil }
func (e *errLogicGame) ApplyAction(_ State, _ Action) (State, float64, error) {
	if e.applyErr != nil {
		return State{}, 0, e.applyErr
	}
	return State{GameID: "err-game", StepIndex: 1}, 0, nil
}
func (e *errLogicGame) IsTerminal(_ State) (TerminalResult, error) {
	if e.isTerminalErr != nil {
		return TerminalResult{}, e.isTerminalErr
	}
	return TerminalResult{IsOver: true}, nil
}
func (e *errLogicGame) GetRichState(_ State) (interface{}, error)  { return nil, nil }
func (e *errLogicGame) GetTensorState(_ State) ([]float32, error)  { return nil, nil }

// TestRunner_IsTerminalError verifies that an error from IsTerminal propagates.
func TestRunner_IsTerminalError(t *testing.T) {
	t.Parallel()
	logic := &errLogicGame{isTerminalErr: errors.New("terminal error")}
	cfg := SessionConfig{
		SessionID: "err-terminal",
		PlayerIDs: []string{"p1"},
		Mode:      RunModeHeadless,
	}
	session, err := NewSession(cfg, logic)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	players := map[string]PlayerAdapter{"p1": NewRandomFallbackAdapter()}
	runner := NewRunner()
	if err := runner.Run(context.Background(), session, players); err == nil {
		t.Fatal("expected error from IsTerminal, got nil")
	}
}

// TestRunner_ApplyActionError verifies that an error from ApplyAction propagates.
func TestRunner_ApplyActionError(t *testing.T) {
	t.Parallel()
	// Use a game that returns not-terminal on first check but errors on apply.
	logic := &applyErrGame{}
	cfg := SessionConfig{
		SessionID: "err-apply",
		PlayerIDs: []string{"p1"},
		Mode:      RunModeHeadless,
	}
	session, err := NewSession(cfg, logic)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	players := map[string]PlayerAdapter{"p1": NewRandomFallbackAdapter()}
	runner := NewRunner()
	if err := runner.Run(context.Background(), session, players); err == nil {
		t.Fatal("expected error from ApplyAction, got nil")
	}
}

type applyErrGame struct{ called int }

func (a *applyErrGame) GetInitialState(_ JSON) (State, error) {
	return State{GameID: "apply-err"}, nil
}
func (a *applyErrGame) ValidateAction(_ State, _ Action) error { return nil }
func (a *applyErrGame) ApplyAction(_ State, _ Action) (State, float64, error) {
	return State{}, 0, errors.New("apply error")
}
func (a *applyErrGame) IsTerminal(_ State) (TerminalResult, error) {
	return TerminalResult{IsOver: false}, nil // never terminal so Run tries to ApplyAction
}
func (a *applyErrGame) GetRichState(_ State) (interface{}, error)  { return nil, nil }
func (a *applyErrGame) GetTensorState(_ State) ([]float32, error)  { return nil, nil }

// ─────────────────────────────────────────────────────────────────────────────
// Replay log integration
// ─────────────────────────────────────────────────────────────────────────────

// TestRunner_WritesReplayLog verifies that Run writes a non-nil replay log
// without error (stub implementation is a no-op, so this mainly tests wiring).
func TestRunner_WritesReplayLog(t *testing.T) {
	t.Parallel()
	const N = 3
	logic := &stubTerminalGame{maxSteps: N}
	cfg := SessionConfig{
		SessionID:  "replay-test",
		PlayerIDs:  []string{"p1"},
		Mode:       RunModeHeadless,
		ReplayPath: t.TempDir() + "/test.glog",
	}
	session, err := NewSession(cfg, logic)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if session.Log == nil {
		t.Fatal("expected non-nil ReplayLog when ReplayPath is set")
	}

	players := map[string]PlayerAdapter{"p1": NewRandomFallbackAdapter()}
	runner := NewRunner()
	if err := runner.Run(context.Background(), session, players); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// activePlayer helper tests
// ─────────────────────────────────────────────────────────────────────────────

func TestActivePlayer_RoundRobin(t *testing.T) {
	t.Parallel()
	s := &Session{Config: SessionConfig{PlayerIDs: []string{"a", "b", "c"}}}
	want := []string{"a", "b", "c", "a", "b", "c"}
	for i, w := range want {
		s.step = int64(i)
		got := activePlayer(s)
		if got != w {
			t.Errorf("step %d: activePlayer = %q, want %q", i, got, w)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// JSON Action payload stamp tests
// ─────────────────────────────────────────────────────────────────────────────

// TestRunner_StampsActorID verifies that the runner stamps ActorID when the
// adapter returns an empty one.
func TestRunner_StampsActorID(t *testing.T) {
	t.Parallel()
	logic := &captureActionGame{maxSteps: 1}
	session := newHeadlessSession(t, logic, []string{"p1"})

	// Adapter that returns an action with blank ActorID.
	players := map[string]PlayerAdapter{"p1": &blankActorAdapter{}}
	runner := NewRunner()
	if err := runner.Run(context.Background(), session, players); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if logic.lastActorID != "p1" {
		t.Errorf("ActorID not stamped: got %q, want %q", logic.lastActorID, "p1")
	}
}

type captureActionGame struct {
	maxSteps    int64
	lastActorID string
}

func (c *captureActionGame) GetInitialState(_ JSON) (State, error) {
	return State{GameID: "capture"}, nil
}
func (c *captureActionGame) ValidateAction(_ State, _ Action) error { return nil }
func (c *captureActionGame) ApplyAction(_ State, a Action) (State, float64, error) {
	c.lastActorID = a.ActorID
	return State{GameID: "capture", StepIndex: 1}, 0, nil
}
func (c *captureActionGame) IsTerminal(s State) (TerminalResult, error) {
	return TerminalResult{IsOver: s.StepIndex >= c.maxSteps}, nil
}
func (c *captureActionGame) GetRichState(_ State) (interface{}, error) { return nil, nil }
func (c *captureActionGame) GetTensorState(_ State) ([]float32, error) { return nil, nil }

type blankActorAdapter struct{}

func (b *blankActorAdapter) RequestAction(_ context.Context, _ StateUpdate) (Action, error) {
	return Action{Payload: json.RawMessage("null")}, nil
}
