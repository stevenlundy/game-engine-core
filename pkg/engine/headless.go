package engine

import (
	"context"
	"log/slog"
	"sync"
)

// ─────────────────────────────────────────────────────────────────────────────
// DiscardHandler
// ─────────────────────────────────────────────────────────────────────────────

// DiscardHandler is a [slog.Handler] that silently discards every log record.
// It is used in [RunModeHeadless] to suppress all structured log output
// without changing any call sites.
type DiscardHandler struct{}

// NewDiscardHandler returns a new DiscardHandler.
func NewDiscardHandler() *DiscardHandler { return &DiscardHandler{} }

// Enabled always returns false, so the slog package skips record construction
// for all levels — making headless mode truly zero-cost for logging.
func (d *DiscardHandler) Enabled(_ context.Context, _ slog.Level) bool { return false }

// Handle discards the record unconditionally.
func (d *DiscardHandler) Handle(_ context.Context, _ slog.Record) error { return nil }

// WithAttrs returns the same handler; attributes are not stored.
func (d *DiscardHandler) WithAttrs(_ []slog.Attr) slog.Handler { return d }

// WithGroup returns the same handler; groups are not tracked.
func (d *DiscardHandler) WithGroup(_ string) slog.Handler { return d }

// ─────────────────────────────────────────────────────────────────────────────
// BatchResult
// ─────────────────────────────────────────────────────────────────────────────

// BatchResult holds the outcome of a single session processed by [BatchRunner].
type BatchResult struct {
	// SessionID echoes the session identifier from the originating [SessionConfig].
	SessionID string

	// Err is non-nil if the session failed to complete (e.g. context cancelled,
	// unrecoverable logic error). A nil Err means the game ran to a terminal
	// state successfully.
	Err error

	// Steps is the number of actions that were applied before the session
	// terminated. Zero if the game was terminal on the first IsTerminal check.
	Steps int64

	// WinnerID is the ID of the winning player, or empty for a draw / error.
	// It is populated only when Err is nil.
	WinnerID string
}

// ─────────────────────────────────────────────────────────────────────────────
// BatchRunner
// ─────────────────────────────────────────────────────────────────────────────

// BatchRunner executes multiple headless game sessions concurrently using a
// fixed-size worker pool. It is the primary entry point for large-scale AI
// training and statistical simulation.
//
// Each worker receives a [SessionConfig], constructs a [Session] using the
// provided [GameLogic] factory, and runs it to completion with
// [RandomFallbackAdapter] players.
type BatchRunner struct {
	// Parallelism controls the maximum number of sessions running
	// concurrently. Defaults to 1 if zero or negative.
	Parallelism int

	// LogicFactory creates a fresh [GameLogic] instance for each session.
	// A factory is required (rather than a single shared instance) so that
	// implementations with internal mutable state remain goroutine-safe.
	LogicFactory func() GameLogic
}

// NewBatchRunner creates a BatchRunner with the given parallelism and logic
// factory. Panics if logicFactory is nil.
func NewBatchRunner(parallelism int, logicFactory func() GameLogic) *BatchRunner {
	if logicFactory == nil {
		panic("engine: BatchRunner logicFactory must not be nil")
	}
	if parallelism <= 0 {
		parallelism = 1
	}
	return &BatchRunner{
		Parallelism:  parallelism,
		LogicFactory: logicFactory,
	}
}

// RunAll runs every config in configs as an independent headless session,
// respecting the configured Parallelism. It blocks until all sessions have
// finished or ctx is cancelled.
//
// The returned slice has the same length as configs and preserves order:
// results[i] corresponds to configs[i].
//
// RunAll returns a non-nil error only for unrecoverable infrastructure
// failures (e.g. unable to create a session); per-session errors are reported
// in BatchResult.Err.
func (b *BatchRunner) RunAll(ctx context.Context, configs []SessionConfig) ([]BatchResult, error) {
	n := len(configs)
	results := make([]BatchResult, n)

	if n == 0 {
		return results, nil
	}

	type work struct {
		idx int
		cfg SessionConfig
	}

	jobs := make(chan work, n)
	for i, cfg := range configs {
		// Force headless mode regardless of what was set in the config.
		cfg.Mode = RunModeHeadless
		jobs <- work{idx: i, cfg: cfg}
	}
	close(jobs)

	parallelism := b.Parallelism
	if parallelism <= 0 {
		parallelism = 1
	}

	var wg sync.WaitGroup
	wg.Add(parallelism)

	for range parallelism {
		go func() {
			defer wg.Done()
			runner := NewRunner()
			fb := NewRandomFallbackAdapter()

			for job := range jobs {
				// Check for context cancellation between jobs.
				if ctx.Err() != nil {
					results[job.idx] = BatchResult{
						SessionID: job.cfg.SessionID,
						Err:       ctx.Err(),
					}
					continue
				}

				logic := b.LogicFactory()
				session, err := NewSession(job.cfg, logic)
				if err != nil {
					results[job.idx] = BatchResult{
						SessionID: job.cfg.SessionID,
						Err:       err,
					}
					continue
				}

				// Build a players map with RandomFallbackAdapters for every player.
				players := make(map[string]PlayerAdapter, len(job.cfg.PlayerIDs))
				for _, pid := range job.cfg.PlayerIDs {
					players[pid] = fb
				}

				runErr := runner.Run(ctx, session, players)

				results[job.idx] = BatchResult{
					SessionID: job.cfg.SessionID,
					Steps:     session.step,
					WinnerID:  session.winnerID,
					Err:       runErr,
				}
			}
		}()
	}

	wg.Wait()
	return results, nil
}
