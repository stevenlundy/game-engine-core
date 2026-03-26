package main_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/game-engine/game-engine-core/pkg/engine"
)

// glogtoolBin holds the path to the compiled glogtool binary, set by TestMain.
var glogtoolBin string

// TestMain builds the glogtool binary once for all tests in this package.
func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "glogtool-test-*")
	if err != nil {
		panic("failed to create temp dir: " + err.Error())
	}
	defer os.RemoveAll(tmp)

	bin := filepath.Join(tmp, "glogtool")
	out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput()
	if err != nil {
		panic("failed to build glogtool: " + err.Error() + "\n" + string(out))
	}
	glogtoolBin = bin

	os.Exit(m.Run())
}

// writeTestGlog creates a .glog file at path with the given mode,
// writes metadata + n step entries, and closes it.
func writeTestGlog(t *testing.T, path string, mode engine.RunMode, n int) {
	t.Helper()
	rl, err := engine.NewReplayLog(path, mode)
	if err != nil {
		t.Fatalf("NewReplayLog: %v", err)
	}
	meta := engine.SessionMetadataEntry{
		SessionID:      "test-session-42",
		RulesetVersion: "v1.0",
		PlayerIDs:      []string{"alice", "bob"},
		StartTimeUnix:  1_700_000_000,
		Mode:           mode.String(),
	}
	if err := rl.WriteMetadata(meta); err != nil {
		t.Fatalf("WriteMetadata: %v", err)
	}
	for i := 0; i < n; i++ {
		entry := engine.ReplayEntry{
			StepIndex:     i,
			ActorID:       "alice",
			ActionTaken:   json.RawMessage(`{"move":"e2e4"}`),
			StateSnapshot: json.RawMessage(`{"board":"start"}`),
			RewardDelta:   1.0,
			IsTerminal:    i == n-1,
		}
		if err := rl.WriteEntry(entry); err != nil {
			t.Fatalf("WriteEntry %d: %v", i, err)
		}
	}
	if err := rl.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// runGlogtool executes glogtool with the given args and returns stdout, stderr, and exit code.
func runGlogtool(args ...string) (stdout, stderr string, exitCode int) {
	cmd := exec.Command(glogtoolBin, args...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			exitCode = exit.ExitCode()
		} else {
			exitCode = -1
		}
	}
	return
}

// ─────────────────────────────────────────────────────────────────────────────
// Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestGlogtool_Inspect(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.glog")
	writeTestGlog(t, path, engine.RunModeLive, 3)

	stdout, stderr, code := runGlogtool("inspect", path)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstderr: %s", code, stderr)
	}

	// stdout must be valid JSON
	var meta map[string]any
	if err := json.Unmarshal([]byte(stdout), &meta); err != nil {
		t.Fatalf("inspect output is not valid JSON: %v\noutput: %s", err, stdout)
	}

	// Must contain the session_id we wrote
	if got, ok := meta["session_id"].(string); !ok || got != "test-session-42" {
		t.Errorf("expected session_id=%q, got %v", "test-session-42", meta["session_id"])
	}
}

func TestGlogtool_Inspect_GZIP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.glog")
	writeTestGlog(t, path, engine.RunModeHeadless, 3) // headless = GZIP

	stdout, stderr, code := runGlogtool("inspect", path)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstderr: %s", code, stderr)
	}

	var meta map[string]any
	if err := json.Unmarshal([]byte(stdout), &meta); err != nil {
		t.Fatalf("inspect output is not valid JSON: %v\noutput: %s", err, stdout)
	}
	if got, ok := meta["session_id"].(string); !ok || got != "test-session-42" {
		t.Errorf("expected session_id=%q, got %v", "test-session-42", meta["session_id"])
	}
}

func TestGlogtool_Dump(t *testing.T) {
	const nEntries = 4
	path := filepath.Join(t.TempDir(), "test.glog")
	writeTestGlog(t, path, engine.RunModeLive, nEntries)

	stdout, stderr, code := runGlogtool("dump", path)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstderr: %s", code, stderr)
	}

	// Must contain the header comment line with session_id
	if !strings.Contains(stdout, "session_id=test-session-42") {
		t.Errorf("dump output missing session_id header\noutput: %s", stdout)
	}

	// Must contain nEntries JSON objects — count opening braces at start of line
	// Each entry is printed as indented JSON starting with "{"
	count := strings.Count(stdout, "\"step_index\"")
	if count != nEntries {
		t.Errorf("expected %d entries in dump output, found %d\noutput: %s", nEntries, count, stdout)
	}

	// Must contain total entry count line
	if !strings.Contains(stdout, "total entries: 4") {
		t.Errorf("dump output missing total entries footer\noutput: %s", stdout)
	}
}

func TestGlogtool_Dump_GZIP(t *testing.T) {
	const nEntries = 3
	path := filepath.Join(t.TempDir(), "test.glog")
	writeTestGlog(t, path, engine.RunModeHeadless, nEntries)

	stdout, stderr, code := runGlogtool("dump", path)
	if code != 0 {
		t.Fatalf("expected exit 0, got %d\nstderr: %s", code, stderr)
	}
	count := strings.Count(stdout, "\"step_index\"")
	if count != nEntries {
		t.Errorf("expected %d entries in GZIP dump, found %d\noutput: %s", nEntries, count, stdout)
	}
}

func TestGlogtool_NoArgs(t *testing.T) {
	_, stderr, code := runGlogtool()
	if code == 0 {
		t.Fatal("expected non-zero exit for no args, got 0")
	}
	if !strings.Contains(stderr, "Usage") && !strings.Contains(stderr, "usage") && !strings.Contains(stderr, "glogtool") {
		t.Errorf("expected usage message in stderr, got: %s", stderr)
	}
}

func TestGlogtool_UnknownSubcommand(t *testing.T) {
	_, stderr, code := runGlogtool("badcmd", "file.glog")
	if code == 0 {
		t.Fatal("expected non-zero exit for unknown subcommand, got 0")
	}
	if !strings.Contains(stderr, "badcmd") {
		t.Errorf("expected subcommand name in stderr, got: %s", stderr)
	}
}

func TestGlogtool_BadPath(t *testing.T) {
	_, stderr, code := runGlogtool("inspect", "/nonexistent/path/file.glog")
	if code == 0 {
		t.Fatal("expected non-zero exit for bad path, got 0")
	}
	if stderr == "" {
		t.Error("expected error message in stderr, got nothing")
	}
}
