// Package shapes holds the declarations the rename tests rewrite.
package shapes

import "fmt"

// Area returns the area of a rectangle.
// Area is mentioned again on this second line.
func Area(width, height int) int {
	return width * height
}

// Rect is a rectangle.
type Rect struct {
	Width  int
	Height int
}

// Scale multiplies both sides by factor.
func (r Rect) Scale(factor int) Rect {
	return Rect{Width: r.Width * factor, Height: r.Height * factor}
}

// Perimeter is the method Shape asks for.
func (r Rect) Perimeter() int {
	return 2 * (r.Width + r.Height)
}

// Shape is used as a type below, so Rect must keep satisfying it.
type Shape interface {
	Perimeter() int
}

var _ Shape = Rect{}

// Box is a second struct with a Width field that a Rect.Width rename must not touch.
type Box struct {
	Width int
}

// Size is the method Sizer asks for.
func (b Box) Size() int {
	return b.Width
}

// Sizer is never used as a type, so Box may stop satisfying it.
type Sizer interface {
	Size() int
}

// Describe mentions Area in a comment and in a string.
func Describe(r Rect) string {
	// Area is computed below.
	return fmt.Sprintf("Area of rect: %d", Area(r.Width, r.Height))
}

// Internal is exported but only referenced inside this package.
func Internal() int {
	return 1
}

// helper is only referenced inside this package.
func helper() int {
	return 2
}

// Shadowed declares a local assist and then refers to helper.
func Shadowed() int {
	assist := 4
	return helper() + assist
}

// Unrelated declares a local Spare without referring to Area.
func Unrelated() int {
	Spare := 5
	return Spare
}
