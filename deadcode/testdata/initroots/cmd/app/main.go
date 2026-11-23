// Command app imports plugin only for the side effects of its init.
package main

import (
	_ "example.com/fixture/plugin"
	"example.com/fixture/registry"
)

func main() {
	println(registry.Describe())
}

// marker:_
func _() {}
