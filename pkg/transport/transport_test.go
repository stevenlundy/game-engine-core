package transport_test

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	pb "github.com/game-engine/game-engine-core/api/proto/gen"
	"github.com/game-engine/game-engine-core/pkg/engine"
	"github.com/game-engine/game-engine-core/pkg/transport"
)

const bufSize = 1 << 20 // 1 MB

// ─────────────────────────────────────────────────────────────────────────────
// Minimal GameLogic for tests: terminates after maxSteps actions.
// ─────────────────────────────────────────────────────────────────────────────

type countdownLogic struct{ maxSteps int }

func (c *countdownLogic) GetInitialState(_ engine.JSON) (engine.State, error) {
	payload, _ := json.Marshal(map[string]int{"step": 0})
	return engine.State{GameID: "test", Payload: payload}, nil
}

func (c *countdownLogic) ValidateAction(_ engine.State, _ engine.Action) error { return nil }

func (c *countdownLogic) ApplyAction(s engine.State, a engine.Action) (engine.State, float64, error) {
	var m map[string]int
	_ = json.Unmarshal(s.Payload, &m)
	m["step"]++
	payload, _ := json.Marshal(m)
	s.Payload = payload
	s.StepIndex++
	return s, 1.0, nil
}

func (c *countdownLogic) IsTerminal(s engine.State) (engine.TerminalResult, error) {
	var m map[string]int
	_ = json.Unmarshal(s.Payload, &m)
	if m["step"] >= c.maxSteps {
		return engine.TerminalResult{IsOver: true, WinnerID: "p1"}, nil
	}
	return engine.TerminalResult{}, nil
}

func (c *countdownLogic) GetRichState(s engine.State) (interface{}, error) { return s, nil }
func (c *countdownLogic) GetTensorState(_ engine.State) ([]float32, error) { return []float32{0}, nil }

// ─────────────────────────────────────────────────────────────────────────────
// Test helpers
// ─────────────────────────────────────────────────────────────────────────────

func newTestServer(t *testing.T, logic engine.GameLogic, maxPlayers int) (*bufconn.Listener, func()) {
	t.Helper()
	lis := bufconn.Listen(bufSize)
	opts := transport.ServerOptions{
		Logic:              logic,
		GameType:           "countdown",
		MaxPlayersPerLobby: maxPlayers,
	}
	srv, _ := transport.NewGRPCServer(opts)
	go func() { _ = srv.Serve(lis) }()
	return lis, func() {
		srv.GracefulStop()
		_ = lis.Close()
	}
}

