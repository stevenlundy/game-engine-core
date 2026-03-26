// Package main shows the smallest possible GameLogic implementation wired to
// the engine Runner. Copy this as a starting point for a new game.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/game-engine/game-engine-core/pkg/components/timing"
	"github.com/game-engine/game-engine-core/pkg/engine"
)

// ─────────────────────────────────────────────────────────────────────────────
// 1. Implement the GameLogic interface
// ─────────────────────────────────────────────────────────────────────────────

// CountdownGame is a trivial game where the state holds a counter that
// decrements by 1 on every action. The game ends when the counter reaches 0.
type CountdownGame struct {
	StartFrom int // initial counter value
}

type countdownState struct {
	Counter int `json:"counter"`
}

func (g *CountdownGame) GetInitialState(_ engine.JSON) (engine.State, error) {
	payload, err := json.Marshal(countdownState{Counter: g.StartFrom})
	if err != nil {
		return engine.State{}, err
	}
	return engine.State{GameID: "countdown", Payload: payload}, nil
}

func (g *CountdownGame) ValidateAction(_ engine.State, _ engine.Action) error {
	return nil // any action is valid
}

func (g *CountdownGame) ApplyAction(s engine.State, a engine.Action) (engine.State, float64, error) {
	var cs countdownState
	if err := json.Unmarshal(s.Payload, &cs); err != nil {
		return s, 0, err
	}
	cs.Counter--
	payload, err := json.Marshal(cs)
	if err != nil {
		return s, 0, err
	}
	s.Payload = payload
	s.StepIndex++
	return s, 1.0, nil // reward = 1 per step
}

func (g *CountdownGame) IsTerminal(s engine.State) (engine.TerminalResult, error) {
	var cs countdownState
	if err := json.Unmarshal(s.Payload, &cs); err != nil {
		return engine.TerminalResult{}, err
	}
	if cs.Counter <= 0 {
		return engine.TerminalResult{IsOver: true, WinnerID: "player1"}, nil
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
	return []float32{float32(cs.Counter)}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// 2. Wire it to the Runner
// ─────────────────────────────────────────────────────────────────────────────

func main() {
	game := &CountdownGame{StartFrom: 5}

	cfg := engine.SessionConfig{
		SessionID: "example-session",
		GameType:  "countdown",
		PlayerIDs: []string{"player1"},
		Mode:      engine.RunModeLive,
		AITimeout: timing.DefaultAITimeout,
	}

	session, err := engine.NewSession(cfg, game)
	if err != nil {
		log.Fatalf("NewSession: %v", err)
	}

	players := map[string]engine.PlayerAdapter{
		"player1": engine.NewRandomFallbackAdapter(),
	}

	runner := engine.NewRunner()
	if err := runner.Run(context.Background(), session, players); err != nil {
		log.Fatalf("Run: %v", err)
	}

	fmt.Println("Game complete!")
}
