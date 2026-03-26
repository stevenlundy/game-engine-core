package grid

import (
	"errors"
	"math"
	"testing"
)

// =============================================================================
// Vec2 / Grid2D
// =============================================================================

func TestGrid2D_InBounds(t *testing.T) {
	g := NewGrid2D[int](5, 4)
	tests := []struct {
		pos     Vec2
		inBound bool
	}{
		{Vec2{0, 0}, true},
		{Vec2{4, 3}, true},
		{Vec2{5, 0}, false},
		{Vec2{0, 4}, false},
		{Vec2{-1, 0}, false},
		{Vec2{0, -1}, false},
		{Vec2{2, 2}, true},
	}
	for _, tc := range tests {
		if got := g.InBounds(tc.pos); got != tc.inBound {
			t.Errorf("InBounds(%v) = %v, want %v", tc.pos, got, tc.inBound)
		}
	}
}

func TestGrid2D_SetGet(t *testing.T) {
	g := NewGrid2D[string](3, 3)
	pos := Vec2{1, 2}
	if err := g.Set(pos, "hello"); err != nil {
		t.Fatalf("Set error: %v", err)
	}
	val, err := g.Get(pos)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if val != "hello" {
		t.Errorf("Get = %q, want %q", val, "hello")
	}
}

func TestGrid2D_GetOutOfBounds(t *testing.T) {
	g := NewGrid2D[int](3, 3)
	_, err := g.Get(Vec2{5, 5})
	if !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("expected ErrOutOfBounds, got %v", err)
	}
}

func TestGrid2D_SetOutOfBounds(t *testing.T) {
	g := NewGrid2D[int](3, 3)
	err := g.Set(Vec2{-1, 0}, 42)
	if !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("expected ErrOutOfBounds, got %v", err)
	}
}

func TestGrid2D_Neighbors4_Interior(t *testing.T) {
	g := NewGrid2D[int](5, 5)
	n := g.Neighbors4(Vec2{2, 2})
	if len(n) != 4 {
		t.Errorf("interior Neighbors4 = %d, want 4", len(n))
	}
}

func TestGrid2D_Neighbors4_Corner(t *testing.T) {
	g := NewGrid2D[int](5, 5)
	n := g.Neighbors4(Vec2{0, 0})
	if len(n) != 2 {
		t.Errorf("corner Neighbors4 = %d, want 2", len(n))
	}
}

func TestGrid2D_Neighbors4_Edge(t *testing.T) {
	g := NewGrid2D[int](5, 5)
	n := g.Neighbors4(Vec2{0, 2}) // left edge
	if len(n) != 3 {
		t.Errorf("edge Neighbors4 = %d, want 3", len(n))
	}
}

func TestGrid2D_Neighbors8_Interior(t *testing.T) {
	g := NewGrid2D[int](5, 5)
	n := g.Neighbors8(Vec2{2, 2})
	if len(n) != 8 {
		t.Errorf("interior Neighbors8 = %d, want 8", len(n))
	}
}

func TestGrid2D_Neighbors8_Corner(t *testing.T) {
	g := NewGrid2D[int](5, 5)
	n := g.Neighbors8(Vec2{0, 0})
	if len(n) != 3 {
		t.Errorf("corner Neighbors8 = %d, want 3", len(n))
	}
}

func TestGrid2D_Neighbors8_Edge(t *testing.T) {
	g := NewGrid2D[int](5, 5)
	n := g.Neighbors8(Vec2{0, 2}) // left edge
	if len(n) != 5 {
		t.Errorf("edge Neighbors8 = %d, want 5", len(n))
	}
}

// =============================================================================
// Vec3 / Grid3D
// =============================================================================

func TestGrid3D_InBounds(t *testing.T) {
	g := NewGrid3D[int](4, 3, 2)
	tests := []struct {
		pos     Vec3
		inBound bool
	}{
		{Vec3{0, 0, 0}, true},
		{Vec3{3, 2, 1}, true},
		{Vec3{4, 0, 0}, false},
		{Vec3{0, 3, 0}, false},
		{Vec3{0, 0, 2}, false},
		{Vec3{-1, 0, 0}, false},
	}
	for _, tc := range tests {
		if got := g.InBounds(tc.pos); got != tc.inBound {
			t.Errorf("InBounds(%v) = %v, want %v", tc.pos, got, tc.inBound)
		}
	}
}

