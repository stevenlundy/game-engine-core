package timing

import (
	"context"
	"testing"
	"time"
)

// TestDefaultAITimeout checks the package-level constant is exactly 50ms.
func TestDefaultAITimeout(t *testing.T) {
	if DefaultAITimeout != 50*time.Millisecond {
		t.Errorf("DefaultAITimeout = %v, want 50ms", DefaultAITimeout)
	}
}

// TestTurnTimer_Fires confirms that a 10ms timer fires within ±5ms.
// Allowed window: [5ms, 15ms] after Start.
func TestTurnTimer_Fires(t *testing.T) {
	const timeout = 10 * time.Millisecond
	const tolerance = 5 * time.Millisecond

	timer := NewTurnTimer(timeout)
	start := time.Now()
	timer.Start(context.Background())

	select {
	case <-timer.Expired():
		elapsed := time.Since(start)
		lo := timeout - tolerance
		hi := timeout + tolerance
		if elapsed < lo || elapsed > hi {
			t.Errorf("Expired fired at %v, want within [%v, %v]", elapsed, lo, hi)
		}
	case <-time.After(timeout + tolerance + 10*time.Millisecond):
		t.Fatal("timer did not fire within the expected window")
	}
}

// TestTurnTimer_Cancelled confirms that stopping a timer prevents the
// Expired() channel from ever firing.
func TestTurnTimer_Cancelled(t *testing.T) {
	const timeout = 50 * time.Millisecond
	timer := NewTurnTimer(timeout)
	timer.Start(context.Background())

	// Stop immediately — well before the timeout elapses.
	timer.Stop()

	// Give it a generous window: if the channel fires within 2×timeout, the
	// test fails. A stopped timer must never close the expired channel.
	select {
	case <-timer.Expired():
		t.Fatal("Expired() fired after Stop() — timer was not cancelled")
	case <-time.After(2 * timeout):
		// Correct: channel did not fire.
	}
}

// TestTurnTimer_StopBeforeStart verifies Stop is safe to call before Start.
func TestTurnTimer_StopBeforeStart(t *testing.T) {
	timer := NewTurnTimer(10 * time.Millisecond)
	timer.Stop() // must not panic
}

// TestTurnTimer_ElapsedMs checks that ElapsedMs grows over time.
func TestTurnTimer_ElapsedMs(t *testing.T) {
	timer := NewTurnTimer(200 * time.Millisecond)
	if timer.ElapsedMs() != 0 {
		t.Error("ElapsedMs should be 0 before Start")
	}
	timer.Start(context.Background())
	time.Sleep(20 * time.Millisecond)
	elapsed := timer.ElapsedMs()
	if elapsed < 15 || elapsed > 50 {
		t.Errorf("ElapsedMs = %d, expected roughly 20ms", elapsed)
	}
	timer.Stop()
}

// TestTurnTimer_ParentContextCancelled verifies that cancelling the parent
// context before the timeout does NOT close the Expired() channel.
func TestTurnTimer_ParentContextCancelled(t *testing.T) {
	const timeout = 100 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	timer := NewTurnTimer(timeout)
	timer.Start(ctx)

	// Cancel the parent context immediately.
	cancel()

	select {
	case <-timer.Expired():
		t.Fatal("Expired() fired when parent context was cancelled (not a deadline exceeded)")
	case <-time.After(timeout + 20*time.Millisecond):
		// Correct: no expiry signal.
	}
}
