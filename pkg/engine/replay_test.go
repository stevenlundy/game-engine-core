package engine

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// ReplayRecord marshal / unmarshal round-trip tests
// ─────────────────────────────────────────────────────────────────────────────

func TestReplayRecord_MarshalUnmarshal_Metadata(t *testing.T) {
	t.Parallel()
	meta := SessionMetadataEntry{
		SessionID:      "s1",
		RulesetVersion: "v2.3",
		PlayerIDs:      []string{"alice", "bob"},
		StartTimeUnix:  1_700_000_000,
		Mode:           "live",
	}
	rec := NewMetadataRecord(meta)

	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got ReplayRecord
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	m, ok := got.Metadata()
	if !ok {
		t.Fatal("Metadata() returned false")
	}
	if m.SessionID != meta.SessionID {
		t.Errorf("SessionID: got %q, want %q", m.SessionID, meta.SessionID)
	}
	if m.RulesetVersion != meta.RulesetVersion {
		t.Errorf("RulesetVersion: got %q, want %q", m.RulesetVersion, meta.RulesetVersion)
	}
	if len(m.PlayerIDs) != len(meta.PlayerIDs) {
		t.Errorf("PlayerIDs len: got %d, want %d", len(m.PlayerIDs), len(meta.PlayerIDs))
	}
	if m.StartTimeUnix != meta.StartTimeUnix {
		t.Errorf("StartTimeUnix: got %d, want %d", m.StartTimeUnix, meta.StartTimeUnix)
	}
	if m.Mode != meta.Mode {
		t.Errorf("Mode: got %q, want %q", m.Mode, meta.Mode)
	}
}

func TestReplayRecord_MarshalUnmarshal_Step(t *testing.T) {
	t.Parallel()
	entry := ReplayEntry{
		StepIndex:     42,
		ActorID:       "player1",
		ActionTaken:   json.RawMessage(`{"move":"e4"}`),
		StateSnapshot: json.RawMessage(`{"board":"..."}`),
		RewardDelta:   1.5,
		IsTerminal:    false,
	}
	rec := NewStepRecord(entry)

	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got ReplayRecord
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	e, ok := got.Entry()
	if !ok {
		t.Fatal("Entry() returned false")
	}
	if e.StepIndex != entry.StepIndex {
		t.Errorf("StepIndex: got %d, want %d", e.StepIndex, entry.StepIndex)
	}
	if e.ActorID != entry.ActorID {
		t.Errorf("ActorID: got %q, want %q", e.ActorID, entry.ActorID)
	}
	if e.RewardDelta != entry.RewardDelta {
		t.Errorf("RewardDelta: got %f, want %f", e.RewardDelta, entry.RewardDelta)
	}
	if e.IsTerminal != entry.IsTerminal {
		t.Errorf("IsTerminal: got %v, want %v", e.IsTerminal, entry.IsTerminal)
	}
}

func TestReplayRecord_UnknownType(t *testing.T) {
	t.Parallel()
	data := []byte(`{"type":"bogus","entry":null}`)
	var rec ReplayRecord
	if err := json.Unmarshal(data, &rec); err == nil {
		t.Fatal("expected error for unknown type, got nil")
	}
}

func TestReplayRecord_MarshalUnknownType(t *testing.T) {
	t.Parallel()
	rec := ReplayRecord{Type: "unknown"}
	if _, err := json.Marshal(rec); err == nil {
		t.Fatal("expected error marshalling unknown type, got nil")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Writer / Reader plain (no GZIP) round-trip
// ─────────────────────────────────────────────────────────────────────────────

func TestReplayLog_PlainRoundTrip(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "test.glog")
	roundTripTest(t, path, RunModeLive, 100)
}

// ─────────────────────────────────────────────────────────────────────────────
// Writer / Reader GZIP round-trip
// ─────────────────────────────────────────────────────────────────────────────

func TestReplayLog_GZIPRoundTrip(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "test.glog")
	roundTripTest(t, path, RunModeHeadless, 100)
}

// ─────────────────────────────────────────────────────────────────────────────
// Integration test: 1,000 entries, full field fidelity
// ─────────────────────────────────────────────────────────────────────────────

// TestReplayLog_1000Entries writes 1,000 step entries (plain) and verifies
// every field survives the round-trip without corruption.
func TestReplayLog_1000Entries(t *testing.T) {
	t.Parallel()
	const N = 1000
	path := filepath.Join(t.TempDir(), "big.glog")
	roundTripTest(t, path, RunModeLive, N)
}

// TestReplayLog_1000Entries_GZIP is the GZIP variant of the integration test.
func TestReplayLog_1000Entries_GZIP(t *testing.T) {
	t.Parallel()
	const N = 1000
	path := filepath.Join(t.TempDir(), "big.glog")
	roundTripTest(t, path, RunModeHeadless, N)
}

