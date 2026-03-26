// Package main is the entry point for the game-engine-core gRPC server.
//
// Configuration is read from environment variables:
//
//	PORT      — TCP port to listen on (default: "50051")
//	GAME_TYPE — Game type identifier passed to all sessions (default: "default")
//	HEADLESS  — Set to "true" or "1" to enable headless / simulation mode
//	LOG_DIR   — Directory for .glog replay files (default: "logs")
//
// The server registers both MatchmakingServer and GameSessionServer and
// enables gRPC server reflection so that tools like grpcurl / evans can
// introspect the API without a local copy of the .proto files.
//
// On SIGTERM or SIGINT the server stops accepting new connections, drains all
// in-flight game sessions (cancelling their contexts), flushes any open replay
// logs, and exits.
package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc/reflection"

	"github.com/game-engine/game-engine-core/pkg/engine"
	"github.com/game-engine/game-engine-core/pkg/transport"
)

func main() {
	logger := slog.Default()

	// ── Read configuration from environment ──────────────────────────────────
	port := envOrDefault("PORT", "50051")
	gameType := envOrDefault("GAME_TYPE", "default")
	headless := envBool("HEADLESS")
	logDir := envOrDefault("LOG_DIR", "logs")

	// Ensure the log directory exists.
	if logDir != "" {
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			logger.Error("failed to create log dir", slog.String("path", logDir), slog.Any("error", err))
			os.Exit(1)
		}
	}

	logger.Info("starting game-engine-core server",
		slog.String("port", port),
		slog.String("game_type", gameType),
		slog.Bool("headless", headless),
		slog.String("log_dir", logDir),
	)

	// ── Build the gRPC server ─────────────────────────────────────────────────
	opts := transport.ServerOptions{
		Logic:    engine.NewNoopGame(),
		GameType: gameType,
		LogDir:   logDir,
		Headless: headless,
		Logger:   logger,
	}

	grpcServer, sessionServer := transport.NewGRPCServer(opts)

	// Enable server reflection for grpcurl / evans / buf tooling.
	reflection.Register(grpcServer)

	// ── TCP listener ─────────────────────────────────────────────────────────
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		logger.Error("failed to listen", slog.String("addr", ":"+port), slog.Any("error", err))
		os.Exit(1)
	}

	// ── Signal handling ───────────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server listening", slog.String("addr", lis.Addr().String()))
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

	// ── Graceful shutdown ─────────────────────────────────────────────────────
	// 1. Stop accepting new connections and wait for in-flight RPCs to finish.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		// Drain in-flight game sessions (cancels their contexts).
		sessionServer.DrainSessions()
		// GracefulStop waits for all active RPCs to complete.
		grpcServer.GracefulStop()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("graceful shutdown complete")
	case <-shutdownCtx.Done():
		logger.Warn("graceful shutdown timed out, forcing stop")
		grpcServer.Stop()
	}
}

// envOrDefault returns the value of the named environment variable, or
// defaultVal if it is not set or is empty.
func envOrDefault(name, defaultVal string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return defaultVal
}

// envBool returns true when the named environment variable is set to "true",
// "1", or "yes" (case-insensitive). All other values (including unset) are
// false.
func envBool(name string) bool {
	switch os.Getenv(name) {
	case "true", "True", "TRUE", "1", "yes", "Yes", "YES":
		return true
	default:
		return false
	}
}
