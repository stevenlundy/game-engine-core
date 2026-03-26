package engine

import (
	"encoding/json"
	"errors"
	"fmt"
)

// SessionMetadataEntry is the header record written as the very first line of
// every .glog file. It is encoded as a [ReplayRecord] with Type == "metadata".
type SessionMetadataEntry struct {
	// SessionID uniquely identifies the session.
	SessionID string `json:"session_id"`

	// RulesetVersion records the version string of the game ruleset in use.
	RulesetVersion string `json:"ruleset_version"`

	// PlayerIDs is the ordered list of participants.
	PlayerIDs []string `json:"player_ids"`

	// StartTimeUnix is the wall-clock start time as seconds since the Unix epoch.
	StartTimeUnix int64 `json:"start_time_unix"`

	// Mode is the human-readable run mode ("live" or "headless").
	Mode string `json:"mode"`
}

// ReplayEntry describes a single state-transition in a game session.
// It is encoded as a [ReplayRecord] with Type == "step".
type ReplayEntry struct {
	// StepIndex is the zero-based ordinal of this transition.
	StepIndex int `json:"step_index"`

	// ActorID is the player who submitted the action that caused this
	// transition.
	ActorID string `json:"actor_id"`

	// ActionTaken is the raw-JSON payload of the action applied this step.
	ActionTaken JSON `json:"action_taken"`

	// StateSnapshot is the raw-JSON encoding of the game [State] immediately
	// after [GameLogic.ApplyAction] was called for this step.
	//
	// Both regular step entries and the terminal entry use the post-apply
	// state, so this field always represents the board as the next player
	// would observe it. The terminal entry carries the final board state;
	// its ActionTaken and RewardDelta are zero-valued (no new action was
	// applied to reach the terminal condition — IsTerminal was detected on
	// the already-updated state).
	StateSnapshot JSON `json:"state_snapshot"`

	// RewardDelta is the immediate reward received by ActorID this step.
	RewardDelta float64 `json:"reward_delta"`

	// IsTerminal is true for the final entry in a session log.
	IsTerminal bool `json:"is_terminal"`
}

// ─────────────────────────────────────────────────────────────────────────────
// ReplayRecord — union/tagged envelope
// ─────────────────────────────────────────────────────────────────────────────

const (
	// RecordTypeMetadata identifies a SessionMetadataEntry record.
	RecordTypeMetadata = "metadata"

	// RecordTypeStep identifies a ReplayEntry record.
	RecordTypeStep = "step"
)

// ReplayRecord is the top-level JSON-L envelope that encodes either a
// [SessionMetadataEntry] or a [ReplayEntry] in a single stream.
//
// On the wire it looks like one of:
//
//	{"type":"metadata","metadata":{…}}
//	{"type":"step","entry":{…}}
//
// Use [NewMetadataRecord] / [NewStepRecord] to construct values, and
// [ReplayRecord.Metadata] / [ReplayRecord.Entry] to extract the payload.
type ReplayRecord struct {
	// Type is the discriminator field: "metadata" or "step".
	Type string `json:"type"`

	// rawMetadata holds the encoded SessionMetadataEntry when Type == "metadata".
	rawMetadata *SessionMetadataEntry

	// rawEntry holds the encoded ReplayEntry when Type == "step".
	rawEntry *ReplayEntry
}

// NewMetadataRecord wraps a [SessionMetadataEntry] in a [ReplayRecord].
func NewMetadataRecord(m SessionMetadataEntry) ReplayRecord {
	return ReplayRecord{Type: RecordTypeMetadata, rawMetadata: &m}
}

// NewStepRecord wraps a [ReplayEntry] in a [ReplayRecord].
func NewStepRecord(e ReplayEntry) ReplayRecord {
	return ReplayRecord{Type: RecordTypeStep, rawEntry: &e}
}

// Metadata returns the [SessionMetadataEntry] and true when Type == "metadata".
// Returns the zero value and false otherwise.
func (r ReplayRecord) Metadata() (SessionMetadataEntry, bool) {
	if r.rawMetadata == nil {
		return SessionMetadataEntry{}, false
	}
	return *r.rawMetadata, true
}

// Entry returns the [ReplayEntry] and true when Type == "step".
// Returns the zero value and false otherwise.
func (r ReplayRecord) Entry() (ReplayEntry, bool) {
	if r.rawEntry == nil {
		return ReplayEntry{}, false
	}
	return *r.rawEntry, true
}

// ─── MarshalJSON ──────────────────────────────────────────────────────────────

// wireRecord is the canonical on-wire shape used by MarshalJSON / UnmarshalJSON.
type wireRecord struct {
	Type     string                `json:"type"`
	Metadata *SessionMetadataEntry `json:"metadata,omitempty"`
	Entry    *ReplayEntry          `json:"entry,omitempty"`
}

// MarshalJSON serialises the record to its tagged JSON envelope.
func (r ReplayRecord) MarshalJSON() ([]byte, error) {
	switch r.Type {
	case RecordTypeMetadata:
		out, err := json.Marshal(wireRecord{Type: r.Type, Metadata: r.rawMetadata})
		if err != nil {
			return nil, fmt.Errorf("engine: marshal metadata record: %w", err)
		}
		return out, nil
	case RecordTypeStep:
		out, err := json.Marshal(wireRecord{Type: r.Type, Entry: r.rawEntry})
		if err != nil {
			return nil, fmt.Errorf("engine: marshal step record: %w", err)
		}
		return out, nil
	default:
		return nil, errors.New("engine: ReplayRecord has unknown type: " + r.Type)
	}
}

// UnmarshalJSON parses a tagged JSON envelope back into a [ReplayRecord].
func (r *ReplayRecord) UnmarshalJSON(data []byte) error {
	var w wireRecord
	if err := json.Unmarshal(data, &w); err != nil {
		return fmt.Errorf("engine: unmarshal replay record: %w", err)
	}
	r.Type = w.Type
	switch w.Type {
	case RecordTypeMetadata:
		if w.Metadata == nil {
			return errors.New("engine: metadata record missing 'metadata' field")
		}
		r.rawMetadata = w.Metadata
	case RecordTypeStep:
		if w.Entry == nil {
			return errors.New("engine: step record missing 'entry' field")
		}
		r.rawEntry = w.Entry
	default:
		return errors.New("engine: unknown ReplayRecord type: " + w.Type)
	}
	return nil
}
