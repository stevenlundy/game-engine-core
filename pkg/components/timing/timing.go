// Package timing provides turn-timer utilities for game sessions.
// It implements a configurable countdown timer with context cancellation support
// and a channel-based expiry signal suitable for driving AI timeout enforcement.
package timing

import (
	"context"
	"sync"
	"time"
)

// DefaultAITimeout is the maximum duration the engine will wait for an AI agent
// to return an action before falling back to a random / safe move.
const DefaultAITimeout = 50 * time.Millisecond

// TurnTimer is a one-shot countdown timer. Create one with a configured Timeout,
// call Start to begin the countdown, and read from Expired() to detect expiry.
// A TurnTimer may only be started once per lifecycle; call Stop to cancel cleanly.
//
// TurnTimer is safe for concurrent use: Start/Stop may be called from different
// goroutines, and ElapsedMs may be polled at any time.
type TurnTimer struct {
	// Timeout is the duration after which the timer expires.
	Timeout time.Duration

	mu        sync.Mutex
	startedAt time.Time
	cancel    context.CancelFunc
	expired   chan struct{}
}

// NewTurnTimer creates a TurnTimer with the given timeout duration.
func NewTurnTimer(timeout time.Duration) *TurnTimer {
	return &TurnTimer{Timeout: timeout}
}

// Start begins the countdown using the provided parent context. If the parent
// context is cancelled before the timeout, the timer is stopped cleanly and the
// Expired() channel is NOT closed. The Expired() channel is closed only when the
// full Timeout duration elapses.
//
// Start must not be called more than once on the same TurnTimer instance.
func (t *TurnTimer) Start(ctx context.Context) {
	t.mu.Lock()
	t.startedAt = time.Now()
	t.expired = make(chan struct{})
	childCtx, cancel := context.WithTimeout(ctx, t.Timeout)
	t.cancel = cancel
	expiredCh := t.expired
	t.mu.Unlock()

	go func() {
		<-childCtx.Done()
		cancel() // release resources even if already done
		if childCtx.Err() == context.DeadlineExceeded {
			// The timeout elapsed naturally — signal expiry.
			close(expiredCh)
		}
		// If the parent was cancelled or Stop() was called, we do NOT close
		// expiredCh, so callers reading Expired() will block forever (or until
		// they cancel their own select).
	}()
}

// Stop cancels the countdown without triggering the Expired() channel.
// It is safe to call Stop multiple times or before Start.
func (t *TurnTimer) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cancel != nil {
		t.cancel()
	}
}

// Expired returns a read-only channel that is closed when the timer's full
// Timeout duration has elapsed since Start was called. The channel is never
// closed if Stop is called first or if the parent context is cancelled.
//
// Callers must call Start before Expired; calling Expired before Start returns
// a nil channel (which blocks forever in a select).
func (t *TurnTimer) Expired() <-chan struct{} {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.expired
}

// ElapsedMs returns the number of milliseconds that have elapsed since Start
// was called. Returns 0 if Start has not been called yet.
func (t *TurnTimer) ElapsedMs() int64 {
	t.mu.Lock()
	started := t.startedAt
	t.mu.Unlock()
	if started.IsZero() {
		return 0
	}
	return time.Since(started).Milliseconds()
}
