// Package transport wires the engine to gRPC by providing server and client
// implementations that bridge the generated proto types to the engine's native
// Go types.
package transport

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"runtime/debug"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/game-engine/game-engine-core/api/proto/gen"
	"github.com/game-engine/game-engine-core/pkg/engine"
)

// ─────────────────────────────────────────────────────────────────────────────
// ServerOptions
// ─────────────────────────────────────────────────────────────────────────────

// ServerOptions holds configuration for [NewGRPCServer].
type ServerOptions struct {
	// Logic is the GameLogic implementation used for every session.
	// Required.
	Logic engine.GameLogic

	// GameType is the name of the game (e.g. "chess"). Used for logging and
	// session metadata.
	GameType string

	// LogDir is the base directory where .glog replay files are written.
	// When empty, replay writing is disabled.
	LogDir string

	// Headless enables RunModeHeadless for all sessions, suppressing logging
	// and enabling GZIP-compressed replay logs.
	Headless bool

	// MaxPlayersPerLobby sets how many players must join before a session
	// starts. Defaults to 2.
	MaxPlayersPerLobby int

	// Logger is the structured logger to use. When nil, slog.Default() is
	// used.
	Logger *slog.Logger
}

func (o *ServerOptions) maxPlayers() int {
	if o.MaxPlayersPerLobby <= 0 {
		return 2
	}
	return o.MaxPlayersPerLobby
}

func (o *ServerOptions) logger() *slog.Logger {
	if o.Logger != nil {
		return o.Logger
	}
	return slog.Default()
}

// ─────────────────────────────────────────────────────────────────────────────
// lobby
// ─────────────────────────────────────────────────────────────────────────────

// lobby tracks the set of players waiting to start a game of a given type.
type lobby struct {
	mu        sync.Mutex
	gameType  string
	playerIDs []string
	subs      []chan *pb.LobbyStatusUpdate // one channel per waiting player
}

// add registers a new player and returns a channel to stream updates on.
func (l *lobby) add(playerID string) chan *pb.LobbyStatusUpdate {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.playerIDs = append(l.playerIDs, playerID)
	ch := make(chan *pb.LobbyStatusUpdate, 8)
	l.subs = append(l.subs, ch)
	return ch
}

// remove removes the player and closes their update channel.
// Returns false if the player was not present.
func (l *lobby) remove(playerID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, pid := range l.playerIDs {
		if pid == playerID {
			// Close the subscriber channel and remove both entries.
			close(l.subs[i])
			l.playerIDs = append(l.playerIDs[:i], l.playerIDs[i+1:]...)
			l.subs = append(l.subs[:i], l.subs[i+1:]...)
			return true
		}
	}
	return false
}

// broadcast sends u to every subscriber. It does not block; slow subscribers
// will miss updates if their channel is full.
func (l *lobby) broadcast(u *pb.LobbyStatusUpdate) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, ch := range l.subs {
		select {
		case ch <- u:
		default:
		}
	}
}

// snapshot returns a point-in-time copy of the current player list.
func (l *lobby) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	cp := make([]string, len(l.playerIDs))
	copy(cp, l.playerIDs)
	return cp
}

// drain closes all subscriber channels and clears the lobby.
func (l *lobby) drain() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, ch := range l.subs {
		close(ch)
	}
	l.playerIDs = nil
	l.subs = nil
}

// ─────────────────────────────────────────────────────────────────────────────
// MatchmakingServer
// ─────────────────────────────────────────────────────────────────────────────

// MatchmakingServer implements [pb.MatchmakingServer]. It manages in-memory
// lobbies, broadcasts status updates to waiting players, and triggers session
// creation once a lobby is full.
type MatchmakingServer struct {
	pb.UnimplementedMatchmakingServer

	opts   ServerOptions
	log    *slog.Logger
	mu     sync.Mutex
	// lobbies maps gameType → *lobby
	lobbies map[string]*lobby
}

