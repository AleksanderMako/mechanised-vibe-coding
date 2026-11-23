// Package symbols answers "who uses this, how, and what does its shape say"
// with full type information, so the answer is complete where grep is not.
package symbols

import (
	"context"
	"errors"
)

// Symbol names a package level declaration, a method or a field.
type Symbol struct {
	Package string // import path
	Name    string // "Name", "Type.Method" or "Type.Field"
}

// String renders the symbol as "import/path.Name" or "import/path.Type.Member".
func (s Symbol) String() string { panic("not implemented") }

// ParseSymbol reads "import/path.Name" or "import/path.Type.Member".
func ParseSymbol(s string) (Symbol, error) { panic("not implemented") }

// Options controls loading. The zero value loads ./... with tests.
type Options struct {
	Patterns     []string // package patterns relative to Root; empty means ./...
	ExcludeTests bool     // leave _test.go files out
}

// Kind says how a reference uses the symbol.
type Kind string

// The reference kinds.
const (
	KindCall       Kind = "call"
	KindConversion Kind = "conversion"
	KindType       Kind = "type"
	KindEmbed      Kind = "embed"
	KindValue      Kind = "value"
)

// Reference is one use of a symbol.
type Reference struct {
	Package string // import path of the package the use is in
	File    string // absolute path
	Line    int
	Column  int
	Kind    Kind
	Test    bool   // the use is in a _test.go file
	Context string // the source line, trimmed
}

// Sentinel errors wrapped by the lookups.
var (
	ErrNotFound     = errors.New("symbol not found")
	ErrNotInterface = errors.New("symbol is not an interface")
	ErrNotStruct    = errors.New("symbol is not a struct")
)

// References lists every use of sym except its declaration, sorted and unique.
func References(ctx context.Context, root string, sym Symbol, opts Options) ([]Reference, error) {
	panic("not implemented")
}

// Implementations lists the named non-interface types in the loaded packages that satisfy iface.
func Implementations(ctx context.Context, root string, iface Symbol, opts Options) ([]Symbol, error) {
	panic("not implemented")
}

// FieldTag describes one field of a struct type.
type FieldTag struct {
	Field     string // field name; the type name for embedded fields
	Type      string // the type as written in source
	Embedded  bool
	Exported  bool
	Tag       string // the raw tag without back quotes
	JSON      string // json name part, "-" when excluded, "" when absent
	OmitEmpty bool   // json tag has omitempty
}

// StructTags lists the fields of struct type typ in declaration order.
func StructTags(ctx context.Context, root string, typ Symbol, opts Options) ([]FieldTag, error) {
	panic("not implemented")
}
