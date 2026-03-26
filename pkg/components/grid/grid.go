// Package grid provides generic 2D and 3D grid data structures, distance
// functions, and an occupancy map for spatial game-state management.
package grid

import (
	"errors"
	"fmt"
	"math"
)

// ---- Sentinel errors --------------------------------------------------------

// ErrOutOfBounds is returned by Get/Set when a position lies outside the grid.
var ErrOutOfBounds = errors.New("grid: position out of bounds")

// ErrAlreadyOccupied is returned by [OccupancyMap.Occupy] when the cell is not empty.
var ErrAlreadyOccupied = errors.New("grid: cell already occupied")

// ErrNotOccupied is returned by [OccupancyMap.Vacate] when the cell is already empty.
var ErrNotOccupied = errors.New("grid: cell not occupied")

// =============================================================================
// 2D
// =============================================================================

// Vec2 is an integer 2D position.
type Vec2 struct{ X, Y int }

// Grid2D is a generic, flat-backed 2D grid.
// The element at (x, y) is stored at index y*Width + x.
type Grid2D[T any] struct {
	Width, Height int
	cells         []T
}

// NewGrid2D allocates a zero-valued Width × Height grid.
func NewGrid2D[T any](width, height int) *Grid2D[T] {
	return &Grid2D[T]{Width: width, Height: height, cells: make([]T, width*height)}
}

// InBounds reports whether pos lies within the grid.
func (g *Grid2D[T]) InBounds(pos Vec2) bool {
	return pos.X >= 0 && pos.X < g.Width && pos.Y >= 0 && pos.Y < g.Height
}

// Get returns the value stored at pos, or [ErrOutOfBounds] if pos is outside the grid.
func (g *Grid2D[T]) Get(pos Vec2) (T, error) {
	if !g.InBounds(pos) {
		var zero T
		return zero, fmt.Errorf("%w: (%d, %d) not in %dx%d grid", ErrOutOfBounds, pos.X, pos.Y, g.Width, g.Height)
	}
	return g.cells[pos.Y*g.Width+pos.X], nil
}

// Set stores val at pos, or returns [ErrOutOfBounds] if pos is outside the grid.
func (g *Grid2D[T]) Set(pos Vec2, val T) error {
	if !g.InBounds(pos) {
		return fmt.Errorf("%w: (%d, %d) not in %dx%d grid", ErrOutOfBounds, pos.X, pos.Y, g.Width, g.Height)
	}
	g.cells[pos.Y*g.Width+pos.X] = val
	return nil
}

// Neighbors4 returns the up-to-4 cardinal neighbours of pos that are in bounds.
func (g *Grid2D[T]) Neighbors4(pos Vec2) []Vec2 {
	dirs := [4]Vec2{{0, -1}, {0, 1}, {-1, 0}, {1, 0}}
	out := make([]Vec2, 0, 4)
	for _, d := range dirs {
		n := Vec2{pos.X + d.X, pos.Y + d.Y}
		if g.InBounds(n) {
			out = append(out, n)
		}
	}
	return out
}

// Neighbors8 returns the up-to-8 cardinal and diagonal neighbours of pos that
// are in bounds.
func (g *Grid2D[T]) Neighbors8(pos Vec2) []Vec2 {
	out := make([]Vec2, 0, 8)
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 {
				continue
			}
			n := Vec2{pos.X + dx, pos.Y + dy}
			if g.InBounds(n) {
				out = append(out, n)
			}
		}
	}
	return out
}

// =============================================================================
// 3D
// =============================================================================

// Vec3 is an integer 3D position.
type Vec3 struct{ X, Y, Z int }

// Grid3D is a generic, flat-backed 3D grid.
// The element at (x, y, z) is stored at index z*Width*Height + y*Width + x.
type Grid3D[T any] struct {
	Width, Height, Depth int
	cells                []T
}

// NewGrid3D allocates a zero-valued Width × Height × Depth grid.
func NewGrid3D[T any](width, height, depth int) *Grid3D[T] {
	return &Grid3D[T]{Width: width, Height: height, Depth: depth, cells: make([]T, width*height*depth)}
}

// InBounds reports whether pos lies within the grid.
func (g *Grid3D[T]) InBounds(pos Vec3) bool {
	return pos.X >= 0 && pos.X < g.Width &&
		pos.Y >= 0 && pos.Y < g.Height &&
		pos.Z >= 0 && pos.Z < g.Depth
}

// Get returns the value stored at pos, or [ErrOutOfBounds] if pos is outside the grid.
func (g *Grid3D[T]) Get(pos Vec3) (T, error) {
	if !g.InBounds(pos) {
		var zero T
		return zero, fmt.Errorf("%w: (%d,%d,%d) not in %dx%dx%d grid",
			ErrOutOfBounds, pos.X, pos.Y, pos.Z, g.Width, g.Height, g.Depth)
	}
	return g.cells[pos.Z*g.Width*g.Height+pos.Y*g.Width+pos.X], nil
}