// newMatchmakingServer constructs a MatchmakingServer.
func newMatchmakingServer(opts ServerOptions) *MatchmakingServer {
	return &MatchmakingServer{
		opts:    opts,
		log:     opts.logger(),
		lobbies: make(map[string]*lobby),
	}
}

// getOrCreateLobby returns the existing lobby for gameType, or creates one.
func (s *MatchmakingServer) getOrCreateLobby(gameType string) *lobby {
	s.mu.Lock()
	defer s.mu.Unlock()
	if l, ok := s.lobbies[gameType]; ok {
		return l
	}
	l := &lobby{gameType: gameType}
	s.lobbies[gameType] = l
	return l
}

// JoinLobby adds the player to a lobby and streams [pb.LobbyStatusUpdate]
// messages until the lobby fills up and the game starts (or the client
// disconnects).
func (s *MatchmakingServer) JoinLobby(req *pb.JoinRequest, stream grpc.ServerStreamingServer[pb.LobbyStatusUpdate]) error {
	if req.GetPlayerId() == "" {
		return status.Error(codes.InvalidArgument, "player_id must not be empty")
	}
	gameType := req.GetGameType()
	if gameType == "" {
		gameType = s.opts.GameType
	}

	l := s.getOrCreateLobby(gameType)
	ch := l.add(req.GetPlayerId())

	s.log.Info("player joined lobby",
		slog.String("player_id", req.GetPlayerId()),
		slog.String("game_type", gameType),
	)

	// Broadcast the updated lobby state to all waiters.
	snap := l.snapshot()
	l.broadcast(&pb.LobbyStatusUpdate{
		SessionId:    "",
		ReadyPlayers: snap,
		GameStarting: len(snap) >= s.opts.maxPlayers(),
	})

	// If the lobby is now full, drain it and signal game start.
	if len(snap) >= s.opts.maxPlayers() {
		l.drain()
	}

	ctx := stream.Context()
	for {
		select {
		case u, ok := <-ch:
			if !ok {
				// Channel closed — lobby was drained (game starting) or player
				// was removed via CancelJoin.
				return nil
			}
			if err := stream.Send(u); err != nil {
				return err
			}
			if u.GetGameStarting() {
				return nil
			}
		case <-ctx.Done():
			l.remove(req.GetPlayerId())
			return ctx.Err()
		}
	}
}

