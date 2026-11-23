// Package plugin is imported by main only for its init, which is reached through that import.
package plugin

import "example.com/fixture/registry"

type greeter struct{}

// marker:greeter.Name
func (greeter) Name() string {
	return prefix() + "greeter"
}

// marker:init
func init() {
	registry.Register(greeter{})
}

// marker:prefix
func prefix() string {
	return "the "
}

// marker:NotRegistered
func NotRegistered() int {
	return 1
}
