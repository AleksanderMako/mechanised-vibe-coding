// Package orphan is imported by no main package, so even what its init runs is dead.
package orphan

import "example.com/fixture/registry"

type ghost struct{}

// marker:ghost.Name
func (ghost) Name() string {
	return "ghost"
}

var description string

// marker:init#1
func init() {
	setUp()
	var namer registry.Namer = ghost{}
	description = describe(namer)
	registry.Register(namer)
}

// marker:setUp
func setUp() {
	description = "set up"
}

// marker:describe
func describe(namer registry.Namer) string {
	return namer.Name()
}

// marker:Exported
func Exported() string {
	return description
}

// marker:init#2
func init() {
	description += " twice"
}