func TestGrid3D_SetGet(t *testing.T) {
	g := NewGrid3D[int](4, 3, 2)
	pos := Vec3{2, 1, 1}
	if err := g.Set(pos, 99); err != nil {
		t.Fatalf("Set error: %v", err)
	}
	val, err := g.Get(pos)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if val != 99 {
		t.Errorf("Get = %d, want 99", val)
	}
}

func TestGrid3D_GetOutOfBounds(t *testing.T) {
	g := NewGrid3D[int](2, 2, 2)
	_, err := g.Get(Vec3{2, 0, 0})
	if !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("expected ErrOutOfBounds, got %v", err)
	}
}

func TestGrid3D_SetOutOfBounds(t *testing.T) {
	g := NewGrid3D[int](2, 2, 2)
	err := g.Set(Vec3{0, 0, 5}, 1)
	if !errors.Is(err, ErrOutOfBounds) {
		t.Fatalf("expected ErrOutOfBounds, got %v", err)
	}
}

// =============================================================================
// Distance functions (table-driven)
// =============================================================================

func TestManhattanDistance2D(t *testing.T) {
	tests := []struct {
		a, b Vec2
		want int
	}{
		{Vec2{0, 0}, Vec2{0, 0}, 0},
		{Vec2{0, 0}, Vec2{3, 4}, 7},
		{Vec2{1, 1}, Vec2{4, 5}, 7},
		{Vec2{5, 5}, Vec2{0, 0}, 10},
		{Vec2{-2, -3}, Vec2{2, 3}, 10},
	}
	for _, tc := range tests {
		if got := ManhattanDistance2D(tc.a, tc.b); got != tc.want {
			t.Errorf("ManhattanDistance2D(%v,%v) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestEuclideanDistance2D(t *testing.T) {
	tests := []struct {
		a, b Vec2
		want float64
	}{
		{Vec2{0, 0}, Vec2{0, 0}, 0},
		{Vec2{0, 0}, Vec2{3, 4}, 5},
		{Vec2{0, 0}, Vec2{1, 1}, math.Sqrt2},
	}
	for _, tc := range tests {
		got := EuclideanDistance2D(tc.a, tc.b)
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("EuclideanDistance2D(%v,%v) = %f, want %f", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestManhattanDistance3D(t *testing.T) {
	tests := []struct {
		a, b Vec3
		want int
	}{
		{Vec3{0, 0, 0}, Vec3{0, 0, 0}, 0},
		{Vec3{0, 0, 0}, Vec3{1, 2, 3}, 6},
		{Vec3{-1, -2, -3}, Vec3{1, 2, 3}, 12},
	}
	for _, tc := range tests {
		if got := ManhattanDistance3D(tc.a, tc.b); got != tc.want {
			t.Errorf("ManhattanDistance3D(%v,%v) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestEuclideanDistance3D(t *testing.T) {
	tests := []struct {
		a, b Vec3
		want float64
	}{
		{Vec3{0, 0, 0}, Vec3{0, 0, 0}, 0},
		{Vec3{0, 0, 0}, Vec3{1, 2, 2}, 3}, // sqrt(1+4+4)=3
		{Vec3{0, 0, 0}, Vec3{2, 3, 6}, 7}, // sqrt(4+9+36)=7
	}
	for _, tc := range tests {
		got := EuclideanDistance3D(tc.a, tc.b)
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("EuclideanDistance3D(%v,%v) = %f, want %f", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestChebyshevDistance2D(t *testing.T) {
	tests := []struct {
		a, b Vec2
		want int
	}{
		{Vec2{0, 0}, Vec2{0, 0}, 0},
		{Vec2{0, 0}, Vec2{3, 1}, 3}, // max(3,1)
		{Vec2{0, 0}, Vec2{1, 4}, 4}, // max(1,4)
		{Vec2{0, 0}, Vec2{3, 3}, 3}, // max(3,3)
		{Vec2{2, 2}, Vec2{5, 6}, 4}, // max(3,4)
	}
	for _, tc := range tests {
		if got := ChebyshevDistance2D(tc.a, tc.b); got != tc.want {
			t.Errorf("ChebyshevDistance2D(%v,%v) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

// =============================================================================
// OccupancyMap
// =============================================================================

func TestOccupancyMap_OccupyAndVacate(t *testing.T) {
	m := NewOccupancyMap(5, 5)
	pos := Vec2{2, 3}

	if m.IsOccupied(pos) {
		t.Fatal("fresh map cell should be empty")
	}

	if err := m.Occupy(pos, "player1"); err != nil {
		t.Fatalf("Occupy error: %v", err)
	}
	if !m.IsOccupied(pos) {
		t.Fatal("cell should be occupied after Occupy")
	}

	id, ok := m.OccupiedBy(pos)
	if !ok || id != "player1" {
		t.Errorf("OccupiedBy = (%q, %v), want (\"player1\", true)", id, ok)
	}

	if err := m.Vacate(pos); err != nil {
		t.Fatalf("Vacate error: %v", err)
	}
	if m.IsOccupied(pos) {
		t.Fatal("cell should be empty after Vacate")
	}
}

func TestOccupancyMap_DoubleOccupy(t *testing.T) {
	m := NewOccupancyMap(3, 3)
	pos := Vec2{1, 1}
	if err := m.Occupy(pos, "a"); err != nil {
		t.Fatal(err)
	}
	err := m.Occupy(pos, "b")
	if !errors.Is(err, ErrAlreadyOccupied) {
		t.Fatalf("expected ErrAlreadyOccupied on double Occupy, got %v", err)
	}
}

func TestOccupancyMap_DoubleVacate(t *testing.T) {
	m := NewOccupancyMap(3, 3)
	pos := Vec2{1, 1}
	if err := m.Occupy(pos, "a"); err != nil {
		t.Fatal(err)
	}
	if err := m.Vacate(pos); err != nil {
		t.Fatal(err)
	}
	err := m.Vacate(pos)
	if !errors.Is(err, ErrNotOccupied) {
		t.Fatalf("expected ErrNotOccupied on double Vacate, got %v", err)
	}
}

func TestOccupancyMap_OutOfBounds(t *testing.T) {
	m := NewOccupancyMap(3, 3)
	oob := Vec2{10, 10}
	if err := m.Occupy(oob, "x"); !errors.Is(err, ErrOutOfBounds) {
		t.Errorf("Occupy OOB: expected ErrOutOfBounds, got %v", err)
	}
	if err := m.Vacate(oob); !errors.Is(err, ErrOutOfBounds) {
		t.Errorf("Vacate OOB: expected ErrOutOfBounds, got %v", err)
	}
}

func TestOccupancyMap_IsOccupied_OOB(t *testing.T) {
	m := NewOccupancyMap(3, 3)
	if m.IsOccupied(Vec2{99, 99}) {
		t.Error("IsOccupied on OOB pos should return false")
	}
}

func TestOccupancyMap_OccupiedBy_Empty(t *testing.T) {
	m := NewOccupancyMap(3, 3)
	id, ok := m.OccupiedBy(Vec2{0, 0})
	if ok || id != "" {
		t.Errorf("OccupiedBy on empty cell: got (%q, %v), want (\"\", false)", id, ok)
	}
}

func TestOccupancyMap_AllOccupied(t *testing.T) {
	m := NewOccupancyMap(3, 3)
	positions := []Vec2{{0, 0}, {1, 1}, {2, 2}}
	for _, p := range positions {
		if err := m.Occupy(p, "e"); err != nil {
			t.Fatal(err)
		}
	}
	all := m.AllOccupied()
	if len(all) != 3 {
		t.Fatalf("AllOccupied = %d, want 3", len(all))
	}
	// verify all expected positions are present
	set := make(map[Vec2]bool)
	for _, p := range all {
		set[p] = true
	}
	for _, p := range positions {
		if !set[p] {
			t.Errorf("position %v missing from AllOccupied", p)
		}
	}
}

func TestOccupancyMap_AllOccupied_Empty(t *testing.T) {
	m := NewOccupancyMap(4, 4)
	if all := m.AllOccupied(); len(all) != 0 {
		t.Errorf("expected 0 occupied cells, got %d", len(all))
	}
}

func TestOccupancyMap_AllOccupied_RowMajorOrder(t *testing.T) {
	m := NewOccupancyMap(3, 3)
	// Occupy in non-sorted order
	for _, p := range []Vec2{{2, 0}, {0, 1}, {1, 0}} {
		_ = m.Occupy(p, "x")
	}
	all := m.AllOccupied()
	// Expected row-major: (1,0), (2,0), (0,1)
	expected := []Vec2{{1, 0}, {2, 0}, {0, 1}}
	if len(all) != len(expected) {
		t.Fatalf("len = %d, want %d", len(all), len(expected))
	}
	for i, p := range all {
		if p != expected[i] {
			t.Errorf("all[%d] = %v, want %v", i, p, expected[i])
		}
	}
}
