// Package shapes is a fixture for the astedit tests.
package shapes

import "io"

// Version is the fixture version.
const Version = "1.0"

// Colors used by shapes.
const (
	// Red is the first color.
	Red = iota
	Green
	Blue // the last color
)

const Width, Height = 10, 20

// Count is a variable, not a constant.
var Count = 3

// Point is a location.
type Point struct {
	// X is horizontal.
	X int // x line comment
	Y int `json:"y"`
}

// Empty has no fields.
type Empty struct{}

// Single has one field.
type Single struct {
	// Only is documented.
	Only string // the only field
}

// Name is not a struct.
type Name string

// Shape has an area.
type Shape interface {
	Area() float64
}

type (
	// Circle is round.
	Circle struct {
		R float64
	}
	// Triangle has three sides.
	Triangle struct {
		A, B, C float64
	}
)

// Square is a shape.
type Square struct {
	Side float64
}

// Area returns the area.
func (s Square) Area() float64 {
	// multiply the side by itself
	return s.Side * s.Side
}

// Close closes the square.
func (s *Square) Close() error { return nil }

var _ io.Closer = (*Square)(nil)

func Helper() int { return 1 }

func outer() {
	type Inner struct{ A int }
	_ = Inner{}
}
