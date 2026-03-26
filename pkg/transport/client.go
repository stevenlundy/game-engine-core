package transport

import (
	"context"
	"fmt"
	"io"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/game-engine/game-engine-core/api/proto/gen"
	"github.com/game-engine/game-engine-core/pkg/engine"
)

// ─────────────────────────────────────────────────────────────────────────────
// ActionSender / StateUpdateReceiver interfaces
// ─────────────────────────────────────────────────────────────────────────────

// ActionSender is the write-side of a [GameClient.Play] stream. The caller
// invokes Send once per turn and Close when the game ends or the client
// disconnects.
type ActionSender interface {
	// Send transmits an action to the server.
	Send(action engine.Action) error

	// Close half-closes the send side of the stream, signalling to the server
	// that the client will not send any more actions.
	Close() error
}

// StateUpdateReceiver is the read-side of a [GameClient.Play] stream. The
// caller blocks on Recv until the server sends an update or the game ends.
type StateUpdateReceiver interface {
	// Recv blocks until the next [engine.StateUpdate] arrives.
	// Returns [io.EOF] when the game has ended (the stream is exhausted).
	Recv() (engine.StateUpdate, error)
}

// ─────────────────────────────────────────────────────────────────────────────
// bidiPlayStream — concrete ActionSender + StateUpdateReceiver
// ─────────────────────────────────────────────────────────────────────────────

// bidiPlayStream wraps the generated gRPC bidirectional stream type and
// translates between proto messages and engine-native types.
type bidiPlayStream struct {
	stream grpc.BidiStreamingClient[pb.Action, pb.StateUpdate]
}

// Send converts an [engine.Action] to a proto [pb.Action] and sends it.
func (b *bidiPlayStream) Send(action engine.Action) error {
	return b.stream.Send(&pb.Action{
		ActorId:     action.ActorID,
		Payload:     action.Payload,
		TimestampMs: action.TimestampMs,
	})
}

// Close half-closes the send direction of the stream.
func (b *bidiPlayStream) Close() error {
	return b.stream.CloseSend()
}

