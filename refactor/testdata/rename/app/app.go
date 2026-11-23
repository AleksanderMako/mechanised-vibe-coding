// Package app uses shapes from another package.
package app

import "example.com/fixture/shapes"

// Build makes a rectangle and reports its area and a box width.
func Build() (int, int) {
	rect := shapes.Rect{Width: 2, Height: 3}
	bigger := rect.Scale(2)
	box := shapes.Box{Width: 7}
	return shapes.Area(bigger.Width, bigger.Height), box.Width + box.Size()
}
