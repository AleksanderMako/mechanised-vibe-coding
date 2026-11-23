// Package lib is the healthy source package of every refactoring.
package lib

// Hello greets name.
func Hello(name string) string {
	return "hello " + name
}

// Adder ignores its receiver so Sum can become a function.
type Adder struct{}

// Sum returns one.
func (Adder) Sum() int {
	return 1
}