// roundTripTest writes n entries (plus a metadata header) to path, then reads
// them all back and asserts field-for-field equality.
func roundTripTest(t *testing.T, path string, mode RunMode, n int) {
	t.Helper()

	// ── Write ──────────────────────────────────────────────────────────────
	rl, err := NewReplayLog(path, mode)
	if err != nil {
		t.Fatalf("NewReplayLog: %v", err)
	}

	wantMeta := SessionMetadataEntry{
		SessionID:      "integration-test",
		RulesetVersion: "v1.0",
		PlayerIDs:      []string{"alice", "bob"},
		StartTimeUnix:  1_700_000_000,
		Mode:           mode.String(),
	}
	if err := rl.WriteMetadata(wantMeta); err != nil {
		t.Fatalf("WriteMetadata: %v", err)
	}

	wantEntries := make([]ReplayEntry, n)
	for i := range n {
		e := ReplayEntry{
			StepIndex:     i,
			ActorID:       fmt.Sprintf("player%d", i%2),
			ActionTaken:   json.RawMessage(fmt.Sprintf(`{"step":%d}`, i)),
			StateSnapshot: json.RawMessage(fmt.Sprintf(`{"state":%d}`, i)),
			RewardDelta:   float64(i) * 0.001,
			IsTerminal:    i == n-1,
		}
		wantEntries[i] = e
		if err := rl.WriteEntry(e); err != nil {
			t.Fatalf("WriteEntry[%d]: %v", i, err)
		}
	}
	if err := rl.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// ── Read ───────────────────────────────────────────────────────────────
	rr, err := OpenReplayLog(path)
	if err != nil {
		t.Fatalf("OpenReplayLog: %v", err)
	}
	defer func() {
		if err := rr.Close(); err != nil {
			t.Errorf("ReplayReader.Close: %v", err)
		}
	}()

	gotMeta, err := rr.ReadMetadata()
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	assertMetaEqual(t, gotMeta, wantMeta)

	for i := range n {
		entry, err := rr.Next()
		if err != nil {
			t.Fatalf("Next[%d]: %v", i, err)
		}
		assertEntryEqual(t, i, entry, wantEntries[i])
	}

	// One more Next should return io.EOF.
	_, err = rr.Next()
	if err != io.EOF {
		t.Errorf("Next after last entry: got %v, want io.EOF", err)
	}
}

func assertMetaEqual(t *testing.T, got, want SessionMetadataEntry) {
	t.Helper()
	if got.SessionID != want.SessionID {
		t.Errorf("meta.SessionID: got %q, want %q", got.SessionID, want.SessionID)
	}
	if got.RulesetVersion != want.RulesetVersion {
		t.Errorf("meta.RulesetVersion: got %q, want %q", got.RulesetVersion, want.RulesetVersion)
	}
	if got.StartTimeUnix != want.StartTimeUnix {
		t.Errorf("meta.StartTimeUnix: got %d, want %d", got.StartTimeUnix, want.StartTimeUnix)
	}
	if got.Mode != want.Mode {
		t.Errorf("meta.Mode: got %q, want %q", got.Mode, want.Mode)
	}
	if len(got.PlayerIDs) != len(want.PlayerIDs) {
		t.Errorf("meta.PlayerIDs len: got %d, want %d", len(got.PlayerIDs), len(want.PlayerIDs))
	}
}

