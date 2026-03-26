package engine

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// gzipMagic are the two identifying bytes at the start of every GZIP stream
// (RFC 1952 §2.3.1).
var gzipMagic = [2]byte{0x1f, 0x8b}

// ReplayReader is the read-side handle for a .glog replay file. It supports
// both plain and GZIP-compressed files, auto-detected from the file header.
//
// Call [OpenReplayLog] to obtain a *ReplayReader, then:
//  1. Call [ReplayReader.ReadMetadata] once to consume the header line.
//  2. Call [ReplayReader.Next] in a loop until it returns [io.EOF].
//  3. Call [ReplayReader.Close] to release resources.
type ReplayReader struct {
	file *os.File
	gz   *gzip.Reader // non-nil for GZIP files
	sc   *bufio.Scanner
}

// OpenReplayLog opens the .glog file at path and returns a *ReplayReader.
// It peeks at the first two bytes to detect GZIP compression and transparently
// wraps the file with a [gzip.Reader] when the magic bytes are present.
//
// The caller must call [ReplayReader.Close] when done.
func OpenReplayLog(path string) (*ReplayReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("engine: OpenReplayLog open %q: %w", path, err)
	}

	// Peek at the first two bytes to detect GZIP.
	magic := make([]byte, 2)
	n, err := io.ReadFull(f, magic)
	if err != nil && n == 0 {
		// Empty file — not a valid replay, but close gracefully.
		_ = f.Close()
		return nil, fmt.Errorf("engine: OpenReplayLog %q: file is empty", path)
	}
	// Seek back to the beginning so the scanner (or gzip reader) sees the
	// full file.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("engine: OpenReplayLog seek %q: %w", path, err)
	}

	rr := &ReplayReader{file: f}

	isGzip := n == 2 && magic[0] == gzipMagic[0] && magic[1] == gzipMagic[1]
	if isGzip {
		gz, err := gzip.NewReader(f)
		if err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("engine: OpenReplayLog gzip init %q: %w", path, err)
		}
		rr.gz = gz
		rr.sc = bufio.NewScanner(gz)
	} else {
		rr.sc = bufio.NewScanner(f)
	}

	// Allow lines up to 4 MB to accommodate large state snapshots.
	rr.sc.Buffer(make([]byte, 64*1024), 4*1024*1024)

	return rr, nil
}

// ReadMetadata reads and parses the first line of the log as a
// [SessionMetadataEntry].
//
// It must be called exactly once, before any call to [ReplayReader.Next].
// Returns an error if the first line is missing, malformed, or is not a
// metadata record.
func (rr *ReplayReader) ReadMetadata() (SessionMetadataEntry, error) {
	rec, err := rr.nextRecord()
	if err != nil {
		return SessionMetadataEntry{}, fmt.Errorf("engine: ReadMetadata: %w", err)
	}
	meta, ok := rec.Metadata()
	if !ok {
		return SessionMetadataEntry{}, fmt.Errorf("engine: ReadMetadata: first record has type %q, want %q", rec.Type, RecordTypeMetadata)
	}
	return meta, nil
}

// Next reads and returns the next [ReplayEntry] from the log.
//
// Returns [io.EOF] when the log is exhausted. Any other error indicates a
// read or parse failure.
func (rr *ReplayReader) Next() (ReplayEntry, error) {
	rec, err := rr.nextRecord()
	if err != nil {
		return ReplayEntry{}, err // includes io.EOF
	}
	entry, ok := rec.Entry()
	if !ok {
		return ReplayEntry{}, fmt.Errorf("engine: Next: expected step record, got %q", rec.Type)
	}
	return entry, nil
}

// nextRecord scans the next non-empty line and decodes it as a [ReplayRecord].
// Returns [io.EOF] when no more lines are available.
func (rr *ReplayReader) nextRecord() (ReplayRecord, error) {
	for rr.sc.Scan() {
		line := rr.sc.Bytes()
		if len(line) == 0 {
			continue // skip blank lines
		}
		var rec ReplayRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			return ReplayRecord{}, fmt.Errorf("engine: parse record: %w", err)
		}
		return rec, nil
	}
	if err := rr.sc.Err(); err != nil {
		return ReplayRecord{}, fmt.Errorf("engine: scanner: %w", err)
	}
	return ReplayRecord{}, io.EOF
}

// Close releases all resources held by the ReplayReader. It is safe to call
// Close multiple times.
func (rr *ReplayReader) Close() error {
	if rr == nil {
		return nil
	}
	var errs []error
	if rr.gz != nil {
		if err := rr.gz.Close(); err != nil {
			errs = append(errs, fmt.Errorf("engine: ReplayReader Close gzip: %w", err))
		}
	}
	if rr.file != nil {
		if err := rr.file.Close(); err != nil {
			errs = append(errs, fmt.Errorf("engine: ReplayReader Close file: %w", err))
		}
	}
	return joinErrors(errs)
}
