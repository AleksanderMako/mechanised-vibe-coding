package refactor

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/AleksanderMako/machanised-vibe-coding/symbols"
)

// loadMode asks for syntax and full type information of the matched packages;
// dependencies come from export data, which is enough to resolve their objects.
const loadMode = packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
	packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedModule

// workspace holds the type checked packages one operation works on.
type workspace struct {
	root       string // absolute module root
	modulePath string // module path from go.mod
	fileSet    *token.FileSet
	packages   []*packages.Package // matched packages and their test variants, without generated test mains
}

// load type checks the packages matched by patterns under root and fails on any package error.
func load(ctx context.Context, root string, patterns []string) (*workspace, error) {
	absoluteRoot, err := checkRoot(root)
	if err != nil {
		return nil, err
	}
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}
	fileSet := token.NewFileSet()
	config := &packages.Config{
		Context: ctx,
		Mode:    loadMode,
		Dir:     absoluteRoot,
		Tests:   true,
		Fset:    fileSet,
	}
	loaded, err := packages.Load(config, patterns...)
	if err != nil {
		return nil, fmt.Errorf("refactor: load packages: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// A broken package means type information is incomplete, so nothing may be rewritten.
	for _, pkg := range loaded {
		if len(pkg.Errors) > 0 {
			return nil, fmt.Errorf("refactor: package %s: %v", pkg.ID, pkg.Errors[0])
		}
	}
	result := &workspace{root: absoluteRoot, fileSet: fileSet}
	for _, pkg := range loaded {
		// The generated test main lives in the build cache and only calls the test functions.
		if strings.HasSuffix(pkg.ID, ".test") {
			continue
		}
		if pkg.Types == nil || pkg.TypesInfo == nil {
			return nil, fmt.Errorf("refactor: package %s has no type information", pkg.ID)
		}
		if pkg.Module != nil && pkg.Module.Main && result.modulePath == "" {
			result.modulePath = pkg.Module.Path
		}
		result.packages = append(result.packages, pkg)
	}
	if result.modulePath == "" {
		result.modulePath, err = readModulePath(absoluteRoot)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}

// checkRoot returns root as an absolute path after making sure it holds a go.mod.
func checkRoot(root string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("refactor: root is empty")
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("refactor: resolve root: %w", err)
	}
	info, err := os.Stat(filepath.Join(absoluteRoot, "go.mod"))
	if err != nil || info.IsDir() {
		return "", fmt.Errorf("refactor: %s does not contain go.mod", root)
	}
	return absoluteRoot, nil
}

// readModulePath reads the module line of go.mod, used when no package was loaded.
func readModulePath(root string) (string, error) {
	content, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", fmt.Errorf("refactor: read go.mod: %w", err)
	}
	for _, line := range strings.Split(string(content), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "module" {
			return strings.Trim(fields[1], `"`), nil
		}
	}
	return "", fmt.Errorf("refactor: go.mod has no module line")
}

// findPackage returns the types of importPath, preferring a matched non-test package,
// then a matched test variant, then any dependency reachable from the matched packages.
func (w *workspace) findPackage(importPath string) *types.Package {
	var testVariant *types.Package
	for _, pkg := range w.packages {
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
	for _, pkg := range w.packages {
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

// resolve finds the object sym names, wrapping symbols.ErrNotFound when it does not exist.
func (w *workspace) resolve(sym symbols.Symbol) (types.Object, error) {
	pkg := w.findPackage(sym.Package)
	if pkg == nil {
		return nil, fmt.Errorf("refactor: package %s is not loaded or imported: %w", sym.Package, symbols.ErrNotFound)
	}
	object := lookupIn(pkg, sym.Name)
	if object == nil {
		return nil, fmt.Errorf("refactor: %s: %w", sym, symbols.ErrNotFound)
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
func (w *workspace) keyOf(object types.Object) objectKey {
	object = originOf(object)
	key := objectKey{name: object.Name()}
	if object.Pkg() != nil {
		key.packagePath = object.Pkg().Path()
	}
	position := w.fileSet.Position(object.Pos())
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

// declaration is where an object is declared in the loaded syntax.
type declaration struct {
	pkg   *packages.Package
	file  *ast.File
	ident *ast.Ident
	path  []ast.Node // enclosing nodes from the identifier outwards to the file
}

// findDeclaration locates the declaring identifier of object in the loaded packages.
func (w *workspace) findDeclaration(object types.Object) (*declaration, bool) {
	key := w.keyOf(object)
	for _, pkg := range w.packages {
		for ident, defined := range pkg.TypesInfo.Defs {
			if defined == nil || ident.Name != key.name || w.keyOf(defined) != key {
				continue
			}
			file := w.fileOf(pkg, ident.Pos())
			if file == nil {
				continue
			}
			path, _ := pathTo(file, ident)
			return &declaration{pkg: pkg, file: file, ident: ident, path: path}, true
		}
	}
	return nil, false
}

// fileOf returns the syntax file of pkg that contains pos.
func (w *workspace) fileOf(pkg *packages.Package, pos token.Pos) *ast.File {
	for _, file := range pkg.Syntax {
		if file.FileStart <= pos && pos <= file.FileEnd {
			return file
		}
	}
	return nil
}

// isPackageLevel reports whether object is declared at package scope.
func isPackageLevel(object types.Object) bool {
	return object.Pkg() != nil && object.Parent() == object.Pkg().Scope()
}

// packagesWithPath returns every loaded variant of the package at importPath.
func (w *workspace) packagesWithPath(importPath string) []*packages.Package {
	var result []*packages.Package
	for _, pkg := range w.packages {
		if pkg.PkgPath == importPath {
			result = append(result, pkg)
		}
	}
	return result
}
