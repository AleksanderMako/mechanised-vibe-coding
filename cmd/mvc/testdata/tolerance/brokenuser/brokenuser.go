// Package brokenuser imports lib and calls Hello, but has a type error in another statement.
package brokenuser

import "example.com/fixture/lib"

// Oops has a type error on purpose: undefinedName is not declared anywhere.
var Oops = undefinedName

// Use calls Hello in a statement the type checker can resolve.
func Use() string {
	return lib.Hello("user")
}