// CancelJoin removes the player from the pending lobby.
func (s *MatchmakingServer) CancelJoin(ctx context.Context, req *pb.JoinRequest) (*pb.JoinResponse, error) {
	if req.GetPlayerId() == "" {
		return nil, status.Error(codes.InvalidArgument, "player_id must not be empty")
	}
	gameType := req.GetGameType()
	if gameType == "" {
		gameType = s.opts.GameType
	}

	s.mu.Lock()
	l, ok := s.lobbies[gameType]
	s.mu.Unlock()

	if !ok || !l.remove(req.GetPlayerId()) {
		return &pb.JoinResponse{
			Status: "not_found",
		}, nil
	}

	s.log.Info("player cancelled join",
		slog.String("player_id", req.GetPlayerId()),
		slog.String("game_type", gameType),
	)

	// Broadcast the updated lobby state to remaining players.
	snap := l.snapshot()
	l.broadcast(&pb.LobbyStatusUpdate{
		ReadyPlayers: snap,
		GameStarting: false,
	})

	return &pb.JoinResponse{
		Status: "cancelled",
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// streamPlayerAdapter — bridges a gRPC bidi stream to engine.PlayerAdapter
// ─────────────────────────────────────────────────────────────────────────────

// streamPlayerAdapter adapts a gRPC bidirectional stream to the
// [engine.PlayerAdapter] interface. The server sends [pb.StateUpdate] messages
// via the stream and waits for [pb.Action] responses.
//
// It is not goroutine-safe; the runner calls RequestAction from a single
// goroutine.
type streamPlayerAdapter struct {
	stream   grpc.BidiStreamingServer[pb.Action, pb.StateUpdate]
	playerID string
}

// RequestAction sends the state update over the stream and waits for the
// player's action response.
func (a *streamPlayerAdapter) RequestAction(ctx context.Context, update engine.StateUpdate) (engine.Action, error) {
	// Convert engine.State to pb.StateUpdate.
	pbUpdate := &pb.StateUpdate{
		State: &pb.State{
			Payload:   update.State.Payload,
			GameId:    update.State.GameID,
			StepIndex: update.State.StepIndex,
		},
		RewardDelta: update.RewardDelta,
		IsTerminal:  update.IsTerminal,
		ActorId:     update.ActorID,
	}

	// Send the state update to the client.
	if err := a.stream.Send(pbUpdate); err != nil {
		return engine.Action{}, fmt.Errorf("transport: stream send failed: %w", err)
	}

	// Receive the action. Honour context cancellation.
	type recvResult struct {
		action *pb.Action
		err    error
	}
	recvCh := make(chan recvResult, 1)
	go func() {
		act, err := a.stream.Recv()
		recvCh <- recvResult{act, err}
	}()

	select {
	case <-ctx.Done():
		return engine.Action{}, ctx.Err()
	case r := <-recvCh:
		if r.err != nil {
			if r.err == io.EOF {
				return engine.Action{}, fmt.Errorf("transport: client closed stream unexpectedly")
			}
			return engine.Action{}, fmt.Errorf("transport: stream recv failed: %w", r.err)
		}
		return engine.Action{
			ActorID:     r.action.GetActorId(),
			Payload:     r.action.GetPayload(),
			TimestampMs: r.action.GetTimestampMs(),
		}, nil
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// GameSessionServer
// ─────────────────────────────────────────────────────────────────────────────

// GameSessionServer implements [pb.GameSessionServer]. It bridges the gRPC
// bidirectional [Play] stream to the engine's [engine.Runner] via a
// [streamPlayerAdapter], and serves completed session replays via [GetReplay].
type GameSessionServer struct {
	pb.UnimplementedGameSessionServer

	opts engine.GameLogic
	sOpts ServerOptions
	log  *slog.Logger
	// activeSessions tracks in-flight session cancel funcs for graceful drain.
	mu       sync.Mutex
	sessions map[string]context.CancelFunc
}

// newGameSessionServer constructs a GameSessionServer.
func newGameSessionServer(opts ServerOptions) *GameSessionServer {
	return &GameSessionServer{
		sOpts:    opts,
		opts:     opts.Logic,
		log:      opts.logger(),
		sessions: make(map[string]context.CancelFunc),
	}
}

// Play implements the bidirectional streaming RPC. It receives the first
// [pb.Action] which must carry a session_id via its actor_id, creates a
// session, and hands control to [engine.Runner].
//
// Protocol: the client sends the very first action with actor_id set to the
// player_id the client wants to play as, and payload set to the initial config
// as raw JSON. The server uses that to initialise the session.
//
// For simplicity in this implementation, each stream = one player in a
// dedicated single-player session (the matchmaking lobby is responsible for
// pairing players before the Play stream is opened in a real deployment).
func (s *GameSessionServer) Play(stream grpc.BidiStreamingServer[pb.Action, pb.StateUpdate]) error {
	ctx := stream.Context()

	// Read the first message to learn the player ID and session config.
	firstMsg, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "expected initial action: %v", err)
	}
	playerID := firstMsg.GetActorId()
	if playerID == "" {
		return status.Error(codes.InvalidArgument, "initial action must have actor_id set to player_id")
	}

	sessionID := fmt.Sprintf("%s-%d", playerID, time.Now().UnixNano())
	gameType := s.sOpts.GameType
	if gameType == "" {
		gameType = "unknown"
	}

	mode := engine.RunModeLive
	if s.sOpts.Headless {
		mode = engine.RunModeHeadless
	}

	var replayPath string
	if s.sOpts.LogDir != "" {
		replayPath = filepath.Join(s.sOpts.LogDir, sessionID+".glog")
	}

	cfg := engine.SessionConfig{
		SessionID:     sessionID,
		GameType:      gameType,
		PlayerIDs:     []string{playerID},
		InitialConfig: engine.JSON(firstMsg.GetPayload()),
		Mode:          mode,
		ReplayPath:    replayPath,
	}

	session, err := engine.NewSession(cfg, s.opts)
	if err != nil {
		return status.Errorf(codes.Internal, "failed to create session: %v", err)
	}

	// Track the session for graceful drain.
	sessCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.sessions[sessionID] = cancel
	s.mu.Unlock()
	defer func() {
		cancel()
		s.mu.Lock()
		delete(s.sessions, sessionID)
		s.mu.Unlock()
	}()

	// Wrap the stream as a PlayerAdapter.
	adapter := &streamPlayerAdapter{stream: stream, playerID: playerID}

	// Wrap with a timeout adapter for AI / human limits.
	timeout := cfg.AITimeout
	if timeout == 0 {
		timeout = 50 * time.Millisecond
	}
	if mode == engine.RunModeLive {
		timeout = engine.DefaultHumanTimeout
	}
	timedAdapter := engine.NewTimeoutAdapter(adapter, engine.NewRandomFallbackAdapter(), timeout)

	players := map[string]engine.PlayerAdapter{
		playerID: timedAdapter,
	}

	runner := engine.NewRunner()
	if runErr := runner.Run(sessCtx, session, players); runErr != nil {
		s.log.Error("session error",
			slog.String("session_id", sessionID),
			slog.Any("error", runErr),
		)
		return status.Errorf(codes.Internal, "session failed: %v", runErr)
	}

	// Send a terminal state update so the client knows the game ended.
	if sendErr := stream.Send(&pb.StateUpdate{
		State: &pb.State{
			Payload:   session.State.Payload,
			GameId:    session.State.GameID,
			StepIndex: session.State.StepIndex,
		},
		IsTerminal: true,
	}); sendErr != nil {
		// Client may have already disconnected; not a hard failure.
		s.log.Warn("failed to send terminal update",
			slog.String("session_id", sessionID),
			slog.Any("error", sendErr),
		)
	}

	return nil
}

// GetReplay opens the .glog file for the requested session and streams
// [pb.ReplayEntry] messages to the client.
func (s *GameSessionServer) GetReplay(req *pb.GetReplayRequest, stream grpc.ServerStreamingServer[pb.ReplayEntry]) error {
	sessionID := req.GetSessionId()
	if sessionID == "" {
		return status.Error(codes.InvalidArgument, "session_id must not be empty")
	}

	logPath := filepath.Join(s.sOpts.LogDir, sessionID+".glog")

	reader, err := engine.OpenReplayLog(logPath)
	if err != nil {
		return status.Errorf(codes.NotFound, "replay not found for session %q: %v", sessionID, err)
	}
	defer reader.Close()

	// Skip the metadata record.
	if _, err := reader.ReadMetadata(); err != nil {
		return status.Errorf(codes.Internal, "failed to read replay metadata: %v", err)
	}

	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		entry, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return status.Errorf(codes.Internal, "replay read error: %v", err)
		}

		pbEntry := &pb.ReplayEntry{
			StepIndex:     int32(entry.StepIndex),
			ActorId:       entry.ActorID,
			ActionTaken:   entry.ActionTaken,
			StateSnapshot: entry.StateSnapshot,
			RewardDelta:   entry.RewardDelta,
			IsTerminal:    entry.IsTerminal,
		}
		if err := stream.Send(pbEntry); err != nil {
			return err
		}
	}
}

