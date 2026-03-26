package engine

// This file contains a minimal stub for [ReplayLog] that satisfies all Phase 5
// forward references. Phase 6 will replace the body of this file with a full
// implementation backed by a buffered, optionally-GZIP-compressed writer.
//
// The stub is intentionally non-functional: writes are no-ops and Close is
// a no-op, so Phase 5 tests can run without a real replay-log implementation.

import "io"

// ReplayEntry describes a single step in a game session for replay/logging
// purposes. Phase 6 will extend this type with full JSON struct tags and a
// union-record envelope; for now the fields mirror the .glog schema from the
// PRD so that the runner can populate them.
type ReplayEntry struct {
	// StepIndex is the zero-based ordinal of this transition.
	StepIndex int64 `json:"step_index"`

	// ActorID is the player who submitted the action that caused this
	// transition.
	ActorID string `json:"actor_id"`

	// ActionTaken is the raw-JSON payload of the action applied this step.
	ActionTaken JSON `json:"action_taken"`

	// StateSnapshot is the raw-JSON encoding of the game State *after*
	// ApplyAction was called.
	StateSnapshot JSON `json:"state_snapshot"`

	// RewardDelta is the immediate reward received by ActorID this step.
	RewardDelta float64 `json:"reward_delta"`

	// IsTerminal is true for the final entry in a session log.
	IsTerminal bool `json:"is_terminal"`
}

// SessionMetadataEntry is the header record written as the first line of every
// .glog file. Phase 6 will add full JSON tags and the union envelope; the
// fields here are sufficient for Phase 5 to call WriteMetadata.
type SessionMetadataEntry struct {
	SessionID      string   `json:"session_id"`
	RulesetVersion string   `json:"ruleset_version"`
	PlayerIDs      []string `json:"player_ids"`
	StartTimeUnix  int64    `json:"start_time_unix"`
	Mode           string   `json:"mode"`
}

// ReplayLog is the write-side handle for a session's .glog replay file.
// The full implementation lives in Phase 6 (replay_writer.go). This stub
// satisfies the forward references in session.go, runner.go, and headless.go.
//
// All methods on a nil *ReplayLog are no-ops, allowing callers to skip a
// nil-check when cfg.ReplayPath is empty.
type ReplayLog struct {
	w io.WriteCloser // underlying writer; nil in the stub
}

// NewReplayLog opens (or creates) the file at path and returns a *ReplayLog.
// In this stub the file is not actually opened; Phase 6 replaces this with a
// real buffered/GZIP writer.
//
// The path and mode parameters are accepted so callers compile unchanged when
// Phase 6 lands.
func NewReplayLog(_ string, _ RunMode) (*ReplayLog, error) {
	// Stub: return a non-nil handle so callers can call methods on it safely.
	return &ReplayLog{}, nil
}

// WriteMetadata writes the session header as the first JSON-L line.
// Stub: no-op.
func (r *ReplayLog) WriteMetadata(_ SessionMetadataEntry) error {
	if r == nil {
		return nil
	}
	return nil
}

// WriteEntry serialises entry to JSON and appends it as a newline-terminated
// line to the log.
// Stub: no-op.
func (r *ReplayLog) WriteEntry(_ ReplayEntry) error {
	if r == nil {
		return nil
	}
	return nil
}

// Flush flushes any buffered data to the underlying writer.
// Stub: no-op.
func (r *ReplayLog) Flush() error {
	if r == nil {
		return nil
	}
	return nil
}

// Close flushes, closes the GZIP writer (if any), and closes the underlying
// file. Safe to call on a nil receiver.
// Stub: no-op.
func (r *ReplayLog) Close() error {
	if r == nil {
		return nil
	}
	if r.w != nil {
		return r.w.Close()
	}
	return nil
}
