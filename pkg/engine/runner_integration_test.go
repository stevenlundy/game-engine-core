package engine

// runner_integration_test.go — Phase 8.2 integration tests.
//
// These tests exercise Runner.Run end-to-end using countdownGame, a
// deterministic GameLogic that counts down from N to 0 and terminates after
// exactly N steps.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// countdownGame — deterministic GameLogic for integration tests
// ─────────────────────────────────────────────────────────────────────────────

// countdownState is the payload stored inside State.Payload for countdownGame.
type countdownState struct {
	Remaining int `json:"remaining"`
}

// countdownGame implements GameLogic. It initialises with a counter equal to
// Steps and terminates (IsOver = true) when the counter reaches zero.
// Each ApplyAction decrements the counter by 1 and returns reward 1.0.
// ValidateAction returns an error when the action payload contains
// {"invalid":true}.
type countdownGame struct {
	// Steps is the number of ApplyAction calls required before the game ends.
	Steps int
	// validateErrOnce, if true, causes ValidateAction to reject the FIRST
	// action it sees (to test error-recovery paths).
	validateErrOnce bool
	validated       atomic.Int64 // counts ValidateAction calls
}

func (g *countdownGame) GetInitialState(_ JSON) (State, error) {
	payload, _ := json.Marshal(countdownState{Remaining: g.Steps})
	return State{
		GameID:    "countdown",
		StepIndex: 0,
		Payload:   payload,
	}, nil
}

func (g *countdownGame) ValidateAction(_ State, a Action) error {
	call := g.validated.Add(1)
	if g.validateErrOnce && call == 1 {
		return fmt.Errorf("countdown: deliberate validation failure on first action")
	}
	if len(a.Payload) > 0 {
		var m map[string]interface{}
		if err := json.Unmarshal(a.Payload, &m); err == nil {
			if v, _ := m["invalid"].(bool); v {
				return fmt.Errorf("countdown: action flagged invalid by payload")
			}
		}
	}
	return nil
}

func (g *countdownGame) ApplyAction(state State, _ Action) (State, float64, error) {
	var cs countdownState
	_ = json.Unmarshal(state.Payload, &cs)
	cs.Remaining--
	payload, _ := json.Marshal(cs)
	return State{
		GameID:    state.GameID,
		StepIndex: state.StepIndex + 1,
		Payload:   payload,
	}, 1.0, nil
}

func (g *countdownGame) IsTerminal(state State) (TerminalResult, error) {
	var cs countdownState
	if err := json.Unmarshal(state.Payload, &cs); err != nil {
		return TerminalResult{}, fmt.Errorf("countdown: IsTerminal unmarshal: %w", err)
	}
	if cs.Remaining <= 0 {
		return TerminalResult{IsOver: true, WinnerID: "p1"}, nil
	}
	return TerminalResult{}, nil
}

func (g *countdownGame) GetRichState(state State) (interface{}, error) {
	var cs countdownState
	if err := json.Unmarshal(state.Payload, &cs); err != nil {
		return nil, err
	}
	return cs, nil
}

func (g *countdownGame) GetTensorState(state State) ([]float32, error) {
	var cs countdownState
	if err := json.Unmarshal(state.Payload, &cs); err != nil {
		return nil, err
	}
	return []float32{float32(cs.Remaining)}, nil
}

// newCountdownSession is a test helper that creates a Session backed by
// countdownGame with N steps, two players, and an optional replay path.
func newCountdownSession(t *testing.T, n int, replayPath string, mode RunMode) *Session {
	t.Helper()
	cfg := SessionConfig{
		SessionID:  fmt.Sprintf("countdown-%s", t.Name()),
		GameType:   "countdown",
		PlayerIDs:  []string{"p1", "p2"},
		Mode:       mode,
		ReplayPath: replayPath,
	}
	s, err := NewSession(cfg, &countdownGame{Steps: n})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	return s
}

// ─────────────────────────────────────────────────────────────────────────────
// 8.2 Test 1 — Live mode with two RandomFallbackAdapter players → valid .glog
// ─────────────────────────────────────────────────────────────────────────────

