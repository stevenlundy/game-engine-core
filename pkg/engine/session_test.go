package engine

import (
	"testing"
	"time"
)

// TestSessionConfig_Defaults verifies that NewSession fills in zero-valued
// AITimeout and HumanTimeout with the package defaults.
func TestSessionConfig_Defaults(t *testing.T) {
	t.Parallel()
	cfg := SessionConfig{
		SessionID: "test-defaults",
		PlayerIDs: []string{"p1"},
		Mode:      RunModeHeadless,
		// AITimeout and HumanTimeout intentionally left zero
	}
	s, err := NewSession(cfg, &noopGame{})
	if err != nil {
		t.Fatalf("NewSession returned unexpected error: %v", err)
	}
	if s.Config.AITimeout != defaultAITimeoutFallback {
		t.Errorf("AITimeout: got %v, want %v", s.Config.AITimeout, defaultAITimeoutFallback)
	}
	if s.Config.HumanTimeout != DefaultHumanTimeout {
		t.Errorf("HumanTimeout: got %v, want %v", s.Config.HumanTimeout, DefaultHumanTimeout)
	}
}

// TestNewSession_ValidationErrors checks each validation guard in NewSession.
func TestNewSession_ValidationErrors(t *testing.T) {
	t.Parallel()
	base := SessionConfig{
		SessionID: "s1",
		PlayerIDs: []string{"p1"},
		Mode:      RunModeHeadless,
	}

	tests := []struct {
		name    string
		mutate  func(*SessionConfig)
		logic   GameLogic
		wantErr bool
	}{
		{
			name:    "missing session ID",
			mutate:  func(c *SessionConfig) { c.SessionID = "" },
			logic:   &noopGame{},
			wantErr: true,
		},
		{
			name:    "empty player IDs",
			mutate:  func(c *SessionConfig) { c.PlayerIDs = nil },
			logic:   &noopGame{},
			wantErr: true,
		},
		{
			name:    "nil logic",
			mutate:  func(_ *SessionConfig) {},
			logic:   nil,
			wantErr: true,
		},
		{
			name:    "valid config",
			mutate:  func(_ *SessionConfig) {},
			logic:   &noopGame{},
			wantErr: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := base
			tc.mutate(&cfg)
			_, err := NewSession(cfg, tc.logic)
			if (err != nil) != tc.wantErr {
				t.Errorf("NewSession() error = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}
}

// TestRunMode_String verifies the String method for each RunMode constant.
func TestRunMode_String(t *testing.T) {
	t.Parallel()
	tests := []struct {
		mode RunMode
		want string
	}{
		{RunModeLive, "live"},
		{RunModeHeadless, "headless"},
		{RunMode(99), "unknown"},
	}
	for _, tc := range tests {
		if got := tc.mode.String(); got != tc.want {
			t.Errorf("RunMode(%d).String() = %q, want %q", tc.mode, got, tc.want)
		}
	}
}

// TestDefaultHumanTimeout verifies the 30 s constant.
func TestDefaultHumanTimeout(t *testing.T) {
	t.Parallel()
	if DefaultHumanTimeout != 30*time.Second {
		t.Errorf("DefaultHumanTimeout = %v, want 30s", DefaultHumanTimeout)
	}
}

// TestNewSession_InitialState verifies that the session's State is populated
// from GetInitialState.
func TestNewSession_InitialState(t *testing.T) {
	t.Parallel()
	logic := &stubTerminalGame{maxSteps: 3}
	cfg := SessionConfig{
		SessionID: "init-state",
		PlayerIDs: []string{"p1"},
		Mode:      RunModeHeadless,
	}
	s, err := NewSession(cfg, logic)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if s.State.GameID != "stub" {
		t.Errorf("State.GameID = %q, want %q", s.State.GameID, "stub")
	}
	if s.step != 0 {
		t.Errorf("initial step = %d, want 0", s.step)
	}
}
