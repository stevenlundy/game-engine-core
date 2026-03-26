package engine

import (
	"context"
	"encoding/json"
	"math/rand"
	"time"

	"github.com/game-engine/game-engine-core/pkg/components/timing"
)

// StateUpdate is the payload sent to a player before they must submit their
// next action. It mirrors the proto StateUpdate message from Phase 2 but uses
// the engine's native Go types so that adapters do not need to import the
// generated proto package.
type StateUpdate struct {
	// State is the current game state the player must react to.
	State State

	// RewardDelta is the reward earned by the receiving player on the
	// *previous* step (0 for the first update in a session).
	RewardDelta float64

	// IsTerminal signals that the game has ended. Adapters that receive a
	// terminal update should not submit a new action.
	IsTerminal bool

	// ActorID is the player ID that is expected to act next. Adapters
	// associated with a different player may observe the update but must
	// not submit an action.
	ActorID string
}

// PlayerAdapter is the abstraction layer between the runner and a concrete
// player back-end (human over gRPC, in-process AI agent, or random fallback).
// The runner calls RequestAction once per turn for the active player.
//
// Implementations must honour context cancellation: if ctx is cancelled before
// an action is produced, RequestAction must return promptly with a non-nil error.
//
// RequestAction must be safe to call from a single goroutine; the runner never
// invokes it concurrently for the same adapter.
type PlayerAdapter interface {
	// RequestAction sends update to the player and waits for a responding
	// [Action]. The returned action's ActorID must match update.ActorID.
	//
	// Returns a non-nil error when:
	//   - ctx is cancelled or its deadline is exceeded before the player responds
	//   - the underlying transport is disconnected
	//   - any other transient failure occurs
	RequestAction(ctx context.Context, update StateUpdate) (Action, error)
}

// ─────────────────────────────────────────────
// RandomFallbackAdapter
// ─────────────────────────────────────────────

// RandomFallbackAdapter is a [PlayerAdapter] that immediately returns a
// randomly-generated (but structurally valid) action. It is used as the
// fallback when an AI timer expires or a player is disconnected.
//
// The generated action has an empty Payload (a JSON null) and its ActorID is
// set to update.ActorID. Games that need a non-null random payload should wrap
// this adapter or supply their own fallback logic.
type RandomFallbackAdapter struct {
	// rng is the source of randomness. If nil, a new source seeded from the
	// current time is created on first use.
	rng *rand.Rand
}

// NewRandomFallbackAdapter creates a RandomFallbackAdapter with an
// auto-seeded random source.
func NewRandomFallbackAdapter() *RandomFallbackAdapter {
	//nolint:gosec // non-cryptographic RNG is intentional for game simulation
	return &RandomFallbackAdapter{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// NewRandomFallbackAdapterWithRng creates a RandomFallbackAdapter with the
// supplied random source. Use this constructor in tests to get deterministic
// behaviour.
func NewRandomFallbackAdapterWithRng(rng *rand.Rand) *RandomFallbackAdapter {
	return &RandomFallbackAdapter{rng: rng}
}

// RequestAction returns a null-payload action immediately, ignoring the
// context. It never blocks and never returns an error.
func (r *RandomFallbackAdapter) RequestAction(_ context.Context, update StateUpdate) (Action, error) {
	return Action{
		ActorID:     update.ActorID,
		Payload:     json.RawMessage("null"),
		TimestampMs: time.Now().UnixMilli(),
	}, nil
}

// ─────────────────────────────────────────────
// TimeoutAdapter
// ─────────────────────────────────────────────

// TimeoutAdapter wraps any [PlayerAdapter] and enforces a hard deadline using
// a [timing.TurnTimer]. If the inner adapter does not return an action before
// the timer expires, TimeoutAdapter cancels the inner call and delegates to
// the fallback adapter.
//
// This is the primary mechanism for enforcing the 50 ms AI timeout (and the
// 30 s human timeout in Live mode).
type TimeoutAdapter struct {
	// inner is the primary adapter (AI agent, gRPC stream, etc.).
	inner PlayerAdapter

	// fallback is invoked when the inner adapter times out.
	fallback PlayerAdapter

	// timeout is the deadline applied to each RequestAction call.
	timeout time.Duration
}

// NewTimeoutAdapter wraps inner with a timeout enforced by a [timing.TurnTimer].
// If inner does not respond within timeout, fallback is invoked.
//
// Panics if inner or fallback is nil.
func NewTimeoutAdapter(inner PlayerAdapter, fallback PlayerAdapter, timeout time.Duration) *TimeoutAdapter {
	if inner == nil {
		panic("engine: TimeoutAdapter inner must not be nil")
	}
	if fallback == nil {
		panic("engine: TimeoutAdapter fallback must not be nil")
	}
	return &TimeoutAdapter{
		inner:    inner,
		fallback: fallback,
		timeout:  timeout,
	}
}

// RequestAction calls the inner adapter with a derived context that expires
// after t.timeout. If the inner adapter returns before the deadline the action
// is forwarded as-is. If the timer fires first, the fallback adapter is called
// with the original (parent) context.
func (t *TimeoutAdapter) RequestAction(ctx context.Context, update StateUpdate) (Action, error) {
	timer := timing.NewTurnTimer(t.timeout)

	// Derived context with the per-turn deadline.
	innerCtx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()

	// Start the turn timer in parallel so we get both the context cancellation
	// signal and the typed expiry channel.
	timer.Start(ctx)
	defer timer.Stop()

	type result struct {
		action Action
		err    error
	}

	ch := make(chan result, 1)
	go func() {
		a, err := t.inner.RequestAction(innerCtx, update)
		ch <- result{a, err}
	}()

	select {
	case res := <-ch:
		// Inner adapter responded in time (or with an error).
		if res.err == nil {
			return res.action, nil
		}
		// Inner returned an error — fall through to fallback.
	case <-timer.Expired():
		// Timer fired: inner was too slow.
	case <-ctx.Done():
		// Parent context cancelled: propagate cancellation.
		return Action{}, ctx.Err()
	}

	// Invoke the fallback using the parent context (not the expired inner ctx).
	return t.fallback.RequestAction(ctx, update)
}
