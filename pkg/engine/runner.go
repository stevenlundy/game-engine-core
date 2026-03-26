package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"
)

// Runner drives the main game loop for a [Session]. It is stateless: all
// mutable session data lives in the [Session] passed to Run.
//
// A single Runner may be reused across multiple sequential sessions but must
// not be shared between goroutines.
type Runner struct{}

// NewRunner creates a Runner. No options are required at this time; the
// constructor exists to keep the public API consistent as the runner grows.
func NewRunner() *Runner { return &Runner{} }

// Run executes the game loop for session until the game reaches a terminal
// state, ctx is cancelled, or an unrecoverable error occurs.
//
// players maps each player ID (matching [SessionConfig.PlayerIDs]) to its
// [PlayerAdapter]. Run returns an error if any required player adapter is
// missing.
//
// Logging behaviour:
//   - [RunModeLive]: one slog line per step at INFO level, written to the
//     default logger.
//   - [RunModeHeadless]: logging is suppressed via a [DiscardHandler].
//
// Replay writing:
//   - When session.Log is non-nil, Run writes a [SessionMetadataEntry] before
//     the first step, one [ReplayEntry] per step, a final entry with
//     IsTerminal=true, then calls Flush and Close.
func (r *Runner) Run(ctx context.Context, session *Session, players map[string]PlayerAdapter) error {
	if session == nil {
		return fmt.Errorf("engine: session must not be nil")
	}

	// Validate that every configured player has an adapter.
	for _, pid := range session.Config.PlayerIDs {
		if _, ok := players[pid]; !ok {
			return fmt.Errorf("engine: missing PlayerAdapter for player %q", pid)
		}
	}

	// Choose the slog logger: discard in Headless, default in Live.
	logger := buildLogger(session.Config.Mode)

	// Write replay metadata header.
	if session.Log != nil {
		meta := SessionMetadataEntry{
			SessionID:      session.Config.SessionID,
			RulesetVersion: "unknown", // Phase 7 will pass this via config
			PlayerIDs:      session.Config.PlayerIDs,
			StartTimeUnix:  time.Now().Unix(),
			Mode:           session.Config.Mode.String(),
		}
		if err := session.Log.WriteMetadata(meta); err != nil {
			return fmt.Errorf("engine: failed to write replay metadata: %w", err)
		}
	}

	// Broadcast the initial state to all spectators (Live mode only).
	broadcastSpectators(session, StateUpdate{
		State:      session.State,
		IsTerminal: false,
	})

	for {
		// ── 0. Check for context cancellation ───────────────────────────
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// ── 1. Check for terminal state ──────────────────────────────────
		result, err := session.Logic.IsTerminal(session.State)
		if err != nil {
			return fmt.Errorf("engine: IsTerminal error at step %d: %w", session.step, err)
		}
		if result.IsOver {
			// Write the terminal replay entry.
			if err := writeReplayEntry(session, Action{}, 0, true); err != nil {
				return err
			}
			// Flush and close the log.
			if session.Log != nil {
				if err := session.Log.Flush(); err != nil {
					logger.Error("replay log flush failed", slog.String("session_id", session.Config.SessionID), slog.Any("error", err))
				}
				if err := session.Log.Close(); err != nil {
					logger.Error("replay log close failed", slog.String("session_id", session.Config.SessionID), slog.Any("error", err))
				}
			}
			logger.Info("game over",
				slog.String("session_id", session.Config.SessionID),
				slog.String("winner_id", result.WinnerID),
				slog.Int64("steps", session.step),
			)
			return nil
		}

		// ── 2. Determine the active player ───────────────────────────────
		activePlayerID := activePlayer(session)

		adapter, ok := players[activePlayerID]
		if !ok {
			return fmt.Errorf("engine: no adapter for active player %q", activePlayerID)
		}

		// ── 3. Send StateUpdate and receive Action ────────────────────────
		update := StateUpdate{
			State:       session.State,
			RewardDelta: 0, // set accurately on the next update
			IsTerminal:  false,
			ActorID:     activePlayerID,
		}

		// Apply per-player timeout via TimeoutAdapter (already wraps the
		// adapter if the session was configured with one). Callers are
		// responsible for pre-wrapping adapters with TimeoutAdapter if
		// desired; Run itself just calls RequestAction.
		action, err := adapter.RequestAction(ctx, update)
		if err != nil {
			if ctx.Err() != nil {
				// Parent context was cancelled — propagate cleanly.
				return ctx.Err()
			}
			// Handle a disconnected / erroring adapter: forfeit in Live mode,
			// use fallback in Headless mode.
			action, err = handleAdapterError(ctx, session, activePlayerID, update, err)
			if err != nil {
				return fmt.Errorf("engine: adapter error for player %q and fallback also failed: %w", activePlayerID, err)
			}
		}

		// Stamp the actor and timestamp if the adapter left them blank.
		if action.ActorID == "" {
			action.ActorID = activePlayerID
		}
		if action.TimestampMs == 0 {
			action.TimestampMs = time.Now().UnixMilli()
		}

		// ── 4. Validate the action ────────────────────────────────────────
		var validatedAction Action
		if validateErr := session.Logic.ValidateAction(session.State, action); validateErr != nil {
			// Invalid action: re-prompt (Live) or apply fallback (Headless).
			validatedAction, err = handleInvalidAction(ctx, session, adapter, activePlayerID, update, validateErr, logger)
			if err != nil {
				return fmt.Errorf("engine: could not recover from invalid action for player %q: %w", activePlayerID, err)
			}
		} else {
			validatedAction = action
		}

		// ── 5. Apply the action ───────────────────────────────────────────
		newState, reward, err := session.Logic.ApplyAction(session.State, validatedAction)
		if err != nil {
			return fmt.Errorf("engine: ApplyAction error at step %d: %w", session.step, err)
		}
		session.State = newState

		// ── 6. Write replay entry ─────────────────────────────────────────
		if err := writeReplayEntry(session, validatedAction, reward, false); err != nil {
			return err
		}

		// ── 7. Emit step log line (Live only) ─────────────────────────────
		logger.Info("step",
			slog.String("session_id", session.Config.SessionID),
			slog.Int64("step", session.step),
			slog.String("actor_id", validatedAction.ActorID),
			slog.Float64("reward", reward),
		)

		// ── 8. Broadcast updated state to spectators ──────────────────────
		broadcastSpectators(session, StateUpdate{
			State:       session.State,
			RewardDelta: reward,
			ActorID:     activePlayerID,
		})

		// ── 9. Increment step counter ─────────────────────────────────────
		session.step++
	}
}

