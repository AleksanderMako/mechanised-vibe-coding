// Package bad holds a type error on purpose.
package bad

// Wrong assigns a string to an int, which the type checker rejects.
var Wrong int = "text"
