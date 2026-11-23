// Package user exercises every way a reference can be classified.
package user

import (
	"io"
	"strings"

	"example.com/fixture/shapes"
)

// defaultSquare uses Square in a var type position.
var defaultSquare shapes.Square // ref:square-var

// Holder embeds Square in a struct, which also gives it Square's Area method.
type Holder struct {
	shapes.Square // ref:square-embed-struct
}

// ShapeSet embeds Shape in an interface.
type ShapeSet interface {
	shapes.Shape // ref:shape-embed-interface
}

// holderIsShape uses Shape in a type position.
var holderIsShape shapes.Shape = Holder{} // ref:shape-var

// Measure takes Square as a parameter.
func Measure(sq shapes.Square) int { // ref:square-param
	return sq.Area() // ref:area-call-on-param
}

// apply takes a function so NewSquare can be passed as a value.
func apply(build func(int) shapes.Square, size int) int { // ref:square-func-param
	return build(size).Area() // ref:area-call-on-result
}

// Use touches every remaining kind of reference.
func Use(reader io.Reader) int { // ref:reader-param
	square := shapes.NewSquare(2)              // ref:newsquare-call
	area := square.Area()                      // ref:area-call
	areaLater := square.Area                   // ref:area-method-value
	unit := shapes.Unit(area)                  // ref:unit-conversion
	copied := shapes.Square(defaultSquare)     // ref:square-conversion
	squares := []shapes.Square{square, copied} // ref:square-slice
	literal := shapes.Square{Side: 1}          // ref:square-composite
	built := apply(shapes.NewSquare, 3)        // ref:newsquare-passed
	maker := shapes.NewSquare                  // ref:newsquare-assigned
	small := shapes.Biggest(1, 2)              // ref:biggest-int
	large := shapes.Biggest(1.5, 2.5)          // ref:biggest-float
	limit := shapes.MaxSide                    // ref:maxside-value
	circle := &shapes.Circle{Radius: 1}
	round := circle.Area() // ref:circle-area-call
	var holder Holder
	promoted := holder.Area() // ref:area-promoted-call
	side := literal.Side      // ref:side-selected
	_ = maker
	_ = holderIsShape
	return area + areaLater() + int(unit) + len(squares) + built + small + int(large) + limit + round + promoted + side
}

// Shout uses a standard library function.
func Shout(word string) string {
	return strings.ToUpper(word) // ref:toupper-call
}
