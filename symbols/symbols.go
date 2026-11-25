// Package symbols answers "who uses this, how, and what does its shape say"
// with full type information, so the answer is complete where grep is not.
package symbols

import (
	"context"
	"errors"
	"fmt"
	"go/types"
	"strings"
)

// Symbol names a package level declaration, a method or a field.
type Symbol struct {
	Package string // import path
	Name    string // "Name", "Type.Method" or "Type.Field"
}

// String renders the symbol as "import/path.Name" or "import/path.Type.Member".
func (s Symbol) String() string { return s.Package + "." + s.Name }

// ParseSymbol reads "import/path.Name" or "import/path.Type.Member".
// The import path ends at the last slash plus the first dot separated segment after it.
func ParseSymbol(s string) (Symbol, error) {
	if s == "" {
		return Symbol{}, fmt.Errorf("symbols: empty symbol")
	}
	lastSlash := strings.LastIndex(s, "/")
	directory, lastElement := "", s
	if lastSlash >= 0 {
		directory, lastElement = s[:lastSlash+1], s[lastSlash+1:]
		// A leading slash or an empty path element cannot be an import path.
		if lastSlash == 0 || strings.Contains(directory, "//") {
			return Symbol{}, fmt.Errorf("symbols: %q has an empty import path element", s)
		}
	}
	segments := strings.Split(lastElement, ".")
	if len(segments) < 2 {
		return Symbol{}, fmt.Errorf("symbols: %q has no dot after the last slash", s)
	}
	if len(segments) > 3 {
		return Symbol{}, fmt.Errorf("symbols: %q has more than two dots after the last slash", s)
	}
	for _, segment := range segments {
		if segment == "" {
			return Symbol{}, fmt.Errorf("symbols: %q has an empty segment or a trailing dot", s)
		}
	}
	return Symbol{Package: directory + segments[0], Name: strings.Join(segments[1:], ".")}, nil
}

// Options controls loading. The zero value loads ./... with tests.
type Options struct {
	Patterns     []string                         // package patterns relative to Root; empty means ./...
	ExcludeTests bool                             // leave _test.go files out
	Strict       bool                             // any broken package is an error, the behaviour before stage 9
	Logf         func(format string, args ...any) // one line per tolerated broken package; nil means silent

	ExcludeGenerated bool // leave references that sit in generated files out of the result
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

	Generated bool   // the file carries the "Code generated ... DO NOT EDIT." header
	Symbol    Symbol // the declaration this is a use of; never a method set symbol
}

// Sentinel errors wrapped by the lookups.
var (
	ErrNotFound     = errors.New("symbol not found")
	ErrNotInterface = errors.New("symbol is not an interface")
	ErrNotStruct    = errors.New("symbol is not a struct")
)

// References lists every use of sym except its declaration, sorted and unique.
func References(ctx context.Context, root string, sym Symbol, opts Options) ([]Reference, error) {
	loaded, err := load(ctx, root, opts)
	if err != nil {
		return nil, err
	}
	if err := loaded.tolerate(sym, scanDependents, opts); err != nil {
		return nil, err
	}
	target, err := loaded.resolve(sym)
	if err != nil {
		return nil, err
	}
	return findReferences(loaded, target)
}

// ReferencesTo lists every use of any symbol in syms except declarations, sorted and unique.
// A method set symbol Type.* stands for every method of Type (stage 10).
func ReferencesTo(ctx context.Context, root string, syms []Symbol, opts Options) ([]Reference, error) {
	return nil, errors.New("symbols.ReferencesTo: not implemented")
}

// Implementations lists the named non-interface types in the loaded packages that satisfy iface.
func Implementations(ctx context.Context, root string, iface Symbol, opts Options) ([]Symbol, error) {
	loaded, err := load(ctx, root, opts)
	if err != nil {
		return nil, err
	}
	if err := loaded.tolerate(iface, scanAll, opts); err != nil {
		return nil, err
	}
	object, err := loaded.resolve(iface)
	if err != nil {
		return nil, err
	}
	if !isInterfaceType(object) {
		return nil, fmt.Errorf("symbols: %s: %w", iface, ErrNotInterface)
	}
	return findImplementations(loaded, iface, object), nil
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
	loaded, err := load(ctx, root, opts)
	if err != nil {
		return nil, err
	}
	if err := loaded.tolerate(typ, scanNone, opts); err != nil {
		return nil, err
	}
	object, err := loaded.resolve(typ)
	if err != nil {
		return nil, err
	}
	typeName, isTypeName := object.(*types.TypeName)
	if !isTypeName {
		return nil, fmt.Errorf("symbols: %s: %w", typ, ErrNotStruct)
	}
	structType, isStruct := types.Unalias(typeName.Type()).Underlying().(*types.Struct)
	if !isStruct {
		return nil, fmt.Errorf("symbols: %s: %w", typ, ErrNotStruct)
	}
	return structFields(loaded, typeName, structType)
}
