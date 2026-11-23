// Package astedit edits Go files by symbol on a comment preserving syntax tree,
// so an edit cannot land in the wrong declaration and anchors never go stale.
package astedit

import (
	"context"
	"errors"

	"github.com/dave/dst"
)

// Op is one structural edit on one Go file. Only this package implements it.
type Op interface {
	// File is the path the op edits, as given to the constructor.
	File() string
	// Describe is a one line human description, for logs.
	Describe() string
	apply(f *dst.File) error
}

// Sentinel errors; errors from Apply wrap one of these and name the op.
var (
	ErrNotFound  = errors.New("target not found")
	ErrAmbiguous = errors.New("more than one target matches")
	ErrExists    = errors.New("target already exists")
	ErrBadSource = errors.New("source snippet does not parse")
)

// AddTableCase appends caseSrc as a new element of the table in testFunc.
func AddTableCase(file, testFunc, caseSrc string) Op { panic("not implemented") }

// AddStructField inserts fieldSrc into struct typeName, after the named field or at the end.
func AddStructField(file, typeName, fieldSrc, after string) Op { panic("not implemented") }

// RemoveStructField removes fieldName from struct typeName.
func RemoveStructField(file, typeName, fieldName string) Op { panic("not implemented") }

// SetDoc replaces the doc comment of symbol ("Name" or "Type.Method") with doc.
func SetDoc(file, symbol, doc string) Op { panic("not implemented") }

// SetConst replaces the value of constant name with valueSrc and, when doc is set, its doc comment.
func SetConst(file, name, valueSrc, doc string) Op { panic("not implemented") }

// AddConformance adds `var _ Iface = (*TypeName)(nil)` after the type declaration.
func AddConformance(file, typeName, ifaceSrc string) Op { panic("not implemented") }

// Apply applies all ops in memory first and then writes every changed file formatted.
func Apply(ctx context.Context, root string, ops ...Op) ([]string, error) {
	panic("not implemented")
}
