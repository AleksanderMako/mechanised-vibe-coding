# Stage 9: broken package tolerance

A follow-up stage that changes how `symbols`, `refactor` and `deadcode` react to a
package that does not compile. Before this stage any load or type error anywhere
under the patterns made the whole call fail, so one stale generated directory
blocked every lookup in a module. After this stage a broken package is tolerated
whenever the result cannot depend on it, and reported through a log callback when
it can.

No existing exported signature changes. Each of the three `Options` structs gains
the same two fields, `refactor` gains one sentinel, and `cmd/mvc` gains a flag.

---

## Definitions

- A **broken package** is a loaded package, test variants included, whose
  `packages.Package.Errors` is not empty, whatever the kind of error (list,
  parse or type).
- A package **depends on** another when the other is in its transitive
  `Imports`. A test variant `p [p.test]` depends on everything `p` depends on
  and on the external test package `p_test` the same way.
- The **declaring package** of a symbol is the package `Symbol.Package` names.
- A **log line** is one call of `Logf` with a format and arguments, producing
  one line without a trailing newline. `Logf == nil` means silent.

## Exported API additions

```go
// in package symbols, refactor and deadcode alike:
type Options struct {
	// ... existing fields unchanged ...
	Strict bool                              // any broken package is an error, the behaviour before stage 9
	Logf   func(format string, args ...any)  // one line per tolerated broken package; nil means silent
}
```

```go
// package refactor
var ErrBrokenPackage = errors.New("a package that may use the symbol has errors")
```

## Behaviour

### symbols (replaces stage 3 rule 5)

1. `Strict`: a broken package makes the call return an error naming the first
   package and its first error, and no partial result, as before.
2. Otherwise, when the declaring package or any of its test variants is broken
   the call is an ordinary error naming it; its symbol table is not trusted.
3. For `References` and `StructTags`, a broken package that is not the declaring
   package and does not depend on it cannot mention the symbol. It is not
   scanned and gets the log line `symbols: ignoring package %s: %v` with the
   package ID and its first error.
4. For `References`, a broken package that depends on the declaring package is
   scanned with the type information the checker produced; identifiers it could
   not resolve are not found. It gets the log line
   `symbols: package %s has errors, results there are best effort: %v`.
5. For `Implementations`, implementing an interface is structural, so every
   broken package may hold an implementation. Each is scanned as in rule 4 and
   gets the same best-effort log line.
6. Log lines are emitted once per package ID, in `packages.Visit` order, before
   the result is returned. The result slices and their sorting are unchanged.

### refactor (amends stage 4 rule 1)

7. `Strict`: a load or type error anywhere is an ordinary error and nothing is
   written, as before.
8. Otherwise, a broken package that is the source package, depends on it, or is
   the destination package of `Move`, makes the call fail with an error wrapping
   `ErrBrokenPackage` that names the package; nothing is written. These are the
   packages a rewrite could miss or corrupt.
9. Every other broken package is ignored with the log line
   `refactor: ignoring package %s: %v`. Interfaces declared in an ignored
   package are not considered for `ErrBreaksInterface`.
10. The final build is unchanged: only the directories of written files are
    built, so an ignored package is never built.

### deadcode (amends stage 7 rule 4)

11. `Strict`: a load or type error is an ordinary error naming the first one, as
    before.
12. Otherwise, broken packages and every loaded package that depends on one are
    excluded from the analysis. A broken package gets the log line
    `deadcode: ignoring package %s: %v`; a dependent gets
    `deadcode: ignoring package %s: it imports broken package %s`. Excluded
    main packages are not roots; when no root remains the call wraps
    `ErrNoRoots`. Functions of excluded packages are never listed. A function
    that only excluded packages reach is listed as dead; the log lines are the
    warning that the result is incomplete.
13. `Orphaned` forwards `Options` unchanged to both `Dead` calls, so log lines
    can appear once per revision. Stage 7 rule 12 (equality with the reference
    tool) holds for modules without broken packages.

### cmd/mvc (amends stage 8)

14. `symbols refs|impls|tags`, every `refactor` action and `dead list|orphaned`
    accept `-strict`, which sets `Options.Strict`. Without it `Logf` writes each
    line to stderr as `mvc: warning: <line>`. Warnings never change the exit
    code and never appear on stdout, also with `-json`.
15. A `refactor` error wrapping `ErrBrokenPackage` is an ordinary error: exit `2`,
    one stderr line.

## Out of scope

Repairing or skipping broken packages in `verify`, `editset` or `astedit`, and
any heuristic that guesses whether a broken file mentions a symbol textually.
