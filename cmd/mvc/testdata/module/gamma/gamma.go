// Package gamma uses shapes so references are spread over several packages.
package gamma

import "example.com/fixture/shapes"

// Gamma returns the area of a small square.
func Gamma() int {
	return shapes.NewSquare(2).Area()
}