// DrainSessions cancels all active sessions and waits for them to finish.
// It is called during graceful shutdown.
func (s *GameSessionServer) DrainSessions() {
	s.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(s.sessions))
	for _, cancel := range s.sessions {
		cancels = append(cancels, cancel)
	}
	s.mu.Unlock()

	for _, cancel := range cancels {
		cancel()
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Interceptors
// ─────────────────────────────────────────────────────────────────────────────

// UnaryLoggingInterceptor logs unary RPC calls with method name, duration, and
// any error code.
func UnaryLoggingInterceptor(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		code := codes.OK
		if err != nil {
			code = status.Code(err)
		}
		log.Info("unary rpc",
			slog.String("method", info.FullMethod),
			slog.Duration("duration", time.Since(start)),
			slog.String("code", code.String()),
		)
		return resp, err
	}
}

// StreamLoggingInterceptor logs streaming RPC calls.
func StreamLoggingInterceptor(log *slog.Logger) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()
		err := handler(srv, ss)
		code := codes.OK
		if err != nil {
			code = status.Code(err)
		}
		log.Info("stream rpc",
			slog.String("method", info.FullMethod),
			slog.Duration("duration", time.Since(start)),
			slog.String("code", code.String()),
		)
		return err
	}
}

// UnaryPanicRecoveryInterceptor recovers from panics in unary handlers,
// logs the stack trace, and returns an Internal status error.
func UnaryPanicRecoveryInterceptor(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				log.Error("panic in unary handler",
					slog.String("method", info.FullMethod),
					slog.Any("panic", r),
					slog.String("stack", string(stack)),
				)
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()
		return handler(ctx, req)
	}
}

