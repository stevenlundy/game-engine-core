// Package enginepb_test is a smoke test that verifies the generated protobuf
// bindings compile correctly and that every message type can be instantiated
// and marshalled without error.
package enginepb_test

import (
	"testing"

	"google.golang.org/protobuf/proto"

	enginepb "github.com/game-engine/game-engine-core/api/proto/gen"
)

// TestInstantiateCommonMessages verifies that every message defined in
// common.proto can be created, populated, marshalled, and unmarshalled.
func TestInstantiateCommonMessages(t *testing.T) {
	t.Run("JSON", func(t *testing.T) {
		msg := &enginepb.JSON{Data: []byte(`{"key":"value"}`)}
		roundTrip(t, msg)
	})

	t.Run("State", func(t *testing.T) {
		msg := &enginepb.State{
			Payload:   []byte("opaque-state"),
			GameId:    "chess",
			StepIndex: 42,
		}
		roundTrip(t, msg)
	})

	t.Run("Action", func(t *testing.T) {
		msg := &enginepb.Action{
			ActorId:     "player-1",
			Payload:     []byte(`{"move":"e4"}`),
			TimestampMs: 1_700_000_000_000,
		}
		roundTrip(t, msg)
	})

	t.Run("StateUpdate", func(t *testing.T) {
		msg := &enginepb.StateUpdate{
			State:       &enginepb.State{GameId: "chess", StepIndex: 1},
			RewardDelta: 0.5,
			IsTerminal:  false,
			ActorId:     "player-1",
		}
		roundTrip(t, msg)
	})

	t.Run("SessionMetadata", func(t *testing.T) {
		msg := &enginepb.SessionMetadata{
			SessionId:      "sess-abc123",
			RulesetVersion: "v1.0.0",
			PlayerIds:      []string{"player-1", "player-2"},
			StartTimeUnix:  1_700_000_000,
		}
		roundTrip(t, msg)
	})
}

// TestInstantiateMatchmakingMessages verifies that every message defined in
// matchmaking.proto can be created, populated, marshalled, and unmarshalled.
func TestInstantiateMatchmakingMessages(t *testing.T) {
	t.Run("JoinRequest", func(t *testing.T) {
		msg := &enginepb.JoinRequest{
			PlayerId: "player-1",
			GameType: "chess",
			Config:   []byte(`{"time_control":"blitz"}`),
		}
		roundTrip(t, msg)
	})

	t.Run("JoinResponse", func(t *testing.T) {
		msg := &enginepb.JoinResponse{
			SessionId: "sess-abc123",
			Status:    "waiting",
			PlayerIds: []string{"player-1"},
		}
		roundTrip(t, msg)
	})

	t.Run("LobbyStatusUpdate", func(t *testing.T) {
		msg := &enginepb.LobbyStatusUpdate{
			SessionId:    "sess-abc123",
			ReadyPlayers: []string{"player-1", "player-2"},
			GameStarting: true,
		}
		roundTrip(t, msg)
	})
}

// TestInstantiateGameSessionMessages verifies that every message defined in
// gamesession.proto can be created, populated, marshalled, and unmarshalled.
func TestInstantiateGameSessionMessages(t *testing.T) {
	t.Run("StartSessionRequest", func(t *testing.T) {
		msg := &enginepb.StartSessionRequest{
			SessionId:     "sess-abc123",
			PlayerId:      "player-1",
			InitialConfig: []byte(`{"board_size":8}`),
		}
		roundTrip(t, msg)
	})

	t.Run("EndSessionRequest", func(t *testing.T) {
		msg := &enginepb.EndSessionRequest{
			SessionId: "sess-abc123",
			Reason:    "player forfeited",
		}
		roundTrip(t, msg)
	})

	t.Run("EndSessionResponse", func(t *testing.T) {
		msg := &enginepb.EndSessionResponse{
			SessionId: "sess-abc123",
			Reason:    "player forfeited",
		}
		roundTrip(t, msg)
	})

	t.Run("GetReplayRequest", func(t *testing.T) {
		msg := &enginepb.GetReplayRequest{
			SessionId: "sess-abc123",
		}
		roundTrip(t, msg)
	})

	t.Run("ReplayEntry", func(t *testing.T) {
		msg := &enginepb.ReplayEntry{
			StepIndex:     7,
			ActorId:       "player-2",
			ActionTaken:   []byte(`{"move":"d5"}`),
			StateSnapshot: []byte("compressed-state"),
			RewardDelta:   -0.1,
			IsTerminal:    false,
		}
		roundTrip(t, msg)
	})
}

// roundTrip marshals msg to wire format and unmarshals it back into a new
// message of the same type, then verifies the two messages are equal.
func roundTrip[M proto.Message](t *testing.T, msg M) {
	t.Helper()

	wire, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("proto.Marshal(%T): %v", msg, err)
	}

	got := msg.ProtoReflect().New().Interface()
	if err := proto.Unmarshal(wire, got); err != nil {
		t.Fatalf("proto.Unmarshal(%T): %v", msg, err)
	}

	if !proto.Equal(msg, got) {
		t.Errorf("round-trip mismatch:\n  want: %v\n   got: %v", msg, got)
	}
}
