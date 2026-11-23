package symbols

import (
	"context"
	"fmt"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/AleksanderMako/machanised-vibe-coding/internal/brokenpkg"
)

// loadMode asks for syntax and full type information of the matched packages only;
// dependencies come from export data, which is enough to resolve their objects.
// NeedImports keeps the import graph go/packages builds anyway, to tell which broken packages depend on a symbol.
const loadMode = packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports |
	packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedModule

// loadResult holds the packages one call works on.
type loadResult struct {
	root     string // absolute module root
	fileSet  *token.FileSet
	packages []*packages.Package // the matched packages and their test variants, without generated test mains
	matched  []*packages.Package // everything packages.Load returned, the roots of the import graph
	broken   []*packages.Package // the broken packages among packages, in packages.Visit order
}

// load type checks the packages matched by opts under root; with opts.Strict it fails on any package error.
func load(ctx context.Context, root string, opts Options) (*loadResult, error) {
	absoluteRoot, err := checkRoot(root)
	if err != nil {
		return nil, err
	}
	patterns := opts.Patterns
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	fileSet := token.NewFileSet()
	config := &packages.Config{
		Context: ctx,
		Mode:    loadMode,
		Dir:     absoluteRoot,
		Tests:   !opts.ExcludeTests,
		Fset:    fileSet,
	}
	loaded, err := packages.Load(config, patterns...)
	if err != nil {
		return nil, fmt.Errorf("symbols: load packages: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opts.Strict {
		// Report the first broken package so no partial answer is ever returned.
		for _, pkg := range loaded {
			if brokenpkg.IsBroken(pkg) {
				return nil, fmt.Errorf("symbols: package %s: %v", pkg.ID, pkg.Errors[0])
			}
		}
	}
	result := &loadResult{root: absoluteRoot, fileSet: fileSet, matched: loaded}
	isMatched := map[*packages.Package]bool{}
	for _, pkg := range loaded {
		// The generated test main lives in the build cache and only calls the test functions.
		if strings.HasSuffix(pkg.ID, ".test") {
			continue
		}
		isMatched[pkg] = true
		if brokenpkg.IsBroken(pkg) {
			continue
		}
		if pkg.Types == nil || pkg.TypesInfo == nil {
			return nil, fmt.Errorf("symbols: package %s has no type information", pkg.ID)
		}
		result.packages = append(result.packages, pkg)
	}
	// Broken packages join the scanned ones only after tolerate decided they may hold results.
	for _, pkg := range brokenpkg.InVisitOrder(loaded) {
		if isMatched[pkg] && brokenpkg.IsBroken(pkg) {
			result.broken = append(result.broken, pkg)
		}
	}
	return result, nil
}

// scanPolicy says which broken packages one lookup scans for its result.
type scanPolicy int

const (
	scanNone       scanPolicy = iota // StructTags reads only the declaring package
	scanDependents                   // References: only packages that import the declaring package can use the symbol
	scanAll                          // Implementations: satisfying an interface needs no import
)

// tolerate applies stage 9 to the broken packages: the declaring package must be sound,
// the others are scanned as best effort or ignored per policy, each with one log line.
func (l *loadResult) tolerate(sym Symbol, policy scanPolicy, opts Options) error {
	// In strict mode load already failed on every broken package.
	if opts.Strict {
		return nil
	}
	for _, pkg := range brokenpkg.InVisitOrder(l.matched) {
		if pkg.PkgPath == sym.Package && brokenpkg.IsBroken(pkg) {
			return fmt.Errorf("symbols: package %s: %v", pkg.ID, pkg.Errors[0])
		}
	}
	logf := opts.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	byID := brokenpkg.IndexByID(l.matched)
	for _, pkg := range l.broken {
		scan := policy == scanAll || (policy == scanDependents && brokenpkg.DependsOn(pkg, sym.Package, byID))
		// A package that failed before type checking has nothing to scan.
		if scan && pkg.Types != nil && pkg.TypesInfo != nil {
			logf("symbols: package %s has errors, results there are best effort: %v", pkg.ID, pkg.Errors[0])
			l.packages = append(l.packages, pkg)
			continue
		}
		logf("symbols: ignoring package %s: %v", pkg.ID, pkg.Errors[0])
	}
	return nil
}

// checkRoot returns root as an absolute path after making sure it holds a go.mod.
func checkRoot(root string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("symbols: root is empty")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("symbols: resolve root: %w", err)
	}
	info, err := os.Stat(filepath.Join(absoluteRoot, "go.mod"))
	if err != nil || info.IsDir() {
		return "", fmt.Errorf("symbols: %s does not contain go.mod", root)
	}
	return absoluteRoot, nil
}