func newTestConn(t *testing.T, lis *bufconn.Listener) *grpc.ClientConn {
	t.Helper()
	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

// ─────────────────────────────────────────────────────────────────────────────
// Tests
// ─────────────────────────────────────────────────────────────────────────────

// TestJoinLobby verifies that two clients joining the same game type both
// receive a LobbyStatusUpdate with GameStarting=true.
func TestJoinLobby(t *testing.T) {
	lis, stop := newTestServer(t, &countdownLogic{maxSteps: 1}, 2)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	join := func(playerID string) <-chan *pb.LobbyStatusUpdate {
		ch := make(chan *pb.LobbyStatusUpdate, 10)
		conn := newTestConn(t, lis)
		client := pb.NewMatchmakingClient(conn)
		stream, err := client.JoinLobby(ctx, &pb.JoinRequest{
			PlayerId: playerID,
			GameType: "countdown",
		})
		if err != nil {
			t.Errorf("JoinLobby(%s): %v", playerID, err)
			close(ch)
			return ch
		}
		go func() {
			defer close(ch)
			for {
				msg, err := stream.Recv()
				if err != nil {
					return
				}
				ch <- msg
			}
		}()
		return ch
	}

	ch1 := join("p1")
	ch2 := join("p2")

	sawStart1, sawStart2 := false, false
	deadline := time.After(5 * time.Second)
	for !sawStart1 || !sawStart2 {
		select {
		case msg, ok := <-ch1:
			if ok && msg.GetGameStarting() {
				sawStart1 = true
			}
		case msg, ok := <-ch2:
			if ok && msg.GetGameStarting() {
				sawStart2 = true
			}
		case <-deadline:
			t.Fatalf("timed out waiting for game_starting: p1=%v p2=%v", sawStart1, sawStart2)
		}
	}
}

// TestPlay runs a single-player countdownGame (N=3 steps) over a gRPC bidi
// stream and confirms the final StateUpdate has IsTerminal=true.
// (Each Play stream is a self-contained single-player session on this server.)
func TestPlay(t *testing.T) {
	const N = 3
	lis, stop := newTestServer(t, &countdownLogic{maxSteps: N}, 1)
	defer stop()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn := newTestConn(t, lis)
	client := pb.NewGameSessionClient(conn)
	stream, err := client.Play(ctx)
	if err != nil {
		t.Fatalf("Play: %v", err)
	}

	// Send initial join message.
	if err := stream.Send(&pb.Action{
		ActorId: "p1",
		Payload: []byte(`{}`),
	}); err != nil {
		t.Fatalf("Send initial: %v", err)
	}

	var updates []*pb.StateUpdate
	for {
		msg, err := stream.Recv()
		if err != nil {
			break
		}
		updates = append(updates, msg)
		if msg.GetIsTerminal() {
			break
		}
		// Echo an action to advance the game.
		if err := stream.Send(&pb.Action{
			ActorId: "p1",
			Payload: []byte(`{}`),
		}); err != nil {
			break
		}
	}

	if len(updates) == 0 {
		t.Fatal("received no StateUpdates")
	}
	last := updates[len(updates)-1]
	if !last.GetIsTerminal() {
		t.Errorf("last StateUpdate: want IsTerminal=true, got false (received %d updates)", len(updates))
	}
	t.Logf("TestPlay: received %d StateUpdates, terminal=%v", len(updates), last.GetIsTerminal())
}

// TestGetReplay verifies that after a session completes, GetReplay streams
// at least one replay entry.
func TestGetReplay(t *testing.T) {
	const N = 2
	logDir := t.TempDir()
	lis := bufconn.Listen(bufSize)
	opts := transport.ServerOptions{
		Logic:              &countdownLogic{maxSteps: N},
		GameType:           "countdown",
		MaxPlayersPerLobby: 1,
		LogDir:             logDir,
	}
	srv, _ := transport.NewGRPCServer(opts)
	go func() { _ = srv.Serve(lis) }()
	defer func() {
		srv.GracefulStop()
		_ = lis.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn := newTestConn(t, lis)
	gsClient := pb.NewGameSessionClient(conn)

	// Play a complete single-player session.
	stream, err := gsClient.Play(ctx)
	if err != nil {
		t.Fatalf("Play: %v", err)
	}
	if err := stream.Send(&pb.Action{ActorId: "solo", Payload: []byte(`{}`)}); err != nil {
		t.Fatalf("Send: %v", err)
	}
	for {
		msg, err := stream.Recv()
		if err != nil {
			break
		}
		if msg.GetIsTerminal() {
			break
		}
		_ = stream.Send(&pb.Action{ActorId: "solo", Payload: []byte(`{}`)})
	}

	// Give the server a moment to flush the replay log to disk.
	time.Sleep(100 * time.Millisecond)

	// The server writes <logDir>/<sessionID>.glog — scan the dir to find it.
	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no .glog file written to LogDir — GetReplay cannot be tested")
	}
	// Strip the .glog extension to get the session ID.
	filename := entries[0].Name()
	sessionID := strings.TrimSuffix(filename, ".glog")
	t.Logf("found replay file: %s (session_id=%s)", filename, sessionID)

	replayStream, err := gsClient.GetReplay(ctx, &pb.GetReplayRequest{SessionId: sessionID})
	if err != nil {
		t.Fatalf("GetReplay: %v", err)
	}
	var count int
	for {
		_, err := replayStream.Recv()
		if err != nil {
			break
		}
		count++
	}
	if count == 0 {
		t.Error("GetReplay returned 0 entries")
	}
	t.Logf("GetReplay returned %d entries for session %s", count, sessionID)
}