// ─────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────

// activePlayer returns the ID of the player whose turn it is. The default
// policy is round-robin over session.Config.PlayerIDs, indexed by the current
// step counter.
//
// Games that need custom turn order should implement it inside GameLogic and
// encode the active player in the state payload; a future runner version may
// expose a hook for this.
func activePlayer(session *Session) string {
	n := int64(len(session.Config.PlayerIDs))
	if n == 0 {
		return ""
	}
	return session.Config.PlayerIDs[session.step%n]
}

// handleAdapterError decides what to do when a PlayerAdapter returns an error.
// In Live mode the player is forfeited (a fallback action is generated and the
// session continues). In Headless mode a random fallback is used immediately.
func handleAdapterError(
	ctx context.Context,
	_ *Session,
	_ string,
	update StateUpdate,
	_ error,
) (Action, error) {
	// Future: in Live mode, mark the player as disconnected (forfeit).
	// For now, apply a random fallback action regardless of mode.
	fb := NewRandomFallbackAdapter()
	return fb.RequestAction(ctx, update)
}

// handleInvalidAction deals with a [GameLogic.ValidateAction] rejection.
//   - Live mode: re-prompt the same adapter once. If the second attempt also
//     fails validation, fall back to the random adapter.
//   - Headless mode: immediately use the random fallback adapter.
func handleInvalidAction(
	ctx context.Context,
	session *Session,
	adapter PlayerAdapter,
	playerID string,
	update StateUpdate,
	validateErr error,
	logger *slog.Logger,
) (Action, error) {
	logger.Warn("invalid action — applying fallback",
		slog.String("session_id", session.Config.SessionID),
		slog.String("player_id", playerID),
		slog.String("error", validateErr.Error()),
	)

	if session.Config.Mode == RunModeLive {
		// Re-prompt once in Live mode.
		action, err := adapter.RequestAction(ctx, update)
		if err == nil {
			if session.Logic.ValidateAction(session.State, action) == nil {
				return action, nil
			}
		}
		// Second attempt failed — use random fallback.
	}

	fb := NewRandomFallbackAdapter()
	return fb.RequestAction(ctx, update)
}

// writeReplayEntry marshals the current session state and action to a
// [ReplayEntry] and appends it to the log. It is a no-op when session.Log
// is nil.
func writeReplayEntry(session *Session, action Action, reward float64, terminal bool) error {
	if session.Log == nil {
		return nil
	}

	// Marshal the current state payload (already JSON, but encode the
	// whole State struct for the snapshot).
	stateSnap, err := json.Marshal(session.State)
	if err != nil {
		return fmt.Errorf("engine: failed to marshal state snapshot: %w", err)
	}

	entry := ReplayEntry{
		StepIndex:     int(session.step),
		ActorID:       action.ActorID,
		ActionTaken:   action.Payload,
		StateSnapshot: stateSnap,
		RewardDelta:   reward,
		IsTerminal:    terminal,
	}
	if err := session.Log.WriteEntry(entry); err != nil {
		return fmt.Errorf("engine: failed to write replay entry at step %d: %w", session.step, err)
	}
	return nil
}

// buildLogger returns a *slog.Logger appropriate for the run mode.
//   - Live:     the default slog logger (writes to stderr).
//   - Headless: a logger backed by a DiscardHandler (all output suppressed).
func buildLogger(mode RunMode) *slog.Logger {
	if mode == RunModeHeadless {
		return slog.New(NewDiscardHandler())
	}
	return slog.Default()
}
