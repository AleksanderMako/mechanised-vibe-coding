package refactor

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"

	"github.com/AleksanderMako/machanised-vibe-coding/symbols"
)

// callSite is one call of the method that will become a function call.
type callSite struct {
	pkg      *packages.Package
	file     *ast.File
	selector *ast.SelectorExpr
}

// demethodize implements Demethodize.
func demethodize(ctx context.Context, root string, method symbols.Symbol, newName string, opts Options) ([]string, error) {
	if _, err := checkRoot(root); err != nil {
		return nil, err
	}
	if newName != "" && !isValidName(newName) {
		return nil, fmt.Errorf("refactor: %q: %w", newName, ErrBadName)
	}
	work, err := load(ctx, root, opts.Patterns)
	if err != nil {
		return nil, err
	}
	object, err := work.resolve(method)
	if err != nil {
		return nil, err
	}
	function, isFunc := object.(*types.Func)
	if !isFunc || function.Type().(*types.Signature).Recv() == nil {
		return nil, fmt.Errorf("refactor: %s is not a method", method)
	}
	declared, found := work.findDeclaration(function)
	if !found {
		return nil, fmt.Errorf("refactor: %s is not declared in the loaded packages", method)
	}
	funcDecl, isFuncDecl := declared.path[1].(*ast.FuncDecl)
	if !isFuncDecl || funcDecl.Recv == nil {
		return nil, fmt.Errorf("refactor: %s is an interface method, not a declared method", method)
	}
	receiverType := function.Type().(*types.Signature).Recv().Type()
	if pointer, isPointer := receiverType.(*types.Pointer); isPointer {
		receiverType = pointer.Elem()
	}
	named, isNamed := types.Unalias(receiverType).(*types.Named)
	if !isNamed {
		return nil, fmt.Errorf("refactor: %s has no named receiver type", method)
	}
	// The receiver's type parameters are in scope of the body; a plain function would lose them.
	if named.TypeParams().Len() > 0 {
		return nil, fmt.Errorf("refactor: %s has a generic receiver: %w", method, ErrUnsupportedUse)
	}
	if err := checkReceiverUnused(declared.pkg, funcDecl); err != nil {
		return nil, fmt.Errorf("refactor: %s: %w", method, err)
	}

	sites, err := work.callSites(function)
	if err != nil {
		return nil, fmt.Errorf("refactor: %s: %w", method, err)
	}
	methodPackage := function.Pkg()
	finalName := newName
	if finalName == "" {
		finalName = function.Name()
	}
	// A function called from another package must be exported to stay reachable.
	for _, site := range sites {
		if site.pkg.PkgPath != methodPackage.Path() && !token.IsExported(finalName) {
			finalName = exportedForm(finalName)
			break
		}
	}

	if err := checkFunctionName(work, methodPackage.Path(), finalName, sites); err != nil {
		return nil, err
	}
	if iface, breaks := work.brokenInterface(named.Obj(), function.Name()); breaks {
		return nil, fmt.Errorf("refactor: %s satisfies %s through %s: %w", named.Obj().Name(), iface, function.Name(), ErrBreaksInterface)
	}

	edits := newEditSet(work.fileSet)
	// "func (t T) Method(" becomes "func newName(".
	edits.replace(funcDecl.Recv.Pos(), funcDecl.Name.End(), finalName)
	docFirstWordEdit(edits, funcDecl.Doc, function.Name(), finalName)
	for _, site := range sites {
		qualifier := ""
		if site.pkg.PkgPath != methodPackage.Path() {
			qualifier = qualifierFor(edits, site.pkg, site.file, methodPackage.Path(), methodPackage.Name())
		}
		edits.replace(site.selector.Pos(), site.selector.End(), qualifier+finalName)
	}
	changes, err := buildChanges(edits)
	if err != nil {
		return nil, err
	}
	return commit(ctx, work.root, changes, opts)
}