// findPackage returns the types of import path, preferring a matched non-test package,
// then a matched test variant, then any dependency reachable from the matched packages.
func (l *loadResult) findPackage(importPath string) *types.Package {
	var testVariant *types.Package
	for _, pkg := range l.packages {
		if pkg.PkgPath != importPath {
			continue
		}
		if pkg.ID == pkg.PkgPath {
			return pkg.Types
		}
		if testVariant == nil {
			testVariant = pkg.Types
		}
	}
	if testVariant != nil {
		return testVariant
	}
	seen := map[*types.Package]bool{}
	for _, pkg := range l.packages {
		if found := findImport(pkg.Types, importPath, seen); found != nil {
			return found
		}
	}
	return nil
}

// findImport searches the import graph of start for importPath, visiting each package once.
func findImport(start *types.Package, importPath string, seen map[*types.Package]bool) *types.Package {
	if start == nil || seen[start] {
		return nil
	}
	seen[start] = true
	if start.Path() == importPath {
		return start
	}
	for _, imported := range start.Imports() {
		if found := findImport(imported, importPath, seen); found != nil {
			return found
		}
	}
	return nil
}

// resolve finds the object sym names, wrapping ErrNotFound when it does not exist.
func (l *loadResult) resolve(sym Symbol) (types.Object, error) {
	pkg := l.findPackage(sym.Package)
	if pkg == nil {
		return nil, fmt.Errorf("symbols: package %s is not loaded or imported: %w", sym.Package, ErrNotFound)
	}
	object := lookupIn(pkg, sym.Name)
	if object == nil {
		return nil, fmt.Errorf("symbols: %s: %w", sym, ErrNotFound)
	}
	return object, nil
}

// lookupIn finds "Name" or "Type.Member" in pkg, or returns nil.
func lookupIn(pkg *types.Package, name string) types.Object {
	typeName, member, hasMember := strings.Cut(name, ".")
	object := pkg.Scope().Lookup(typeName)
	if !hasMember || object == nil {
		return object
	}
	if _, isTypeName := object.(*types.TypeName); !isTypeName {
		return nil
	}
	// Pointer receivers are allowed so methods declared on *T are found from T.
	memberObject, _, _ := types.LookupFieldOrMethod(object.Type(), true, pkg, member)
	return memberObject
}

// objectKey identifies an object by where it is declared, so the copies that test
// variants of one package hold of the same declaration compare equal.
type objectKey struct {
	packagePath string
	name        string
	file        string
	line        int
	column      int
}

// keyOf returns the key of object, comparing generic instantiations by their origin.
func (l *loadResult) keyOf(object types.Object) objectKey {
	object = originOf(object)
	key := objectKey{name: object.Name()}
	if object.Pkg() != nil {
		key.packagePath = object.Pkg().Path()
	}
	position := l.fileSet.Position(object.Pos())
	key.file, key.line, key.column = position.Filename, position.Line, position.Column
	return key
}

// originOf maps a method or field of an instantiated generic type back to its declaration.
func originOf(object types.Object) types.Object {
	switch typed := object.(type) {
	case *types.Func:
		return typed.Origin()
	case *types.Var:
		return typed.Origin()
	}
	return object
}