// StreamPanicRecoveryInterceptor recovers from panics in stream handlers.
func StreamPanicRecoveryInterceptor(log *slog.Logger) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) (err error) {
		defer func() {
			if r := recover(); r != nil {
				stack := debug.Stack()
				log.Error("panic in stream handler",
					slog.String("method", info.FullMethod),
					slog.Any("panic", r),
					slog.String("stack", string(stack)),
				)
				err = status.Errorf(codes.Internal, "internal server error")
			}
		}()
		return handler(srv, ss)
	}
}

// UnaryDeadlineInterceptor injects a default deadline on unary calls that
// arrive without one.
func UnaryDeadlineInterceptor(defaultTimeout time.Duration) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
			defer cancel()
		}
		return handler(ctx, req)
	}
}

// StreamDeadlineInterceptor injects a default deadline on streaming calls
// that arrive without one.
func StreamDeadlineInterceptor(defaultTimeout time.Duration) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := ss.Context()
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
			defer cancel()
			ss = &wrappedStream{ServerStream: ss, ctx: ctx}
		}
		return handler(srv, ss)
	}
}

// wrappedStream overrides the context of a [grpc.ServerStream].
type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context { return w.ctx }

// ─────────────────────────────────────────────────────────────────────────────
// NewGRPCServer — public constructor
// ─────────────────────────────────────────────────────────────────────────────

// NewGRPCServer constructs a *grpc.Server with the MatchmakingServer and
// GameSessionServer registered, and the standard set of server-side
// interceptors applied:
//   - panic recovery (both unary and streaming)
//   - structured request logging
//   - deadline injection (30 s default for unary, none for streams)
//
// Additional grpc.ServerOption values (e.g. TLS credentials) can be passed as
// extraOpts.
//
// The returned *GameSessionServer is also returned so that the caller can
// invoke [GameSessionServer.DrainSessions] during graceful shutdown.
func NewGRPCServer(opts ServerOptions, extraOpts ...grpc.ServerOption) (*grpc.Server, *GameSessionServer) {
	log := opts.logger()
	const defaultUnaryTimeout = 30 * time.Second

	serverOpts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(
			UnaryPanicRecoveryInterceptor(log),
			UnaryLoggingInterceptor(log),
			UnaryDeadlineInterceptor(defaultUnaryTimeout),
		),
		grpc.ChainStreamInterceptor(
			StreamPanicRecoveryInterceptor(log),
			StreamLoggingInterceptor(log),
		),
	}
	serverOpts = append(serverOpts, extraOpts...)

	srv := grpc.NewServer(serverOpts...)

	mm := newMatchmakingServer(opts)
	gs := newGameSessionServer(opts)

	pb.RegisterMatchmakingServer(srv, mm)
	pb.RegisterGameSessionServer(srv, gs)

	return srv, gs
}