// Set stores val at pos, or returns [ErrOutOfBounds] if pos is outside the grid.
func (g *Grid3D[T]) Set(pos Vec3, val T) error {
	if !g.InBounds(pos) {
		return fmt.Errorf("%w: (%d,%d,%d) not in %dx%dx%d grid",
			ErrOutOfBounds, pos.X, pos.Y, pos.Z, g.Width, g.Height, g.Depth)
	}
	g.cells[pos.Z*g.Width*g.Height+pos.Y*g.Width+pos.X] = val
	return nil
}

// =============================================================================
// Distance functions
// =============================================================================

// ManhattanDistance2D returns the L1 distance between a and b on a 2D grid.
func ManhattanDistance2D(a, b Vec2) int {
	dx := a.X - b.X
	if dx < 0 {
		dx = -dx
	}
	dy := a.Y - b.Y
	if dy < 0 {
		dy = -dy
	}
	return dx + dy
}

// EuclideanDistance2D returns the straight-line (L2) distance between a and b.
func EuclideanDistance2D(a, b Vec2) float64 {
	dx := float64(a.X - b.X)
	dy := float64(a.Y - b.Y)
	return math.Sqrt(dx*dx + dy*dy)
}

// ManhattanDistance3D returns the L1 distance between a and b in 3D space.
func ManhattanDistance3D(a, b Vec3) int {
	dx := a.X - b.X
	if dx < 0 {
		dx = -dx
	}
	dy := a.Y - b.Y
	if dy < 0 {
		dy = -dy
	}
	dz := a.Z - b.Z
	if dz < 0 {
		dz = -dz
	}
	return dx + dy + dz
}

// EuclideanDistance3D returns the straight-line (L2) distance between a and b
// in 3D space.
func EuclideanDistance3D(a, b Vec3) float64 {
	dx := float64(a.X - b.X)
	dy := float64(a.Y - b.Y)
	dz := float64(a.Z - b.Z)
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// ChebyshevDistance2D returns the chessboard (L∞) distance between a and b.
// This equals the number of king moves required to travel between the two cells.
func ChebyshevDistance2D(a, b Vec2) int {
	dx := a.X - b.X
	if dx < 0 {
		dx = -dx
	}
	dy := a.Y - b.Y
	if dy < 0 {
		dy = -dy
	}
	if dx > dy {
		return dx
	}
	return dy
}

// =============================================================================
// OccupancyMap
// =============================================================================

// OccupancyMap is a thin wrapper over Grid2D[string] where an empty string
// signifies an unoccupied cell. Entity IDs must be non-empty strings.
type OccupancyMap struct {
	g *Grid2D[string]
}

// NewOccupancyMap allocates an empty occupancy map of the given dimensions.
func NewOccupancyMap(width, height int) *OccupancyMap {
	return &OccupancyMap{g: NewGrid2D[string](width, height)}
}

// Occupy marks pos as occupied by entityID.
// Returns [ErrOutOfBounds] if pos is outside the map.
// Returns [ErrAlreadyOccupied] if the cell is not empty.
func (m *OccupancyMap) Occupy(pos Vec2, entityID string) error {
	cur, err := m.g.Get(pos)
	if err != nil {
		return err
	}
	if cur != "" {
		return fmt.Errorf("%w: pos (%d,%d) is occupied by %q", ErrAlreadyOccupied, pos.X, pos.Y, cur)
	}
	return m.g.Set(pos, entityID)
}

// Vacate clears the occupant at pos.
// Returns [ErrOutOfBounds] if pos is outside the map.
// Returns [ErrNotOccupied] if the cell is already empty.
func (m *OccupancyMap) Vacate(pos Vec2) error {
	cur, err := m.g.Get(pos)
	if err != nil {
		return err
	}
	if cur == "" {
		return fmt.Errorf("%w: pos (%d,%d)", ErrNotOccupied, pos.X, pos.Y)
	}
	return m.g.Set(pos, "")
}

// IsOccupied reports whether pos holds an entity.
func (m *OccupancyMap) IsOccupied(pos Vec2) bool {
	v, err := m.g.Get(pos)
	return err == nil && v != ""
}

// OccupiedBy returns the entity ID at pos and true if occupied, or ("", false)
// if the cell is empty or out of bounds.
func (m *OccupancyMap) OccupiedBy(pos Vec2) (string, bool) {
	v, err := m.g.Get(pos)
	if err != nil || v == "" {
		return "", false
	}
	return v, true
}

// AllOccupied returns all positions that currently contain an entity, in
// row-major order (y ascending, then x ascending).
func (m *OccupancyMap) AllOccupied() []Vec2 {
	var out []Vec2
	for y := 0; y < m.g.Height; y++ {
		for x := 0; x < m.g.Width; x++ {
			pos := Vec2{x, y}
			if m.IsOccupied(pos) {
				out = append(out, pos)
			}
		}
	}
	return out
}
