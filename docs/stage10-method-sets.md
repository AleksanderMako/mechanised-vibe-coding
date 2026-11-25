# Stage 10: method set references and generated files

A follow-up stage to `symbols` and its `refs` action. Two gaps found in use:

- Asking "who calls any of these" needed one `refs` call per method and a
  merge by hand. A refactoring question such as "every call of `Exec`, `Query`
  and `QueryRow` on the database handle" spans several methods on several
  types, so `refs` now takes several symbols at once, and `Type.*` stands for
  the method set of a type.
- Generated files (sqlc, gqlgen, wire, protobuf) carry the most references in
  a large module and are never the ones an agent edits. A reference now says
  whether its file is generated, and generated files can be left out.

No existing exported signature changes. `symbols.Options` gains one field,
`symbols.Reference` gains two, package `symbols` gains one function and the
`refs` action gains a flag, extra positional arguments and one text column.

---

## Definitions

- A **generated file** is a file for which `go/ast.IsGenerated` reports true:
  a comment line `// Code generated ... DO NOT EDIT.` (or the `/* */` form)
  before the first non-comment, non-blank text, as described at
  https://go.dev/s/generatedcode. The file name plays no part.
- A **method set symbol** is a `Symbol` whose `Name` is `Type.*`: a type name,
  a dot and a single asterisk. `ParseSymbol` already accepts it ("*" is a non
  empty segment) and `String` renders it back; nothing in `ParseSymbol` changes.
- The **methods of a type** `T`, for `Type.*`:
  - when `T` resolves to a named type (or an alias of one) whose underlying
    type is an interface: every method of the interface, embedded interfaces
    included (`types.Interface.Method` after completion);
  - otherwise, when `T` resolves to a named type: the methods declared with
    receiver `T` or `*T` (`types.Named.Method`), promoted methods of embedded
    fields excluded;
  - type parameters and instantiations never qualify; the origin named type is
    used.

## Exported API additions

```go
package symbols

type Options struct {
	// ... existing fields unchanged ...
	ExcludeGenerated bool // leave references that sit in generated files out of the result
}

type Reference struct {
	// ... existing fields unchanged ...
	Generated bool   // the file is a generated file
	Symbol    Symbol // the declaration this is a use of; never a method set symbol
}

// ReferencesTo lists every use of any symbol in syms, excluding declarations,
// sorted by Package, File, Line, Column, with duplicates removed. A method set
// symbol Type.* stands for every method of Type.
func ReferencesTo(ctx context.Context, root string, syms []Symbol, opts Options) ([]Reference, error)
```

## Behaviour

Rules 1 to 19 of stage 3 and the stage 9 rules stay as they are. The rules
below extend them.

`Reference.Generated` and `Options.ExcludeGenerated`, in `References` and
`ReferencesTo` alike:

20. `Generated` is true exactly when the reference's file is a generated file.
    The header is read from the parsed syntax, so a file with the header in
    the wrong place (after the package clause, or after code) is not generated.
21. With `ExcludeGenerated`, references in generated files are absent from the
    result. Loading and type checking are unchanged, so references in hand
    written files that use declarations from generated files are still found,
    and a generated file that does not compile is tolerated or reported exactly
    as before.
22. `ExcludeTests` and `ExcludeGenerated` combine: a generated `_test.go` file
    is dropped by either.

`Reference.Symbol`:

23. `Symbol` is the symbol the identifier resolves to, as `Symbol{Package,
    Name}` with `Name` being `Name`, `Type.Method` or `Type.Field`, the form
    `References` accepts as input. For a method it is the type that declares
    the method, so a method reached through an embedded interface reports the
    embedded interface's type (`io.Reader.Read` for a call through an
    interface that embeds `io.Reader`), and a method declared on `*T` reports
    `T.Method`.
24. `References(ctx, root, sym, opts)` fills `Symbol` too. For a plain symbol
    `Symbol` equals `sym`; for a method set symbol it is the method the
    reference uses.

Method set symbols, in `References` and `ReferencesTo` alike:

25. `Type.*` resolves `Type` in the symbol's package like any type name (stage
    3 rules 4 to 7). A `Type` that does not exist wraps `ErrNotFound`. A `Type`
    that is not a type name (a function, variable or constant) wraps
    `ErrNotFound`.
26. The references of `Type.*` are the union of the references of each method
    of the type, as if each `Type.Method` had been asked for.
27. A type with no methods gives an empty list and no error.
28. `Name` equal to `*` alone (no type), or `*.Member`, wraps `ErrNotFound`.

`ReferencesTo`:

29. An empty `syms` is an ordinary error (not `ErrNotFound`); nothing is
    loaded.
30. Every symbol in `syms` is resolved before any reference is collected. When
    one does not resolve the call returns that error and no partial result;
    with several unresolved symbols the first in `syms` order is reported.
31. The result is the union of `References` for each symbol, each reference
    carrying its own `Symbol`. A position is listed once even when two entries
    of `syms` cover it (the same method named directly and through `Type.*`,
    or the same symbol given twice).
32. Sorting is by `Package`, `File`, `Line`, `Column`, as for `References`;
    the order of `syms` does not affect the output.
33. Broken packages: the stage 9 rules apply per symbol as if `References` had
    been called for each; a broken package that could hold a reference to any
    of the symbols is tolerated and logged, and each broken package is logged
    at most once per call. `Strict` keeps its stage 9 meaning.
34. The packages are loaded once for the whole call.

---

## cmd/mvc

The `refs` action of the `symbols` group changes; `impls` and `tags` do not.

```
mvc symbols refs -root DIR [-pkg P]... [-no-tests] [-no-generated] [-strict] [-json] SYMBOL...
```

35. One or more `SYMBOL` arguments; none is a usage error (exit 2, usage on
    stderr). Each is parsed with `ParseSymbol`; a parse failure is a usage
    error naming the argument. The action calls `ReferencesTo` with all of
    them, also when only one is given.
36. `-no-generated` sets `ExcludeGenerated`. `-no-tests`, `-strict`, `-pkg`,
    `-root` and `-json` keep their meaning.
37. Text output is one line per reference,
    `FILE:LINE:COL<TAB>KIND<TAB>SYMBOL<TAB>CONTEXT`, where `SYMBOL` is
    `Reference.Symbol.String()`. This replaces the stage 8 format, which had no
    `SYMBOL` column.
38. `-json` prints the `[]symbols.Reference`, with `Generated` and `Symbol`
    present on every element.
39. An unresolved symbol is an ordinary error (exit 2, one `mvc: ` line on
    stderr). An empty result is success with no output (exit 0).
40. The action help names the `Type.*` form and `-no-generated`.

## Out of scope

Dynamic dispatch (a call through an interface method is not a reference of
the concrete implementations, and the reverse), package wide wildcards
(`pkg.*`), promoted methods of embedded struct fields under `Type.*`,
generated file detection for `impls` and `tags`, and file name conventions
such as `*.pb.go` or `*_gen.go`.
