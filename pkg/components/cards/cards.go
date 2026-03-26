// Package cards provides reusable card-game primitives: decks, shuffling, dealing,
// and hand management with hidden-state masking for multi-player games.
package cards

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
)

// ErrInsufficientCards is returned by [Deck.Deal] and [Deck.DealTo] when the deck
// does not contain enough cards to satisfy the request.
var ErrInsufficientCards = errors.New("cards: insufficient cards in deck")

// Card represents a single playing card. Suit and Rank carry game-specific
// semantics (e.g. "hearts"/"A" for a standard deck). ID must be unique within a
// deck so that cards can be referenced unambiguously. Meta carries any additional
// game-specific data as raw JSON.
type Card struct {
	Suit string          `json:"suit"`
	Rank string          `json:"rank"`
	ID   string          `json:"id"`
	Meta json.RawMessage `json:"meta,omitempty"`
}

// Deck wraps a slice of [Card] values and maintains a separate copy of the
// original ordered slice so that [Deck.Reset] can restore the deck without
// re-constructing it from scratch.
type Deck struct {
	cards    []Card
	original []Card
}

// NewDeck constructs a new Deck from the provided cards. The supplied slice is
// copied so that the caller retains ownership of the original backing array.
func NewDeck(cards []Card) *Deck {
	orig := make([]Card, len(cards))
	copy(orig, cards)
	current := make([]Card, len(cards))
	copy(current, cards)
	return &Deck{cards: current, original: orig}
}

// Size returns the number of cards currently in the deck.
func (d *Deck) Size() int { return len(d.cards) }

// IsEmpty reports whether the deck contains no cards.
func (d *Deck) IsEmpty() bool { return len(d.cards) == 0 }

// Reset restores the deck to its original ordered state as provided to [NewDeck].
func (d *Deck) Reset() {
	d.cards = make([]Card, len(d.original))
	copy(d.cards, d.original)
}

// Shuffle randomises the order of cards in place using the Fisher-Yates algorithm.
// The caller supplies an explicit *rand.Rand so that results are deterministic
// given a fixed seed — essential for replay fidelity and statistical testing.
func (d *Deck) Shuffle(rng *rand.Rand) {
	n := len(d.cards)
	for i := n - 1; i > 0; i-- {
		j := rng.Intn(i + 1)
		d.cards[i], d.cards[j] = d.cards[j], d.cards[i]
	}
}

// Deal removes and returns the top n cards from the deck (index 0 is "top").
// Returns [ErrInsufficientCards] if the deck holds fewer than n cards.
func (d *Deck) Deal(n int) ([]Card, error) {
	if n > len(d.cards) {
		return nil, fmt.Errorf("%w: requested %d, have %d", ErrInsufficientCards, n, len(d.cards))
	}
	dealt := make([]Card, n)
	copy(dealt, d.cards[:n])
	d.cards = d.cards[n:]
	return dealt, nil
}

// DealTo deals n cards to each hand in round-robin order. Cards are taken from
// the top of the deck one at a time and distributed to each hand in sequence,
// cycling through the hands until each has received n cards.
// Returns [ErrInsufficientCards] if the deck cannot satisfy the full request.
func (d *Deck) DealTo(hands []*Hand, n int) error {
	total := n * len(hands)
	if total > len(d.cards) {
		return fmt.Errorf("%w: requested %d (%d hands × %d each), have %d",
			ErrInsufficientCards, total, len(hands), n, len(d.cards))
	}
	for round := 0; round < n; round++ {
		for _, h := range hands {
			card, _ := d.Deal(1) // error impossible: we checked above
			h.Add(card...)
		}
	}
	return nil
}

// Hand holds the cards belonging to a single player identified by OwnerID.
type Hand struct {
	OwnerID string
	Cards   []Card
}

// Add appends one or more cards to the hand.
func (h *Hand) Add(cards ...Card) {
	h.Cards = append(h.Cards, cards...)
}

// Remove finds the card with the given cardID, removes it from the hand, and
// returns it. Returns an error if no card with that ID exists.
func (h *Hand) Remove(cardID string) (Card, error) {
	for i, c := range h.Cards {
		if c.ID == cardID {
			h.Cards = append(h.Cards[:i], h.Cards[i+1:]...)
			return c, nil
		}
	}
	return Card{}, fmt.Errorf("cards: card %q not found in hand", cardID)
}

// MaskFor returns a copy of the hand as seen from viewerID's perspective.
// If viewerID matches the hand's OwnerID, all cards are returned unmodified.
// Otherwise, each card's Suit and Rank are zeroed out and its ID is replaced
// with "hidden", simulating the opaque back of a card to an opponent.
func (h *Hand) MaskFor(viewerID string) Hand {
	masked := Hand{OwnerID: h.OwnerID, Cards: make([]Card, len(h.Cards))}
	if viewerID == h.OwnerID {
		copy(masked.Cards, h.Cards)
		return masked
	}
	for i, c := range h.Cards {
		masked.Cards[i] = Card{
			Suit: "",
			Rank: "",
			ID:   "hidden",
			Meta: nil,
		}
		_ = c // card data intentionally withheld
	}
	return masked
}
