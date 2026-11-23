// Package beta uses shapes so references are spread over several packages.
package beta

import "example.com/fixture/shapes"

// Beta returns the area of a small square.
func Beta() int {
	return shapes.NewSquare(2).Area()
}