// checkReceiverUnused wraps ErrReceiverUsed when the named receiver appears in the body.
func checkReceiverUnused(pkg *packages.Package, funcDecl *ast.FuncDecl) error {
	names := funcDecl.Recv.List[0].Names
	if len(names) == 0 || names[0].Name == "_" || funcDecl.Body == nil {
		return nil
	}
	receiver := pkg.TypesInfo.Defs[names[0]]
	if receiver == nil {
		return nil
	}
	var usedErr error
	ast.Inspect(funcDecl.Body, func(node ast.Node) bool {
		if ident, isIdent := node.(*ast.Ident); isIdent && usedErr == nil && pkg.TypesInfo.Uses[ident] == receiver {
			usedErr = fmt.Errorf("receiver %s used at %s: %w", ident.Name, pkg.Fset.Position(ident.Pos()), ErrReceiverUsed)
		}
		return usedErr == nil
	})
	return usedErr
}

// callSites finds every call of method and rejects uses that cannot become a plain function call.
func (w *workspace) callSites(method *types.Func) ([]callSite, error) {
	methodKey := w.keyOf(method)
	var sites []callSite
	seen := map[editKey]bool{}
	for _, pkg := range w.packages {
		parentsByFile := map[*ast.File]*parentMaps{}
		for ident, used := range pkg.TypesInfo.Uses {
			if ident.Name != method.Name() || w.keyOf(used) != methodKey {
				continue
			}
			position := w.fileSet.Position(ident.Pos())
			key := editKey{file: position.Filename, start: position.Offset}
			if seen[key] {
				continue
			}
			seen[key] = true
			file := w.fileOf(pkg, ident.Pos())
			if file == nil {
				continue
			}
			parents, cached := parentsByFile[file]
			if !cached {
				parents = buildParentMaps(file)
				parentsByFile[file] = parents
			}
			selector := parents.selectorOf[ident]
			if selector == nil {
				return nil, fmt.Errorf("use at %s: %w", position, ErrUnsupportedUse)
			}
			selection := pkg.TypesInfo.Selections[selector]
			if selection == nil || selection.Kind() != types.MethodVal {
				return nil, fmt.Errorf("method expression at %s: %w", position, ErrUnsupportedUse)
			}
			if parents.callOf[selector] == nil {
				return nil, fmt.Errorf("method value at %s: %w", position, ErrUnsupportedUse)
			}
			if !isPlainReceiver(selector.X) {
				return nil, fmt.Errorf("receiver expression with possible side effects at %s: %w", position, ErrUnsupportedUse)
			}
			sites = append(sites, callSite{pkg: pkg, file: file, selector: selector})
		}
	}
	return sites, nil
}

// isPlainReceiver accepts identifiers, selector chains of identifiers, and their dereferences,
// because dropping them cannot skip a call, a channel receive or an index expression.
func isPlainReceiver(expr ast.Expr) bool {
	switch typed := expr.(type) {
	case *ast.Ident:
		return true
	case *ast.SelectorExpr:
		return isPlainReceiver(typed.X)
	case *ast.StarExpr:
		return isPlainReceiver(typed.X)
	case *ast.ParenExpr:
		return isPlainReceiver(typed.X)
	}
	return false
}

// checkFunctionName rejects a name already declared in the package, and a call site where a local declaration would capture it.
func checkFunctionName(work *workspace, packagePath, name string, sites []callSite) error {
	for _, pkg := range work.packagesWithPath(packagePath) {
		if pkg.Types.Scope().Lookup(name) != nil {
			return fmt.Errorf("refactor: %s is already declared in package %s: %w", name, packagePath, ErrConflict)
		}
		for _, file := range pkg.Syntax {
			if scope := pkg.TypesInfo.Scopes[file]; scope != nil && scope.Lookup(name) != nil {
				return fmt.Errorf("refactor: %s is already declared in %s: %w", name, fileNameOf(work.fileSet, file.Pos()), ErrConflict)
			}
		}
	}
	for _, site := range sites {
		if site.pkg.PkgPath != packagePath {
			continue
		}
		packageScope := site.pkg.Types.Scope()
		inner := packageScope.Innermost(site.selector.Pos())
		if inner == nil {
			continue
		}
		if scope, object := inner.LookupParent(name, site.selector.Pos()); object != nil && scope != packageScope && scope != types.Universe {
			return fmt.Errorf("refactor: %s is shadowed at %s: %w", name, work.fileSet.Position(site.selector.Pos()), ErrConflict)
		}
	}
	return nil
}
