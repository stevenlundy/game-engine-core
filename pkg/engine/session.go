package engine

import (
	"errors"
	"time"
)

// RunMode controls whether the engine runs in human-facing Live mode or
// high-throughput Headless simulation mode.
type RunMode int

const (
	// RunModeLive enables structured slog output, human-friendly timeouts
	// (default 30 s per move), and spectator broadcast. Use this mode when
	// real players are connected over gRPC.
	RunModeLive RunMode = iota

	// RunModeHeadless suppresses all logging (DiscardHandler), forces GZIP
	// compression on the replay log, and enables the BatchRunner worker pool.
	// Use this mode for AI training and large-scale simulation.
	RunModeHeadless
)

// String returns a human-readable representation of the RunMode.
func (m RunMode) String() string {
	switch m {
	case RunModeLive:
		return "live"
	case RunModeHeadless:
		return "headless"
	default:
		return "unknown"
	}
}

// SessionConfig carries all parameters required to initialise a game session.
// It is immutable after being passed to [NewSession].
type SessionConfig struct {
	// SessionID uniquely identifies this session across the system.
	// Must be a non-empty string; the runner uses it to name the replay file
	// and as the root key in all structured log lines.
	SessionID string

	// GameType is the registered name of the game (e.g. "chess", "tic-tac-toe").
	// It is recorded in the replay metadata but does not drive any engine logic.
	GameType string

	// PlayerIDs is the ordered list of participant IDs. The runner validates
	// that every player has a corresponding [PlayerAdapter] before starting.
	PlayerIDs []string

	// InitialConfig is the game-specific JSON configuration passed verbatim to
	// [GameLogic.GetInitialState].
	InitialConfig JSON

	// Mode selects Live or Headless execution (see [RunModeLive], [RunModeHeadless]).
	Mode RunMode

	// AITimeout is the maximum duration the engine waits for an AI adapter to
	// return an action before invoking the fallback. Defaults to
	// [timing.DefaultAITimeout] (50 ms) when zero.
	AITimeout time.Duration

	// HumanTimeout is the maximum duration the engine waits for a human player
	// to return an action in Live mode before treating it as a forfeit / timeout.
	// Defaults to [DefaultHumanTimeout] (30 s) when zero.
	HumanTimeout time.Duration

	// ReplayPath is the filesystem path where the .glog replay file will be
	// written. If empty, replay writing is skipped (useful in unit tests).
	ReplayPath string
}

// DefaultHumanTimeout is the per-move deadline applied to human players in
// Live mode. After this duration elapses without a response the engine marks
// the player's turn as a forfeit.
const DefaultHumanTimeout = 30 * time.Second

// Session holds the full runtime state of a single game session: its
// configuration, the current game state, the logic implementation, the replay
// log, and a monotonically-incrementing step counter.
//
// A Session is created by [NewSession] and then handed to [Runner.Run].
// It must not be shared between goroutines while the runner is active.
type Session struct {
	// Config is the immutable configuration supplied at construction time.
	Config SessionConfig

	// State is the current (mutable) game state, updated after each successful
	// [GameLogic.ApplyAction] call.
	State State

	// Logic is the game-specific implementation driving this session.
	Logic GameLogic

	// Log is the replay log writer. It may be nil when Config.ReplayPath is
	// empty or before Phase 6 wires up the real implementation. The runner
	// checks for nil before writing.
	Log *ReplayLog

	// step is the number of actions that have been successfully applied.
	// It starts at 0 and is incremented by the runner; it mirrors
	// State.StepIndex after the first ApplyAction call.
	step int64
}

// NewSession constructs a new Session: it validates the config, calls
// [GameLogic.GetInitialState] with cfg.InitialConfig to obtain the opening
// state, and opens the replay log if cfg.ReplayPath is non-empty.
//
// Returns an error if:
//   - cfg.SessionID is empty
//   - cfg.PlayerIDs is empty
//   - logic is nil
//   - GetInitialState returns an error
func NewSession(cfg SessionConfig, logic GameLogic) (*Session, error) {
	if cfg.SessionID == "" {
		return nil, errors.New("engine: SessionConfig.SessionID must not be empty")
	}
	if len(cfg.PlayerIDs) == 0 {
		return nil, errors.New("engine: SessionConfig.PlayerIDs must not be empty")
	}
	if logic == nil {
		return nil, errors.New("engine: logic must not be nil")
	}

	// Apply defaults.
	if cfg.AITimeout == 0 {
		cfg.AITimeout = defaultAITimeoutFallback
	}
	if cfg.HumanTimeout == 0 {
		cfg.HumanTimeout = DefaultHumanTimeout
	}

	// Obtain the initial game state from the logic implementation.
	initialState, err := logic.GetInitialState(cfg.InitialConfig)
	if err != nil {
		return nil, errors.New("engine: GetInitialState failed: " + err.Error())
	}

	s := &Session{
		Config: cfg,
		State:  initialState,
		Logic:  logic,
	}

	// Open the replay log if a path was provided.
	if cfg.ReplayPath != "" {
		rl, err := NewReplayLog(cfg.ReplayPath, cfg.Mode)
		if err != nil {
			return nil, errors.New("engine: failed to open replay log: " + err.Error())
		}
		s.Log = rl
	}

	return s, nil
}

// defaultAITimeoutFallback mirrors timing.DefaultAITimeout (50 ms) without
// creating a package-level import cycle. Phase 5 callers that need the
// canonical constant should import pkg/components/timing directly.
const defaultAITimeoutFallback = 50 * time.Millisecond
