package cards

import (
	"errors"
	"math/rand"
	"testing"
)

// ---- helpers ----------------------------------------------------------------

func standardDeck() []Card {
	suits := []string{"hearts", "diamonds", "clubs", "spades"}
	ranks := []string{"A", "2", "3", "4", "5", "6", "7", "8", "9", "10", "J", "Q", "K"}
	cards := make([]Card, 0, 52)
	for _, s := range suits {
		for _, r := range ranks {
			cards = append(cards, Card{Suit: s, Rank: r, ID: s + "-" + r})
		}
	}
	return cards
}

// ---- Deck -------------------------------------------------------------------

func TestNewDeck_Size(t *testing.T) {
	d := NewDeck(standardDeck())
	if d.Size() != 52 {
		t.Fatalf("expected 52 cards, got %d", d.Size())
	}
}

func TestDeck_IsEmpty(t *testing.T) {
	d := NewDeck(nil)
	if !d.IsEmpty() {
		t.Fatal("expected empty deck")
	}
	d2 := NewDeck(standardDeck())
	if d2.IsEmpty() {
		t.Fatal("expected non-empty deck")
	}
}

func TestDeck_Reset(t *testing.T) {
	d := NewDeck(standardDeck())
	rng := rand.New(rand.NewSource(42))
	d.Shuffle(rng)
	d.Reset()
	if d.Size() != 52 {
		t.Fatalf("after reset expected 52, got %d", d.Size())
	}
	orig := standardDeck()
	for i, c := range d.cards {
		if c.ID != orig[i].ID {
			t.Fatalf("card %d mismatch after reset: got %s, want %s", i, c.ID, orig[i].ID)
		}
	}
}

// ---- Shuffle ----------------------------------------------------------------

func TestShuffle_IsDeterministic(t *testing.T) {
	d1, d2 := NewDeck(standardDeck()), NewDeck(standardDeck())
	d1.Shuffle(rand.New(rand.NewSource(99)))
	d2.Shuffle(rand.New(rand.NewSource(99)))
	for i := range d1.cards {
		if d1.cards[i].ID != d2.cards[i].ID {
			t.Fatalf("same seed produced different results at index %d", i)
		}
	}
}

// TestShuffle_UniformDistribution confirms that each card position is filled
// roughly uniformly over 10,000 shuffles (chi-squared-style bounds check).
func TestShuffle_UniformDistribution(t *testing.T) {
	const runs = 10_000
	const deckSize = 52

	// counts[position][cardIndex] = number of times card landed at position
	counts := make([][]int, deckSize)
	for i := range counts {
		counts[i] = make([]int, deckSize)
	}

	d := NewDeck(standardDeck())
	orig := standardDeck()
	// build ID → index map
	idxOf := make(map[string]int, deckSize)
	for i, c := range orig {
		idxOf[c.ID] = i
	}

	rng := rand.New(rand.NewSource(1337))
	for run := 0; run < runs; run++ {
		d.Reset()
		d.Shuffle(rng)
		for pos, c := range d.cards {
			counts[pos][idxOf[c.ID]]++
		}
	}

	// Each cell should be near runs/deckSize ≈ 192.
	expected := float64(runs) / float64(deckSize)
	// Allow ±40% tolerance — a uniform distribution at 10 k runs will sit
	// well within this range; a broken shuffle will deviate dramatically.
	lo, hi := expected*0.60, expected*1.40
	for pos := 0; pos < deckSize; pos++ {
		for card := 0; card < deckSize; card++ {
			v := float64(counts[pos][card])
			if v < lo || v > hi {
				t.Errorf("position %d card %d: count %.0f out of [%.0f, %.0f]",
					pos, card, v, lo, hi)
			}
		}
	}
}

// ---- Deal -------------------------------------------------------------------

func TestDeal_Success(t *testing.T) {
	d := NewDeck(standardDeck())
	cards, err := d.Deal(5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cards) != 5 {
		t.Fatalf("expected 5 cards, got %d", len(cards))
	}
	if d.Size() != 47 {
		t.Fatalf("expected 47 remaining, got %d", d.Size())
	}
}

func TestDeal_ErrInsufficientCards(t *testing.T) {
	d := NewDeck(standardDeck())
	_, err := d.Deal(53)
	if !errors.Is(err, ErrInsufficientCards) {
		t.Fatalf("expected ErrInsufficientCards, got %v", err)
	}
}

func TestDeal_ExactlyEmpty(t *testing.T) {
	d := NewDeck(standardDeck())
	_, err := d.Deal(52)
	if err != nil {
		t.Fatalf("unexpected error dealing all cards: %v", err)
	}
	if !d.IsEmpty() {
		t.Fatal("deck should be empty after dealing all cards")
	}
}

// ---- DealTo -----------------------------------------------------------------

