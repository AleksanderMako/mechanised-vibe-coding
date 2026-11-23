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
)

// loadMode asks for syntax and full type information of the matched packages only;
// dependencies come from export data, which is enough to resolve their objects.
const loadMode = packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
	packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedModule

// loadResult holds the packages one call works on.
type loadResult struct {
	root     string // absolute module root
	fileSet  *token.FileSet
	packages []*packages.Package // the matched packages and their test variants, without generated test mains
}

// load type checks the packages matched by opts under root and fails on any package error.
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
	// Report the first broken package so no partial answer is ever returned.
	for _, pkg := range loaded {
		if len(pkg.Errors) > 0 {
			return nil, fmt.Errorf("symbols: package %s: %v", pkg.ID, pkg.Errors[0])
		}
	}
	result := &loadResult{root: absoluteRoot, fileSet: fileSet}
	for _, pkg := range loaded {
		// The generated test main lives in the build cache and only calls the test functions.
		if strings.HasSuffix(pkg.ID, ".test") {
			continue
		}
		if pkg.Types == nil || pkg.TypesInfo == nil {
			return nil, fmt.Errorf("symbols: package %s has no type information", pkg.ID)
		}
		result.packages = append(result.packages, pkg)
	}
	return result, nil
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