func assertEntryEqual(t *testing.T, idx int, got, want ReplayEntry) {
	t.Helper()
	if got.StepIndex != want.StepIndex {
		t.Errorf("[%d] StepIndex: got %d, want %d", idx, got.StepIndex, want.StepIndex)
	}
	if got.ActorID != want.ActorID {
		t.Errorf("[%d] ActorID: got %q, want %q", idx, got.ActorID, want.ActorID)
	}
	if got.IsTerminal != want.IsTerminal {
		t.Errorf("[%d] IsTerminal: got %v, want %v", idx, got.IsTerminal, want.IsTerminal)
	}
	// Float precision: use exact equality (JSON round-trip should be lossless
	// for values representable as 64-bit IEEE 754).
	if math.Abs(got.RewardDelta-want.RewardDelta) > 1e-12 {
		t.Errorf("[%d] RewardDelta: got %v, want %v", idx, got.RewardDelta, want.RewardDelta)
	}
	if string(got.ActionTaken) != string(want.ActionTaken) {
		t.Errorf("[%d] ActionTaken: got %s, want %s", idx, got.ActionTaken, want.ActionTaken)
	}
	if string(got.StateSnapshot) != string(want.StateSnapshot) {
		t.Errorf("[%d] StateSnapshot: got %s, want %s", idx, got.StateSnapshot, want.StateSnapshot)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Additional edge-case tests
// ─────────────────────────────────────────────────────────────────────────────

// TestReplayLog_CloseWithoutFlushIsSafe ensures Close can be called on a fresh
// log (no writes yet) without panicking or returning an error.
func TestReplayLog_CloseWithoutFlushIsSafe(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "empty.glog")
	rl, err := NewReplayLog(path, RunModeLive)
	if err != nil {
		t.Fatalf("NewReplayLog: %v", err)
	}
	if err := rl.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// TestReplayLog_NilReceiverIsSafe verifies all methods on a nil *ReplayLog are
// no-ops.
func TestReplayLog_NilReceiverIsSafe(t *testing.T) {
	t.Parallel()
	var rl *ReplayLog
	if err := rl.WriteMetadata(SessionMetadataEntry{}); err != nil {
		t.Errorf("nil WriteMetadata: %v", err)
	}
	if err := rl.WriteEntry(ReplayEntry{}); err != nil {
		t.Errorf("nil WriteEntry: %v", err)
	}
	if err := rl.Flush(); err != nil {
		t.Errorf("nil Flush: %v", err)
	}
	if err := rl.Close(); err != nil {
		t.Errorf("nil Close: %v", err)
	}
}

// TestOpenReplayLog_NonExistent verifies that opening a missing file returns
// an error.
func TestOpenReplayLog_NonExistent(t *testing.T) {
	t.Parallel()
	_, err := OpenReplayLog("/nonexistent/path/test.glog")
	if err == nil {
		t.Fatal("expected error for nonexistent path, got nil")
	}
}

// TestOpenReplayLog_AutoDetectsGZIP writes a GZIP log and confirms the reader
// auto-detects the format correctly.
func TestOpenReplayLog_AutoDetectsGZIP(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "auto.glog")

	// Write with GZIP.
	rl, err := NewReplayLog(path, RunModeHeadless)
	if err != nil {
		t.Fatalf("NewReplayLog: %v", err)
	}
	if err := rl.WriteMetadata(SessionMetadataEntry{
		SessionID: "gzip-detect",
		PlayerIDs: []string{"p1"},
		Mode:      "headless",
	}); err != nil {
		t.Fatalf("WriteMetadata: %v", err)
	}
	if err := rl.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Verify the file starts with GZIP magic bytes.
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	magic := make([]byte, 2)
	if _, err := io.ReadFull(f, magic); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	_ = f.Close()
	if magic[0] != 0x1f || magic[1] != 0x8b {
		t.Errorf("not GZIP magic: %x %x", magic[0], magic[1])
	}

	// Read back — should auto-detect.
	rr, err := OpenReplayLog(path)
	if err != nil {
		t.Fatalf("OpenReplayLog: %v", err)
	}
	meta, err := rr.ReadMetadata()
	if err != nil {
		t.Fatalf("ReadMetadata: %v", err)
	}
	if meta.SessionID != "gzip-detect" {
		t.Errorf("SessionID: got %q, want %q", meta.SessionID, "gzip-detect")
	}
	_ = rr.Close()
}

// ─────────────────────────────────────────────────────────────────────────────
// Benchmarks (6.4)
// ─────────────────────────────────────────────────────────────────────────────

// BenchmarkReplayLog_Plain measures throughput of plain (uncompressed) writes.
func BenchmarkReplayLog_Plain(b *testing.B) {
	benchmarkReplayLog(b, RunModeLive)
}

// BenchmarkReplayLog_GZIP measures throughput of GZIP-compressed writes.
func BenchmarkReplayLog_GZIP(b *testing.B) {
	benchmarkReplayLog(b, RunModeHeadless)
}

func benchmarkReplayLog(b *testing.B, mode RunMode) {
	b.Helper()
	const entriesPerRun = 10_000

	entry := ReplayEntry{
		StepIndex:     1,
		ActorID:       "player1",
		ActionTaken:   json.RawMessage(`{"move":"e2e4"}`),
		StateSnapshot: json.RawMessage(`{"board":"rnbqkbnrpppppppp................................PPPPPPPPRNBQKBNR","turn":1}`),
		RewardDelta:   0.5,
		IsTerminal:    false,
	}
	meta := SessionMetadataEntry{
		SessionID:      "bench-session",
		RulesetVersion: "v1.0",
		PlayerIDs:      []string{"p1", "p2"},
		StartTimeUnix:  1_700_000_000,
		Mode:           mode.String(),
	}

	b.ResetTimer()
	for range b.N {
		path := filepath.Join(b.TempDir(), "bench.glog")
		rl, err := NewReplayLog(path, mode)
		if err != nil {
			b.Fatalf("NewReplayLog: %v", err)
		}
		if err := rl.WriteMetadata(meta); err != nil {
			b.Fatalf("WriteMetadata: %v", err)
		}
		for range entriesPerRun {
			if err := rl.WriteEntry(entry); err != nil {
				b.Fatalf("WriteEntry: %v", err)
			}
		}
		if err := rl.Close(); err != nil {
			b.Fatalf("Close: %v", err)
		}
	}
	b.SetBytes(int64(entriesPerRun)) // report entries/op
}