func TestDealTo_RoundRobin(t *testing.T) {
	d := NewDeck(standardDeck())
	h1 := &Hand{OwnerID: "p1"}
	h2 := &Hand{OwnerID: "p2"}
	h3 := &Hand{OwnerID: "p3"}
	if err := d.DealTo([]*Hand{h1, h2, h3}, 5); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, h := range []*Hand{h1, h2, h3} {
		if len(h.Cards) != 5 {
			t.Errorf("hand %s: expected 5 cards, got %d", h.OwnerID, len(h.Cards))
		}
	}
	if d.Size() != 52-15 {
		t.Errorf("expected %d remaining, got %d", 52-15, d.Size())
	}
}

func TestDealTo_RoundRobinOrder(t *testing.T) {
	// With a fresh (unshuffled) deck the first card goes to h1, second to h2, etc.
	d := NewDeck(standardDeck())
	orig := standardDeck()
	h1, h2 := &Hand{OwnerID: "p1"}, &Hand{OwnerID: "p2"}
	if err := d.DealTo([]*Hand{h1, h2}, 2); err != nil {
		t.Fatal(err)
	}
	// Round 1: h1 gets orig[0], h2 gets orig[1]
	// Round 2: h1 gets orig[2], h2 gets orig[3]
	if h1.Cards[0].ID != orig[0].ID {
		t.Errorf("h1 round-1 card: got %s, want %s", h1.Cards[0].ID, orig[0].ID)
	}
	if h2.Cards[0].ID != orig[1].ID {
		t.Errorf("h2 round-1 card: got %s, want %s", h2.Cards[0].ID, orig[1].ID)
	}
	if h1.Cards[1].ID != orig[2].ID {
		t.Errorf("h1 round-2 card: got %s, want %s", h1.Cards[1].ID, orig[2].ID)
	}
	if h2.Cards[1].ID != orig[3].ID {
		t.Errorf("h2 round-2 card: got %s, want %s", h2.Cards[1].ID, orig[3].ID)
	}
}

func TestDealTo_ErrInsufficientCards(t *testing.T) {
	d := NewDeck(standardDeck()) // 52 cards
	h1, h2, h3 := &Hand{OwnerID: "p1"}, &Hand{OwnerID: "p2"}, &Hand{OwnerID: "p3"}
	// 3 hands × 18 each = 54 > 52
	err := d.DealTo([]*Hand{h1, h2, h3}, 18)
	if !errors.Is(err, ErrInsufficientCards) {
		t.Fatalf("expected ErrInsufficientCards, got %v", err)
	}
}

// ---- Hand -------------------------------------------------------------------

func TestHand_AddRemove(t *testing.T) {
	h := &Hand{OwnerID: "alice"}
	c := Card{Suit: "hearts", Rank: "A", ID: "hearts-A"}
	h.Add(c)
	if len(h.Cards) != 1 {
		t.Fatalf("expected 1 card after Add, got %d", len(h.Cards))
	}
	removed, err := h.Remove("hearts-A")
	if err != nil {
		t.Fatalf("Remove error: %v", err)
	}
	if removed.ID != "hearts-A" {
		t.Errorf("removed wrong card: %s", removed.ID)
	}
	if len(h.Cards) != 0 {
		t.Error("hand should be empty after Remove")
	}
}

func TestHand_RemoveNotFound(t *testing.T) {
	h := &Hand{OwnerID: "bob"}
	_, err := h.Remove("nonexistent")
	if err == nil {
		t.Fatal("expected error removing nonexistent card")
	}
}

// ---- MaskFor ----------------------------------------------------------------

func TestMaskFor_OwnerSeesAll(t *testing.T) {
	h := &Hand{OwnerID: "alice"}
	h.Add(
		Card{Suit: "hearts", Rank: "A", ID: "hearts-A"},
		Card{Suit: "spades", Rank: "K", ID: "spades-K"},
	)
	masked := h.MaskFor("alice")
	if len(masked.Cards) != 2 {
		t.Fatalf("owner should see all 2 cards, got %d", len(masked.Cards))
	}
	for i, c := range masked.Cards {
		if c.ID == "hidden" {
			t.Errorf("card %d should not be hidden for owner", i)
		}
		if c.Suit == "" || c.Rank == "" {
			t.Errorf("card %d suit/rank should be visible to owner", i)
		}
	}
}

func TestMaskFor_OpponentSeesHidden(t *testing.T) {
	h := &Hand{OwnerID: "alice"}
	h.Add(
		Card{Suit: "hearts", Rank: "A", ID: "hearts-A"},
		Card{Suit: "spades", Rank: "K", ID: "spades-K"},
		Card{Suit: "clubs", Rank: "2", ID: "clubs-2"},
	)
	masked := h.MaskFor("bob")
	if len(masked.Cards) != 3 {
		t.Fatalf("opponent should see same card count, got %d", len(masked.Cards))
	}
	for i, c := range masked.Cards {
		if c.ID != "hidden" {
			t.Errorf("card %d ID should be \"hidden\" for opponent, got %q", i, c.ID)
		}
		if c.Suit != "" {
			t.Errorf("card %d Suit should be empty for opponent, got %q", i, c.Suit)
		}
		if c.Rank != "" {
			t.Errorf("card %d Rank should be empty for opponent, got %q", i, c.Rank)
		}
	}
	// OwnerID in the masked view must still reflect the real owner
	if masked.OwnerID != "alice" {
		t.Errorf("masked OwnerID should be alice, got %q", masked.OwnerID)
	}
}

