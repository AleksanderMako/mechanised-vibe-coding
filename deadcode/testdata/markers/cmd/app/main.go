// Command app uses shapes without ever calling a marker method.
package main

import "example.com/fixture/shapes"

func main() {
	println(shapes.Count())
}
