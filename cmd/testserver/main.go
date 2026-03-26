// Package main is the test harness server for game-engine-core integration tests.
//
// It registers a CountdownGame — a deterministic GameLogic implementation that
// terminates after a fixed number of steps — against the full gRPC stack. This
// gives cross-language client tests (Python, TypeScript) a real server to talk
// to without needing a concrete game implementation.
//
// Configuration via environment variables:
//
//	PORT             — TCP port to listen on (default: "50051")
//	GAME_TYPE        — Game type identifier (default: "countdown")
//	COUNTDOWN_STEPS  — Number of steps before the game ends (default: "5")
//	MAX_PLAYERS      — Players required to start a session (default: "1")
//	LOG_DIR          — Directory for .glog replay files (default: "" = disabled)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"google.golang.org/grpc/reflection"

	"github.com/game-engine/game-engine-core/pkg/engine"
	"github.com/game-engine/game-engine-core/pkg/transport"
)

// ─────────────────────────────────────────────────────────────────────────────
// CountdownGame — a deterministic GameLogic for integration testing
// ─────────────────────────────────────────────────────────────────────────────

// countdownState is the JSON payload stored in engine.State.Payload.
type countdownState struct {
	Step int `json:"step"`
}

// CountdownGame terminates after MaxSteps actions. Every action is valid,
// every step earns a reward of 1.0, and the first player is always the winner.
type CountdownGame struct {
	MaxSteps int
}

func (g *CountdownGame) GetInitialState(_ engine.JSON) (engine.State, error) {
	payload, err := json.Marshal(countdownState{Step: 0})
	if err != nil {
		return engine.State{}, err
	}
	return engine.State{GameID: "countdown", Payload: payload}, nil
}

func (g *CountdownGame) ValidateAction(_ engine.State, _ engine.Action) error {
	return nil
}

func (g *CountdownGame) ApplyAction(s engine.State, a engine.Action) (engine.State, float64, error) {
	var cs countdownState
	if err := json.Unmarshal(s.Payload, &cs); err != nil {
		return s, 0, fmt.Errorf("countdownGame: unmarshal state: %w", err)
	}
	cs.Step++
	payload, err := json.Marshal(cs)
	if err != nil {
		return s, 0, fmt.Errorf("countdownGame: marshal state: %w", err)
	}
	s.Payload = payload
	s.StepIndex++
	return s, 1.0, nil
}

func (g *CountdownGame) IsTerminal(s engine.State) (engine.TerminalResult, error) {
	var cs countdownState
	if err := json.Unmarshal(s.Payload, &cs); err != nil {
		return engine.TerminalResult{}, fmt.Errorf("countdownGame: unmarshal state: %w", err)
	}
	if cs.Step >= g.MaxSteps {
		return engine.TerminalResult{IsOver: true, WinnerID: "p1"}, nil
	}
	return engine.TerminalResult{}, nil
}

func (g *CountdownGame) GetRichState(s engine.State) (interface{}, error) {
	var cs countdownState
	return cs, json.Unmarshal(s.Payload, &cs)
}

func (g *CountdownGame) GetTensorState(s engine.State) ([]float32, error) {
	var cs countdownState
	if err := json.Unmarshal(s.Payload, &cs); err != nil {
		return nil, err
	}
	return []float32{float32(cs.Step), float32(g.MaxSteps)}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// main
// ─────────────────────────────────────────────────────────────────────────────

func main() {
	logger := slog.Default()

	port := envOrDefault("PORT", "50051")
	gameType := envOrDefault("GAME_TYPE", "countdown")
	countdownSteps := envInt("COUNTDOWN_STEPS", 5)
	maxPlayers := envInt("MAX_PLAYERS", 1)
	logDir := os.Getenv("LOG_DIR")

	if logDir != "" {
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			logger.Error("failed to create log dir", slog.String("path", logDir), slog.Any("error", err))
			os.Exit(1)
		}
	}

	logger.Info("starting test server",
		slog.String("port", port),
		slog.String("game_type", gameType),
		slog.Int("countdown_steps", countdownSteps),
		slog.Int("max_players", maxPlayers),
	)

	opts := transport.ServerOptions{
		Logic:              &CountdownGame{MaxSteps: countdownSteps},
		GameType:           gameType,
		LogDir:             logDir,
		MaxPlayersPerLobby: maxPlayers,
		Logger:             logger,
	}

	grpcServer, sessionServer := transport.NewGRPCServer(opts)
	reflection.Register(grpcServer)

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		logger.Error("failed to listen", slog.String("addr", ":"+port), slog.Any("error", err))
		os.Exit(1)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("test server listening", slog.String("addr", lis.Addr().String()))
		serverErr <- grpcServer.Serve(lis)
	}()

	select {
	case sig := <-quit:
		logger.Info("received signal, shutting down", slog.String("signal", sig.String()))
	case err := <-serverErr:
		if err != nil {
			logger.Error("server error", slog.Any("error", err))
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		sessionServer.DrainSessions()
		grpcServer.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("shutdown complete")
	case <-shutdownCtx.Done():
		grpcServer.Stop()
	}
}

func envOrDefault(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func envInt(name string, def int) int {
	if v := os.Getenv(name); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
