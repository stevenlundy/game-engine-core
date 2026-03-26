package engine

import (
	"context"
	"encoding/json"
	"sync"
)

// ─────────────────────────────────────────────────────────────────────────────
// Spectator broadcast
// ─────────────────────────────────────────────────────────────────────────────

// SpectatorRegistry maintains a set of channels to which the runner pushes a
// [StateUpdate] after every action in Live mode. Phase 7 will wire the gRPC
// server-side streaming handler to register/deregister these channels.
//
// All operations on SpectatorRegistry are goroutine-safe.
type SpectatorRegistry struct {
	mu       sync.RWMutex
	channels map[uint64]chan<- StateUpdate
	nextID   uint64
}

// globalSpectators is the process-wide registry used by [broadcastSpectators].
// Phase 7 will replace this with a per-session registry stored on the Session
// or the gRPC server struct.
var globalSpectators = &SpectatorRegistry{
	channels: make(map[uint64]chan<- StateUpdate),
}

// Register adds ch to the registry and returns a unique token that can be
// passed to Deregister when the spectator disconnects.
func (sr *SpectatorRegistry) Register(ch chan<- StateUpdate) uint64 {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	id := sr.nextID
	sr.nextID++
	sr.channels[id] = ch
	return id
}

// Deregister removes the channel associated with token from the registry.
// The channel is NOT closed by this call; the caller is responsible for
// draining and closing it.
func (sr *SpectatorRegistry) Deregister(token uint64) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	delete(sr.channels, token)
}

// Broadcast sends update to every registered spectator channel using
// non-blocking sends. Spectators that are not ready to receive (full channel
// buffer) are skipped silently to avoid stalling the game loop.
func (sr *SpectatorRegistry) Broadcast(update StateUpdate) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	for _, ch := range sr.channels {
		select {
		case ch <- update:
		default:
			// Channel full — skip this spectator rather than block the loop.
		}
	}
}

// broadcastSpectators sends update to all registered spectators. In Headless
// mode this is a no-op (no spectators are registered, and the call returns
// immediately after the RLock/RUnlock with zero channels).
func broadcastSpectators(_ *Session, update StateUpdate) {
	globalSpectators.Broadcast(update)
}

// ─────────────────────────────────────────────────────────────────────────────
// Graceful disconnect (forfeit)
// ─────────────────────────────────────────────────────────────────────────────

// ForfeitAdapter is a [PlayerAdapter] that always returns a forfeit marker
// action. It is installed in place of a disconnected player's normal adapter
// so that the runner can continue the game and log the disconnection cleanly.
//
// The forfeit action has a Payload of `{"forfeit":true}` and its ActorID is
// set from the StateUpdate.
type ForfeitAdapter struct{}

// Ensure ForfeitAdapter satisfies PlayerAdapter at compile time.
var _ PlayerAdapter = (*ForfeitAdapter)(nil)

// RequestAction immediately returns a forfeit action with payload
// `{"forfeit":true}`. It never blocks and never returns an error.
func (f *ForfeitAdapter) RequestAction(_ context.Context, update StateUpdate) (Action, error) {
	return Action{
		ActorID: update.ActorID,
		Payload: json.RawMessage(`{"forfeit":true}`),
	}, nil
}

// markDisconnected replaces the adapter for playerID in the players map with a
// [RandomFallbackAdapter], effectively forfeiting all future turns for that
// player. This is called by the runner when a gRPC stream drops mid-game
// (Phase 7 will hook into this via the grpc stream error handler).
func markDisconnected(players map[string]PlayerAdapter, playerID string) {
	players[playerID] = NewRandomFallbackAdapter()
}
