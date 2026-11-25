// Package store declares the types whose method sets are looked up.
package store

import "io"

// Base is embedded in Handle; its Close is promoted and so not part of Handle.*.
type Base struct{}

// Close is declared on Base.
func (Base) Close() error { return nil }

// Handle has methods on Handle and *Handle and embeds Base.
type Handle struct {
	Base
	name string
}

// Exec is declared with a value receiver.
func (h Handle) Exec(query string) error { return nil }

// Query is declared with a pointer receiver.
func (h *Handle) Query(query string) string { return h.name + query }

// Querier embeds io.Reader, so Querier.* includes io.Reader's Read.
type Querier interface {
	io.Reader
	Query(query string) string
}

// Plain is a type without methods.
type Plain struct{ N int }

// Empty is an interface without methods.
type Empty interface{}

// Box is generic; Box.* uses the origin type.
type Box[T any] struct{ value T }

// Get is declared on the generic type.
func (b Box[T]) Get() T { return b.value }

// Open is a function, not a type.
func Open() *Handle { return &Handle{} }

// Default is a variable, not a type.
var Default = &Handle{}

// Limit is a constant, not a type.
const Limit = 3
