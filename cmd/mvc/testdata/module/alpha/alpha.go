// Package alpha uses shapes so references are spread over several packages.
package alpha

import "example.com/fixture/shapes"

// Alpha returns the area of a small square.
func Alpha() int {
	return shapes.NewSquare(2).Area()
}