func TestRunner_LiveMode_TwoRandomPlayers_ProducesValidGlog(t *testing.T) {
	t.Parallel()
	const N = 6

	dir := t.TempDir()
	replayPath := filepath.Join(dir, "live.glog")

	session := newCountdownSession(t, N, replayPath, RunModeLive)

	players := map[string]PlayerAdapter{
		"p1": NewRandomFallbackAdapter(),
		"p2": NewRandomFallbackAdapter(),
	}

	runner := NewRunner()
	if err := runner.Run(context.Background(), session, players); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if session.step != N {
		t.Errorf("step = %d, want %d", session.step, N)
	}

	// Verify the .glog was created and is readable.
	info, err := os.Stat(replayPath)
	if err != nil {
		t.Fatalf("stat replay: %v", err)
	}
	if info.Size() == 0 {
		t.Error("replay log file is empty")
	}

	// Read it back and count entries.
	rr, err := OpenReplayLog(replayPath)
	if err != nil {
		t.Fatalf("OpenReplayLog: %v", err)
	}
	defer rr.Close()

	meta, err := rr.ReadMetadata()
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if meta.Mode != "live" {
		t.Errorf("meta.Mode = %q, want \"live\"", meta.Mode)
	}

	// Expect N step entries + 1 terminal entry = N+1 total.
	entryCount := 0
	lastTerminal := false
	for {
		entry, err := rr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Next entry %d: %v", entryCount, err)
		}
		entryCount++
		lastTerminal = entry.IsTerminal
	}
	// Runner writes N step entries (non-terminal) then 1 terminal entry.
	if entryCount != N+1 {
		t.Errorf("entry count = %d, want %d", entryCount, N+1)
	}
	if !lastTerminal {
		t.Error("last entry should be terminal")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 8.2 Test 2 — Headless mode produces GZIP .glog with round-trip reader
// ─────────────────────────────────────────────────────────────────────────────

func TestRunner_HeadlessMode_ProducesGZIPGlog(t *testing.T) {
	t.Parallel()
	const N = 8

	dir := t.TempDir()
	replayPath := filepath.Join(dir, "headless.glog")

	session := newCountdownSession(t, N, replayPath, RunModeHeadless)

	players := map[string]PlayerAdapter{
		"p1": NewRandomFallbackAdapter(),
		"p2": NewRandomFallbackAdapter(),
	}

	runner := NewRunner()
	if err := runner.Run(context.Background(), session, players); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Verify GZIP magic bytes.
	f, err := os.Open(replayPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	magic := make([]byte, 2)
	if _, err := io.ReadFull(f, magic); err != nil {
		t.Fatalf("read magic: %v", err)
	}
	_ = f.Close()
	if magic[0] != 0x1f || magic[1] != 0x8b {
		t.Fatalf("file is not GZIP-compressed: magic %x %x", magic[0], magic[1])
	}

	// Full round-trip read.
	rr, err := OpenReplayLog(replayPath)
	if err != nil {
		t.Fatalf("OpenReplayLog: %v", err)
	}
	defer rr.Close()

	meta, err := rr.ReadMetadata()
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if meta.Mode != "headless" {
		t.Errorf("meta.Mode = %q, want \"headless\"", meta.Mode)
	}

	entryCount := 0
	var lastEntry ReplayEntry
	for {
		entry, err := rr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Next[%d]: %v", entryCount, err)
		}
		lastEntry = entry
		entryCount++
	}
	if entryCount != N+1 {
		t.Errorf("entry count = %d, want %d", entryCount, N+1)
	}
	if !lastEntry.IsTerminal {
		t.Error("last entry should have IsTerminal=true")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 8.2 Test 3 — 50ms AI timeout: slow adapter (200ms) → fallback is used
// ─────────────────────────────────────────────────────────────────────────────

// trackingFallbackAdapter wraps RandomFallbackAdapter and counts how many
// times it was invoked so we can assert the fallback was actually used.
type trackingFallbackAdapter struct {
	inner *RandomFallbackAdapter
	calls atomic.Int64
}

func (t *trackingFallbackAdapter) RequestAction(ctx context.Context, update StateUpdate) (Action, error) {
	t.calls.Add(1)
	return t.inner.RequestAction(ctx, update)
}

func TestRunner_AITimeout_FallbackUsedAfter50ms(t *testing.T) {
	t.Parallel()
	const N = 3

	logic := &countdownGame{Steps: N}
	cfg := SessionConfig{
		SessionID: "timeout-test",
		PlayerIDs: []string{"p1"},
		Mode:      RunModeHeadless,
		AITimeout: 50 * time.Millisecond,
	}
	session, err := NewSession(cfg, logic)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// slowAdapter sleeps 200ms — always exceeds the 50ms timeout.
	slowInner := &slowAdapter{delay: 200 * time.Millisecond}
	fallback := &trackingFallbackAdapter{inner: NewRandomFallbackAdapter()}
	// Wrap: inner = slow (200ms), fallback = tracker, timeout = 50ms.
	tAdapter := NewTimeoutAdapter(slowInner, fallback, 50*time.Millisecond)

	players := map[string]PlayerAdapter{"p1": tAdapter}
	runner := NewRunner()

	start := time.Now()
	if err := runner.Run(context.Background(), session, players); err != nil {
		t.Fatalf("Run: %v", err)
	}
	elapsed := time.Since(start)

	// Each of the N steps should time out at ~50ms: total < N*150ms.
	if elapsed > time.Duration(N)*150*time.Millisecond {
		t.Errorf("Run took %v, expected well under %v (fallback should have fired)", elapsed, time.Duration(N)*150*time.Millisecond)
	}

	if fallback.calls.Load() == 0 {
		t.Error("fallback was never called — timeout did not fire")
	}

	if session.step != N {
		t.Errorf("step = %d, want %d", session.step, N)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 8.2 Test 4 — ValidateAction error recovery: runner does not crash
// ─────────────────────────────────────────────────────────────────────────────

// invalidOnceAdapter returns an action with payload {"invalid":true} exactly
// once, then returns valid null actions.
type invalidOnceAdapter struct {
	calls atomic.Int64
}

func (a *invalidOnceAdapter) RequestAction(_ context.Context, update StateUpdate) (Action, error) {
	n := a.calls.Add(1)
	if n == 1 {
		return Action{
			ActorID: update.ActorID,
			Payload: json.RawMessage(`{"invalid":true}`),
		}, nil
	}
	return Action{
		ActorID: update.ActorID,
		Payload: json.RawMessage("null"),
	}, nil
}

func TestRunner_ValidateActionError_RecoveryViaFallback(t *testing.T) {
	t.Parallel()
	const N = 4

	// countdownGame with validateErrOnce=true rejects the first action it sees.
	logic := &countdownGame{Steps: N, validateErrOnce: true}
	cfg := SessionConfig{
		SessionID: "validate-recovery",
		PlayerIDs: []string{"p1"},
		Mode:      RunModeHeadless,
	}
	session, err := NewSession(cfg, logic)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// The adapter returns a valid action every time — it's the game logic that
	// rejects the very first call. The runner must recover via fallback and
	// still complete all N steps.
	players := map[string]PlayerAdapter{"p1": NewRandomFallbackAdapter()}
	runner := NewRunner()
	if err := runner.Run(context.Background(), session, players); err != nil {
		t.Fatalf("Run should not fail due to a single validation error; got: %v", err)
	}
	if session.step != N {
		t.Errorf("step = %d, want %d", session.step, N)
	}
}

// TestRunner_ValidateActionError_InvalidPayload tests the path where the
// player adapter itself sends an invalid-payload action; the runner applies
// fallback and completes normally.
func TestRunner_ValidateActionError_InvalidPayloadAdapter(t *testing.T) {
	t.Parallel()
	const N = 3

	logic := &countdownGame{Steps: N}
	cfg := SessionConfig{
		SessionID: "invalid-payload-test",
		PlayerIDs: []string{"p1"},
		Mode:      RunModeHeadless,
	}
	session, err := NewSession(cfg, logic)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	// First action from this adapter has {"invalid":true}; subsequent ones are
	// null. countdownGame.ValidateAction rejects {"invalid":true}.
	players := map[string]PlayerAdapter{"p1": &invalidOnceAdapter{}}
	runner := NewRunner()
	if err := runner.Run(context.Background(), session, players); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if session.step != N {
		t.Errorf("step = %d, want %d", session.step, N)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 8.2 / 8.4 — Benchmarks
// ─────────────────────────────────────────────────────────────────────────────

// BenchmarkRunnerHeadless measures the time per complete game of countdownGame
// with N=100 steps in headless mode (no I/O, no logging).
func BenchmarkRunnerHeadless(b *testing.B) {
	const N = 100
	players := map[string]PlayerAdapter{
		"p1": NewRandomFallbackAdapter(),
		"p2": NewRandomFallbackAdapter(),
	}
	runner := NewRunner()

	b.ResetTimer()
	for i := range b.N {
		cfg := SessionConfig{
			SessionID: fmt.Sprintf("bench-%d", i),
			PlayerIDs: []string{"p1", "p2"},
			Mode:      RunModeHeadless,
		}
		session, err := NewSession(cfg, &countdownGame{Steps: N})
		if err != nil {
			b.Fatalf("NewSession: %v", err)
		}
		if err := runner.Run(context.Background(), session, players); err != nil {
			b.Fatalf("Run: %v", err)
		}
	}
}

// BenchmarkBatchRunner measures the BatchRunner with 1,000 concurrent headless
// countdownGame sessions (N=10 each), reporting total wall time and
// games/second via b.N iterations of the 1,000-game batch.
func BenchmarkBatchRunner(b *testing.B) {
	const (
		gamesPerBatch = 1000
		stepsPerGame  = 10
	)

	configs := make([]SessionConfig, gamesPerBatch)
	for i := range configs {
		configs[i] = SessionConfig{
			SessionID: fmt.Sprintf("batch-bench-%d", i),
			PlayerIDs: []string{"p1", "p2"},
			Mode:      RunModeHeadless,
		}
	}

	br := NewBatchRunner(8, func() GameLogic {
		return &countdownGame{Steps: stepsPerGame}
	})

	b.ResetTimer()
	for range b.N {
		results, err := br.RunAll(context.Background(), configs)
		if err != nil {
			b.Fatalf("RunAll: %v", err)
		}
		for _, r := range results {
			if r.Err != nil {
				b.Fatalf("session %s failed: %v", r.SessionID, r.Err)
			}
		}
	}
	b.ReportMetric(float64(gamesPerBatch), "games/op")
}
