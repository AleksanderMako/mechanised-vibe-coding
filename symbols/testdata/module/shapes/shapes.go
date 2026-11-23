// Package shapes declares the symbols the tests look up.
package shapes

import "io"

// Shape is implemented by Square through a value receiver and by Circle through a pointer receiver.
type Shape interface {
	Area() int
}

// Anything is the empty interface, so every named type implements it.
type Anything interface{}

// Square implements Shape with a value receiver.
type Square struct {
	Side int
}

// Area is a value receiver method.
func (s Square) Area() int { // ref:square-receiver
	side := s.Side // ref:side-in-method
	return side * side
}

// Circle implements Shape only as *Circle.
type Circle struct {
	Radius int
}

// Area is a pointer receiver method.
func (c *Circle) Area() int {
	return 3 * c.Radius * c.Radius
}

// Point does not implement Shape.
type Point struct {
	X int
	Y int
}

// Figure is an alias, so Implementations must not list it.
type Figure = Square // ref:square-alias

// Unit is a named type that is not a struct.
type Unit int

// MaxSide is a constant.
const MaxSide = 100

// NewSquare is a plain function.
func NewSquare(side int) Square { // ref:square-result
	return Square{Side: side} // ref:square-literal-in-shapes
}

// Biggest is generic so both of its instantiations count as references.
func Biggest[T int | float64](a, b T) T {
	if a > b {
		return a
	}
	return b
}

// Tagged has one field per json tag form the contract lists.
type Tagged struct {
	Name   string `json:"name"`
	Secret string `json:"-"`
	Note   string `json:",omitempty"`
	Count  int    `json:"count,omitempty"`
	Plain  bool
	Column string `db:"column_name"`
	Square        // ref:square-embed
	*Circle
	io.Reader                  // ref:reader-embed
	Scores    map[string][]int `json:"scores" db:"scores"`
	hidden    int              `db:"hidden"`
}
