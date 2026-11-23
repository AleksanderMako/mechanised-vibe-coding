// Package brokenuser imports lib and uses it, but has a type error in another statement.
package brokenuser

import "example.com/fixture/lib"

// Oops has a type error on purpose: undefinedName is not declared anywhere.
var Oops = undefinedName

// Use references lib in statements the type checker can still resolve.
func Use() string {
	var config lib.Config         // ref:broken-config
	return lib.Hello(config.Name) // ref:broken-hello
}

// UserSpeaker type-checks although its package is broken.
type UserSpeaker struct{}

// Speak makes UserSpeaker a Speaker.
func (UserSpeaker) Speak() string {
	return "user"
}