// Recv blocks for the next state update from the server.
func (b *bidiPlayStream) Recv() (engine.StateUpdate, error) {
	msg, err := b.stream.Recv()
	if err != nil {
		return engine.StateUpdate{}, err
	}
	var st engine.State
	if s := msg.GetState(); s != nil {
		st = engine.State{
			GameID:    s.GetGameId(),
			StepIndex: s.GetStepIndex(),
			Payload:   s.GetPayload(),
		}
	}
	return engine.StateUpdate{
		State:       st,
		RewardDelta: msg.GetRewardDelta(),
		IsTerminal:  msg.GetIsTerminal(),
		ActorID:     msg.GetActorId(),
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// MatchmakingClient
// ─────────────────────────────────────────────────────────────────────────────

// MatchmakingClient is a convenience wrapper around the generated
// [pb.MatchmakingClient] that exposes idiomatic Go channel-based APIs.
type MatchmakingClient struct {
	inner pb.MatchmakingClient
}

// NewMatchmakingClient creates a MatchmakingClient wrapping conn.
func NewMatchmakingClient(conn grpc.ClientConnInterface) *MatchmakingClient {
	return &MatchmakingClient{inner: pb.NewMatchmakingClient(conn)}
}

// Join sends a JoinRequest and returns a receive-only channel that delivers
// [pb.LobbyStatusUpdate] messages until the lobby fills or ctx is cancelled.
//
// The channel is closed when the underlying stream ends. Errors from the
// stream are surfaced as the last item: a nil update with the error stored in
// the returned error return value (for the initial connection step only).
func (c *MatchmakingClient) Join(ctx context.Context, req *pb.JoinRequest) (<-chan *pb.LobbyStatusUpdate, error) {
	stream, err := c.inner.JoinLobby(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("matchmaking: JoinLobby RPC failed: %w", err)
	}

	ch := make(chan *pb.LobbyStatusUpdate, 8)
	go func() {
		defer close(ch)
		for {
			msg, err := stream.Recv()
			if err != nil {
				// EOF or cancelled — stop pumping.
				return
			}
			select {
			case ch <- msg:
			case <-ctx.Done():
				return
			}
			if msg.GetGameStarting() {
				return
			}
		}
	}()

	return ch, nil
}

// CancelJoin sends a CancelJoin RPC.
func (c *MatchmakingClient) CancelJoin(ctx context.Context, req *pb.JoinRequest) (*pb.JoinResponse, error) {
	resp, err := c.inner.CancelJoin(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("matchmaking: CancelJoin RPC failed: %w", err)
	}
	return resp, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// GameClient
// ─────────────────────────────────────────────────────────────────────────────

// GameClient is a convenience wrapper around the generated [pb.GameSessionClient]
// that hides stream lifecycle management and type conversions.
type GameClient struct {
	inner pb.GameSessionClient
}

// NewGameClient creates a GameClient wrapping conn.
func NewGameClient(conn grpc.ClientConnInterface) *GameClient {
	return &GameClient{inner: pb.NewGameSessionClient(conn)}
}

// Play opens a bidirectional stream and returns an [ActionSender] and a
// [StateUpdateReceiver]. The caller sends the initial action (with actor_id
// set to the player's ID and payload set to the session config) before
// invoking any other method.
func (c *GameClient) Play(ctx context.Context) (ActionSender, StateUpdateReceiver, error) {
	stream, err := c.inner.Play(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("game: Play RPC failed: %w", err)
	}
	s := &bidiPlayStream{stream: stream}
	return s, s, nil
}

// GetReplay streams the replay entries for a completed session. The returned
// channel delivers [engine.ReplayEntry] values and is closed when the stream
// ends. The returned error is non-nil only for the initial RPC failure.
func (c *GameClient) GetReplay(ctx context.Context, sessionID string) (<-chan engine.ReplayEntry, error) {
	stream, err := c.inner.GetReplay(ctx, &pb.GetReplayRequest{SessionId: sessionID})
	if err != nil {
		return nil, fmt.Errorf("game: GetReplay RPC failed: %w", err)
	}

	ch := make(chan engine.ReplayEntry, 16)
	go func() {
		defer close(ch)
		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				return
			}
			entry := engine.ReplayEntry{
				StepIndex:     int(msg.GetStepIndex()),
				ActorID:       msg.GetActorId(),
				ActionTaken:   msg.GetActionTaken(),
				StateSnapshot: msg.GetStateSnapshot(),
				RewardDelta:   msg.GetRewardDelta(),
				IsTerminal:    msg.GetIsTerminal(),
			}
			select {
			case ch <- entry:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// GRPCPlayerAdapter
// ─────────────────────────────────────────────────────────────────────────────

// GRPCPlayerAdapter wraps a [GameClient] Play stream and satisfies the
// [engine.PlayerAdapter] interface. This adapter is used by the server's
// internal play loop or by a remote client that wants to act as a
// "shadow" player.
//
// Usage:
//
//	sender, receiver, err := client.Play(ctx)
//	// send initial action first…
//	adapter := NewGRPCPlayerAdapter(sender, receiver, "player-1")
type GRPCPlayerAdapter struct {
	sender   ActionSender
	receiver StateUpdateReceiver
	playerID string
}

// NewGRPCPlayerAdapter creates a GRPCPlayerAdapter. playerID must match the
// actor_id used to open the stream.
func NewGRPCPlayerAdapter(sender ActionSender, receiver StateUpdateReceiver, playerID string) *GRPCPlayerAdapter {
	return &GRPCPlayerAdapter{
		sender:   sender,
		receiver: receiver,
		playerID: playerID,
	}
}

// RequestAction sends the StateUpdate to the server (via the already-open
// stream) and waits for the server's StateUpdate containing the expected
// action acknowledgement. In the gRPC adapter model, the "action" is
// received from the human/AI client connected to the other side.
//
// More precisely: RequestAction sends the current state update and then
// blocks waiting to receive the server's prompt back, then sends the action
// on behalf of the real player. This adapter is intended for test scenarios
// where a single Go process controls both sides of the stream.
//
// For production use, the human or AI client drives actions; GRPCPlayerAdapter
// is primarily a test/integration utility.
func (a *GRPCPlayerAdapter) RequestAction(ctx context.Context, update engine.StateUpdate) (engine.Action, error) {
	// Receive the next state update from the server.
	recv, err := a.receiver.Recv()
	if err != nil {
		return engine.Action{}, fmt.Errorf("grpc_adapter: recv failed: %w", err)
	}
	if recv.IsTerminal {
		return engine.Action{}, io.EOF
	}

	// Derive the action from the update (here we just echo back a null action
	// for the adapter to satisfy the interface; real callers embed their AI).
	action := engine.Action{
		ActorID:     a.playerID,
		Payload:     update.State.Payload,
		TimestampMs: time.Now().UnixMilli(),
	}

	if err := a.sender.Send(action); err != nil {
		return engine.Action{}, fmt.Errorf("grpc_adapter: send failed: %w", err)
	}

	return action, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Client-side interceptors
// ─────────────────────────────────────────────────────────────────────────────
//
// # Interceptor scope: unary only
//
// The interceptors in this section ([UnaryRetryInterceptor] and
// [UnaryDeadlineInjectionInterceptor]) apply exclusively to **unary** RPCs
// (currently [MatchmakingClient.CancelJoin]).
//
// The three streaming calls — [MatchmakingClient.Join] (server-streaming),
// [GameClient.Play] (bidirectional), and [GameClient.GetReplay]
// (server-streaming) — are intentionally NOT wrapped by retry or
// deadline-injection interceptors. The reasons are:
//
//  1. Streaming retry requires reconnection semantics: the client must
//     re-establish the stream, re-send the initial join action, and reconcile
//     any state the server has already advanced. That logic belongs in the
//     caller (e.g. [engine.GameClient] or the bot loop), not in a generic
//     transport interceptor.
//
//  2. Deadline/timeout for streams is best managed via the context.Context
//     passed to each call. Pass a context with a deadline or call
//     context.WithTimeout before invoking Play/JoinLobby/GetReplay.
//
// If automatic streaming retry is added in the future, it should be
// implemented as a named StreamClientInterceptor with explicit reconnect
// and idempotency guarantees documented alongside it.

// transientCodes are gRPC status codes that are safe to retry.
var transientCodes = map[codes.Code]bool{
	codes.Unavailable:       true,
	codes.ResourceExhausted: true,
	codes.DeadlineExceeded:  true,
}

// RetryConfig configures the exponential back-off retry interceptor.
type RetryConfig struct {
	// MaxAttempts is the maximum number of attempts (including the first).
	// Zero or negative means 3.
	MaxAttempts int

	// InitialBackoff is the delay before the first retry. Defaults to 100 ms.
	InitialBackoff time.Duration

	// MaxBackoff caps the delay between retries. Defaults to 5 s.
	MaxBackoff time.Duration

	// Multiplier is the factor by which the backoff grows each attempt.
	// Defaults to 2.0.
	Multiplier float64
}

func (c *RetryConfig) maxAttempts() int {
	if c.MaxAttempts <= 0 {
		return 3
	}
	return c.MaxAttempts
}

func (c *RetryConfig) initialBackoff() time.Duration {
	if c.InitialBackoff <= 0 {
		return 100 * time.Millisecond
	}
	return c.InitialBackoff
}

func (c *RetryConfig) maxBackoff() time.Duration {
	if c.MaxBackoff <= 0 {
		return 5 * time.Second
	}
	return c.MaxBackoff
}

func (c *RetryConfig) multiplier() float64 {
	if c.Multiplier <= 0 {
		return 2.0
	}
	return c.Multiplier
}

// UnaryRetryInterceptor returns a client-side unary interceptor that retries
// transient errors with exponential back-off.
func UnaryRetryInterceptor(cfg RetryConfig) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		backoff := cfg.initialBackoff()
		maxAttempts := cfg.maxAttempts()

		var lastErr error
		for attempt := 0; attempt < maxAttempts; attempt++ {
			lastErr = invoker(ctx, method, req, reply, cc, opts...)
			if lastErr == nil {
				return nil
			}
			code := status.Code(lastErr)
			if !transientCodes[code] {
				return lastErr
			}
			if attempt == maxAttempts-1 {
				break
			}
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
			next := time.Duration(float64(backoff) * cfg.multiplier())
			if next > cfg.maxBackoff() {
				next = cfg.maxBackoff()
			}
			backoff = next
		}
		return lastErr
	}
}

// UnaryDeadlineInjectionInterceptor is a client-side interceptor that
// injects a default deadline on outgoing unary calls that do not already have
// one.
func UnaryDeadlineInjectionInterceptor(defaultTimeout time.Duration) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, defaultTimeout)
			defer cancel()
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// DefaultClientDialOptions returns the recommended dial options for
// production clients: retry with exponential back-off and a 10 s default
// deadline on unary calls.
func DefaultClientDialOptions() []grpc.DialOption {
	return []grpc.DialOption{
		grpc.WithChainUnaryInterceptor(
			UnaryRetryInterceptor(RetryConfig{}),
			UnaryDeadlineInjectionInterceptor(10*time.Second),
		),
	}
}
