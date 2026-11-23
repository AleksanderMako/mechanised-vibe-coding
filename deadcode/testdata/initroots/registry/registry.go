// Package registry holds what plugin registers from its init.
package registry

// Namer is the interface plugin registers a value as.
type Namer interface {
	Name() string
}

var registered []Namer

// marker:Register
func Register(namer Namer) {
	registered = append(registered, namer)
}

// marker:Describe
func Describe() string {
	description := ""
	for _, namer := range registered {
		description += namer.Name()
	}
	return description
}

// A blank function can never be called, even in a package main uses.
// marker:_
func _() {}

// marker:Forget
func Forget() {
	registered = nil
}
