package engine

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

// replayLogBufSize is the size of the bufio.Writer buffer used by ReplayLog.
// 64 KB amortises syscall overhead while keeping memory use modest.
const replayLogBufSize = 64 * 1024

// ReplayLog is the write-side handle for a session's .glog replay file.
//
// It wraps an underlying *os.File with a 64 KB [bufio.Writer] and, in
// [RunModeHeadless], an additional [gzip.Writer] for transparent compression.
//
// All public methods are safe to call concurrently from multiple goroutines;
// a [sync.Mutex] serialises writes internally.
//
// Methods on a nil *ReplayLog are no-ops, so callers can safely skip a nil
// check when [SessionConfig.ReplayPath] is empty.
type ReplayLog struct {
	mu   sync.Mutex
	file *os.File
	gz   *gzip.Writer  // non-nil only in Headless mode
	bw   *bufio.Writer // always non-nil when file != nil
}

// NewReplayLog creates (or truncates) the file at path, wraps it in a 64 KB
// [bufio.Writer], and — when mode is [RunModeHeadless] — also wraps it in a
// [gzip.Writer] for transparent compression.
//
// The caller is responsible for calling [ReplayLog.Close] when the session
// ends.
func NewReplayLog(path string, mode RunMode) (*ReplayLog, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("engine: NewReplayLog create %q: %w", path, err)
	}

	rl := &ReplayLog{file: f}

	if mode == RunModeHeadless {
		gz, err := gzip.NewWriterLevel(f, gzip.BestSpeed)
		if err != nil {
			_ = f.Close()
			return nil, fmt.Errorf("engine: NewReplayLog gzip init: %w", err)
		}
		rl.gz = gz
		rl.bw = bufio.NewWriterSize(gz, replayLogBufSize)
	} else {
		rl.bw = bufio.NewWriterSize(f, replayLogBufSize)
	}

	return rl, nil
}

// WriteMetadata encodes meta as a metadata [ReplayRecord] and writes it as
// the first newline-terminated JSON-L line in the log.
//
// This method is goroutine-safe.
func (r *ReplayLog) WriteMetadata(meta SessionMetadataEntry) error {
	if r == nil {
		return nil
	}
	return r.writeRecord(NewMetadataRecord(meta))
}

// WriteEntry serialises entry as a step [ReplayRecord] and appends it as a
// newline-terminated JSON-L line.
//
// This method is goroutine-safe.
func (r *ReplayLog) WriteEntry(entry ReplayEntry) error {
	if r == nil {
		return nil
	}
	return r.writeRecord(NewStepRecord(entry))
}

// writeRecord is the internal hot-path: it marshals rec to JSON, appends a
// newline, and writes the result to bw under the mutex.
func (r *ReplayLog) writeRecord(rec ReplayRecord) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("engine: writeRecord marshal: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, err := r.bw.Write(data); err != nil {
		return fmt.Errorf("engine: writeRecord write: %w", err)
	}
	if err := r.bw.WriteByte('\n'); err != nil {
		return fmt.Errorf("engine: writeRecord newline: %w", err)
	}
	return nil
}

// Flush flushes the [bufio.Writer] (and, if active, the [gzip.Writer]) so
// that all buffered data is passed to the underlying file. It does not close
// any writer or the file.
//
// This method is goroutine-safe.
func (r *ReplayLog) Flush() error {
	if r == nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	return r.flushLocked()
}

// flushLocked performs the flush while the caller already holds mu.
func (r *ReplayLog) flushLocked() error {
	if err := r.bw.Flush(); err != nil {
		return fmt.Errorf("engine: Flush bufio: %w", err)
	}
	if r.gz != nil {
		if err := r.gz.Flush(); err != nil {
			return fmt.Errorf("engine: Flush gzip: %w", err)
		}
	}
	return nil
}

// Close flushes all buffered data, finalises the GZIP stream (if active), and
// closes the underlying file. Calling Close on a nil *ReplayLog is a no-op.
//
// After Close returns, the ReplayLog must not be used.
func (r *ReplayLog) Close() error {
	if r == nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Flush bufio and gzip first.
	if err := r.flushLocked(); err != nil {
		// Still attempt to close the file even on flush error.
		_ = r.closeWritersLocked()
		return err
	}
	return r.closeWritersLocked()
}

// closeWritersLocked closes the gzip writer (if any) and then the underlying
// file. Must be called with mu held.
func (r *ReplayLog) closeWritersLocked() error {
	var errs []error

	if r.gz != nil {
		if err := r.gz.Close(); err != nil {
			errs = append(errs, fmt.Errorf("engine: Close gzip: %w", err))
		}
	}
	if r.file != nil {
		if err := r.file.Close(); err != nil {
			errs = append(errs, fmt.Errorf("engine: Close file: %w", err))
		}
	}

	return joinErrors(errs)
}

// joinErrors returns nil if errs is empty, the single error if len == 1,
// or a combined error message otherwise.
func joinErrors(errs []error) error {
	switch len(errs) {
	case 0:
		return nil
	case 1:
		return errs[0]
	default:
		msg := errs[0].Error()
		for _, e := range errs[1:] {
			msg += "; " + e.Error()
		}
		return fmt.Errorf("%s", msg)
	}
}

// ─── ensure ReplayLog satisfies io.Closer ─────────────────────────────────────
var _ io.Closer = (*ReplayLog)(nil)