func TestMaskFor_DoesNotMutateOriginal(t *testing.T) {
	h := &Hand{OwnerID: "alice"}
	h.Add(Card{Suit: "hearts", Rank: "A", ID: "hearts-A"})
	_ = h.MaskFor("bob")
	if h.Cards[0].Suit != "hearts" {
		t.Error("MaskFor must not mutate the original hand")
	}
}

func TestMaskFor_EmptyHand(t *testing.T) {
	h := &Hand{OwnerID: "alice"}
	masked := h.MaskFor("bob")
	if len(masked.Cards) != 0 {
		t.Errorf("expected 0 cards, got %d", len(masked.Cards))
	}
}

// ---- Edge-case tests --------------------------------------------------------

func TestDeal_ZeroCards(t *testing.T) {
	d := NewDeck(standardDeck())
	cards, err := d.Deal(0)
	if err != nil {
		t.Fatalf("Deal(0) unexpected error: %v", err)
	}
	if len(cards) != 0 {
		t.Errorf("Deal(0) expected 0 cards, got %d", len(cards))
	}
	if d.Size() != 52 {
		t.Errorf("deck size should not change after Deal(0), got %d", d.Size())
	}
}

func TestDeal_EmptyDeck(t *testing.T) {
	d := NewDeck(nil)
	_, err := d.Deal(1)
	if !errors.Is(err, ErrInsufficientCards) {
		t.Fatalf("expected ErrInsufficientCards dealing from empty deck, got %v", err)
	}
}

func TestDeck_ResetAfterDeal(t *testing.T) {
	d := NewDeck(standardDeck())
	_, _ = d.Deal(10)
	d.Reset()
	if d.Size() != 52 {
		t.Errorf("after Reset size should be 52, got %d", d.Size())
	}
}

func TestDeck_ShuffleEmptyDeck(t *testing.T) {
	d := NewDeck(nil)
	// Should not panic on empty deck.
	d.Shuffle(rand.New(rand.NewSource(1)))
	if d.Size() != 0 {
		t.Errorf("size should remain 0 after shuffling empty deck, got %d", d.Size())
	}
}

func TestDeck_ShuffleSingleCard(t *testing.T) {
	d := NewDeck([]Card{{Suit: "hearts", Rank: "A", ID: "A"}})
	d.Shuffle(rand.New(rand.NewSource(1)))
	if d.Size() != 1 {
		t.Errorf("size should remain 1 after shuffling single card deck, got %d", d.Size())
	}
}

func TestDealTo_SingleHand(t *testing.T) {
	d := NewDeck(standardDeck())
	h := &Hand{OwnerID: "solo"}
	if err := d.DealTo([]*Hand{h}, 5); err != nil {
		t.Fatalf("DealTo single hand: %v", err)
	}
	if len(h.Cards) != 5 {
		t.Errorf("expected 5 cards, got %d", len(h.Cards))
	}
}

func TestDealTo_ZeroCards(t *testing.T) {
	d := NewDeck(standardDeck())
	h := &Hand{OwnerID: "p"}
	if err := d.DealTo([]*Hand{h}, 0); err != nil {
		t.Fatalf("DealTo(0) unexpected error: %v", err)
	}
	if len(h.Cards) != 0 {
		t.Errorf("expected 0 cards dealt, got %d", len(h.Cards))
	}
}

func TestHand_RemoveFromMiddle(t *testing.T) {
	h := &Hand{OwnerID: "player"}
	h.Add(
		Card{ID: "a"},
		Card{ID: "b"},
		Card{ID: "c"},
	)
	removed, err := h.Remove("b")
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if removed.ID != "b" {
		t.Errorf("removed card ID = %q, want %q", removed.ID, "b")
	}
	if len(h.Cards) != 2 {
		t.Errorf("expected 2 cards remaining, got %d", len(h.Cards))
	}
	if h.Cards[0].ID != "a" || h.Cards[1].ID != "c" {
		t.Errorf("remaining cards = [%s, %s], want [a, c]", h.Cards[0].ID, h.Cards[1].ID)
	}
}

func TestNewDeck_IndependentCopy(t *testing.T) {
	// Verify that mutating the original slice does not affect the deck.
	orig := standardDeck()
	d := NewDeck(orig)
	orig[0].Suit = "MUTATED"
	if d.cards[0].Suit == "MUTATED" {
		t.Error("NewDeck should copy the slice — mutation of original must not affect deck")
	}
}

// ---- Benchmarks -------------------------------------------------------------

// BenchmarkShuffle measures Fisher-Yates shuffle performance on a 52-card deck.
func BenchmarkShuffle(b *testing.B) {
	d := NewDeck(standardDeck())
	rng := rand.New(rand.NewSource(42))
	b.ResetTimer()
	for range b.N {
		d.Reset()
		d.Shuffle(rng)
	}
}
