# Stage 3: symbols

One package for type aware impact analysis over a Go module: who references a
symbol and how, which named types implement an interface, and what the struct
tags of a type say. It replaces grep, which lies when output is truncated or a
name is reused in another package.

Built on `golang.org/x/tools/go/packages` with full type information,
including test files. Works on a module checkout given by `Root`. Never uses
the current working directory.

---

## Package symbols

### Exported API

```go
package symbols

// Symbol names a package level declaration, a method or a field.
type Symbol struct {
	Package string // import path
	Name    string // "Name", "Type.Method" or "Type.Field"
}

// String renders the symbol as "import/path.Name" or "import/path.Type.Member".
func (s Symbol) String() string

// ParseSymbol reads "import/path.Name" or "import/path.Type.Member".
// The import path ends at the last slash plus the first dot separated segment
// after it: "github.com/a/b/pkg.Type.Method" is package "github.com/a/b/pkg",
// name "Type.Method"; "io.Reader" is package "io", name "Reader".
func ParseSymbol(s string) (Symbol, error)

// Options controls loading. The zero value loads ./... with tests.
type Options struct {
	Patterns     []string // package patterns relative to Root; empty means ./...
	ExcludeTests bool     // leave _test.go files out
}

// Kind says how a reference uses the symbol.
type Kind string

const (
	KindCall       Kind = "call"       // the symbol is called: f(), x.M()
	KindConversion Kind = "conversion" // a type used as a conversion: T(x)
	KindType       Kind = "type"       // a type used in a type position: var x T, []T, T{...}, func(T)
	KindEmbed      Kind = "embed"      // a type embedded in a struct or interface
	KindValue      Kind = "value"      // any other use: passed, assigned, selected, compared
)

// Reference is one use of a symbol.
type Reference struct {
	Package string // import path of the package the use is in
	File    string // absolute path
	Line    int
	Column  int
	Kind    Kind
	Test    bool // the use is in a _test.go file
	Context string // the source line, trimmed
}

// References lists every use of sym in the loaded packages, excluding its own
// declaration, sorted by Package, File, Line, Column, with duplicates removed.
func References(ctx context.Context, root string, sym Symbol, opts Options) ([]Reference, error)

// Implementations lists the named non-interface types declared in the loaded
// packages whose method set, as T or *T, satisfies the interface iface. Sorted
// by Symbol.String().
func Implementations(ctx context.Context, root string, iface Symbol, opts Options) ([]Symbol, error)

// FieldTag describes one field of a struct type.
type FieldTag struct {
	Field     string // field name; the type name for embedded fields
	Type      string // the type as written in source
	Embedded  bool
	Exported  bool
	Tag       string // the raw tag without back quotes, "" when none
	JSON      string // json name: the tag's name part, "-" when excluded, "" when no json tag or no name part
	OmitEmpty bool   // json tag has omitempty
}

// StructTags lists the fields of struct type typ in declaration order.
func StructTags(ctx context.Context, root string, typ Symbol, opts Options) ([]FieldTag, error)

// ErrNotFound is wrapped when a symbol does not resolve.
var ErrNotFound = errors.New("symbol not found")

// ErrNotInterface is wrapped when Implementations is given a non-interface type.
var ErrNotInterface = errors.New("symbol is not an interface")

// ErrNotStruct is wrapped when StructTags is given a non-struct type.
var ErrNotStruct = errors.New("symbol is not a struct")
```

### Behaviour

`ParseSymbol`:

1. An empty string, a string without a dot after the last slash, a trailing dot, an empty segment (`a/.B`, `io..X`), a leading slash, `//` in the path, or more than two dots after the last slash is an error.
2. Everything before the last slash plus the segment up to the first dot after it is the package; the rest is the name.
3. `String` is the inverse for every valid symbol.

Loading:

4. `Root` empty or without `go.mod` is an ordinary error. Patterns are passed to `packages.Load` relative to `Root`, with `Tests: !ExcludeTests` and syntax plus type info loaded.
5. Packages with load errors or type errors make the call return an error that names the first package and error. Partial results are never returned.
6. The symbol's package must be one of the loaded packages or one of their dependencies, so `io.Reader` resolves when the module imports `io`. A symbol whose package is loaded but whose name is not declared there wraps `ErrNotFound`. A package not reachable at all also wraps `ErrNotFound`.
7. A method `Type.Method` is looked up on the named type with `types.LookupFieldOrMethod` with pointer receiver allowed. A field `Type.Field` is looked up the same way.

`References`:

8. Every identifier in the loaded packages whose `types.Object` is the symbol's object is a reference, except the declaring identifier. For generics the origin object is compared so instantiations count.
9. Test variants of a package produced by `go/packages` (the package with test files and the external test package) can contain the same non-test file twice; duplicates by `File`, `Line`, `Column` are removed.
10. `Kind` classification, by the innermost enclosing node of the identifier (looking through a selector `x.Name` to its parent):
   - parent is a `CallExpr` with the identifier (or selector, or for a generic symbol the explicit instantiation `F[int]`) as `Fun`: `KindConversion` when the object is a type, else `KindCall`;
   - the identifier is the type of an embedded field (a `Field` with no names) in a struct or interface, also behind `*`, a package qualifier or type arguments: `KindEmbed`; union elements of an interface type set are `KindType`;
   - the object is a type used anywhere else: `KindType`;
   - otherwise `KindValue`. A method value `x.M` not called is `KindValue`.
11. `Test` is true when the file name ends in `_test.go`.
12. `Context` is the source line of the reference with leading and trailing whitespace removed.
13. `ExcludeTests` drops test files from loading, so references in them are absent.

`Implementations`:

14. `iface` must resolve to a named type (or an alias of one) whose underlying type is an interface, else wraps `ErrNotInterface`.
15. Candidates are the named types declared at package level in the loaded packages, excluding interfaces and type aliases. A candidate is listed when `types.Implements(T, iface)` or `types.Implements(*T, iface)` holds. An empty interface lists every candidate.
16. Each listed type appears once, as `Symbol{Package, TypeName}`, sorted by `String()`.

`StructTags`:

17. `typ` must resolve to a named type (or an alias of one) whose underlying type is a struct, else wraps `ErrNotStruct`. Struct types from dependencies, including the standard library, work too.
18. Fields are listed in declaration order; a multi-name field `A, B T` gives one entry per name. `Type` is the type expression as written in the source file, printed with `go/printer`; for `type B A` with no struct literal of its own it is the `go/types` string. Embedded fields have `Field` set to the type's name without package qualifier or pointer.
19. `Tag` is the raw tag text without the surrounding back quotes. `JSON` is the first comma separated part of the `json` tag; `OmitEmpty` is true when a later part is `omitempty`. A `json:"-"` tag gives `JSON` `-`. A `json:",omitempty"` tag gives empty `JSON` and `OmitEmpty` true.

### Out of scope

Editing, cross module references beyond the loaded patterns, call graphs, gopls.
