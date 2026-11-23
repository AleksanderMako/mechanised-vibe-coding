// Package shapes is the fixture package the mvc tests look up, edit and rename.
package shapes

// Version is the fixture version.
const Version = "1.0"

// Shape has an area.
type Shape interface {
	Area() int
}

// Square is a shape with equal sides.
type Square struct {
	Side  int    `json:"side"`
	Label string `json:"label,omitempty"`
}

// Area returns the area.
func (s Square) Area() int {
	return s.Side * s.Side
}

// NewSquare builds a square.
func NewSquare(side int) Square {
	return Square{Side: side}
}

// Abandoned is called by nobody, so dead code analysis reports it.
func Abandoned() int {
	return 0
}
