package refactor

import (
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/AleksanderMako/machanised-vibe-coding/symbols"
)

// identUse is one identifier, declaration or use, that denotes the renamed object.
type identUse struct {
	pkg     *packages.Package
	ident   *ast.Ident
	defines bool
}

// rename implements Rename.
func rename(ctx context.Context, root string, sym symbols.Symbol, newName string, opts Options) ([]string, error) {
	if _, err := checkRoot(root); err != nil {
		return nil, err
	}
	if !isValidName(newName) {
		return nil, fmt.Errorf("refactor: %q: %w", newName, ErrBadName)
	}
	work, err := load(ctx, root, opts.Patterns)
	if err != nil {
		return nil, err
	}
	target, err := work.resolve(sym)
	if err != nil {
		return nil, err
	}
	oldName := target.Name()
	if oldName == newName {
		return []string{}, nil
	}
	declared, found := work.findDeclaration(target)
	if !found {
		return nil, fmt.Errorf("refactor: %s is not declared in the loaded packages", sym)
	}
	if variable, isVar := target.(*types.Var); isVar && variable.Embedded() {
		return nil, fmt.Errorf("refactor: %s is an embedded field, rename its type instead: %w", sym, ErrUnsupportedUse)
	}

	// Renaming a type also renames the fields that embed it, because their name is the type name.
	keys := map[objectKey]bool{work.keyOf(target): true}
	if _, isTypeName := target.(*types.TypeName); isTypeName {
		for _, field := range work.embeddedFieldsOf(target) {
			keys[field] = true
		}
	}
	uses := work.identsOf(keys, oldName)

	if err := checkRenameConflicts(work, sym, target, declared, uses, newName); err != nil {
		return nil, err
	}

	edits := newEditSet(work.fileSet)
	for _, use := range uses {
		edits.replace(use.ident.Pos(), use.ident.End(), newName)
	}
	for _, doc := range docOf(declared.path) {
		docFirstWordEdit(edits, doc, oldName, newName)
	}
	changes, err := buildChanges(edits)
	if err != nil {
		return nil, err
	}
	return commit(ctx, work.root, changes, opts)
}

// embeddedFieldsOf returns the keys of struct fields that embed the type typeName.
func (w *workspace) embeddedFieldsOf(typeName types.Object) []objectKey {
	typeKey := w.keyOf(typeName)
	var fields []objectKey
	for _, pkg := range w.packages {
		for ident, defined := range pkg.TypesInfo.Defs {
			field, isVar := defined.(*types.Var)
			if !isVar || !field.Embedded() || ident.Name != typeKey.name {
				continue
			}
			if used := pkg.TypesInfo.Uses[ident]; used != nil && w.keyOf(used) == typeKey {
				fields = append(fields, w.keyOf(field))
			}
		}
	}
	return fields
}

// identsOf returns every identifier in the loaded packages whose object has one of keys.
func (w *workspace) identsOf(keys map[objectKey]bool, name string) []identUse {
	var uses []identUse
	for _, pkg := range w.packages {
		for ident, defined := range pkg.TypesInfo.Defs {
			if defined != nil && ident.Name == name && keys[w.keyOf(defined)] {
				uses = append(uses, identUse{pkg: pkg, ident: ident, defines: true})
			}
		}
		for ident, used := range pkg.TypesInfo.Uses {
			if ident.Name == name && keys[w.keyOf(used)] {
				uses = append(uses, identUse{pkg: pkg, ident: ident})
			}
		}
	}
	return uses
}

// checkRenameConflicts applies rules 6 to 8 of the contract before anything is written.
func checkRenameConflicts(work *workspace, sym symbols.Symbol, target types.Object, declared *declaration,
	uses []identUse, newName string) error {
	declaringPath := target.Pkg().Path()

	// An exported name used elsewhere cannot become unexported.
	if target.Exported() && !token.IsExported(newName) {
		for _, use := range uses {
			if use.pkg.PkgPath != declaringPath {
				return fmt.Errorf("refactor: %s is used in package %s and cannot become unexported: %w",
					sym, use.pkg.PkgPath, ErrConflict)
			}
		}
	}

	switch typed := target.(type) {
	case *types.Func:
		signature := typed.Type().(*types.Signature)
		if signature.Recv() != nil {
			return checkMethodRename(work, sym, typed, signature, newName)
		}
	case *types.Var:
		if typed.IsField() {
			return checkFieldRename(work, sym, declared, newName)
		}
	}
	if isPackageLevel(target) {
		if err := checkPackageLevelName(work, declaringPath, newName); err != nil {
			return err
		}
	}
	return checkShadowing(work, uses, newName)
}

// checkPackageLevelName rejects newName when the package or one of its files already declares it,
// or when code of the package uses a predeclared identifier of that name that the rename would capture.
func checkPackageLevelName(work *workspace, packagePath, newName string) error {
	for _, pkg := range work.packagesWithPath(packagePath) {
		if pkg.Types.Scope().Lookup(newName) != nil {
			return fmt.Errorf("refactor: %s is already declared in package %s: %w", newName, packagePath, ErrConflict)
		}
		for _, file := range pkg.Syntax {
			if scope := pkg.TypesInfo.Scopes[file]; scope != nil && scope.Lookup(newName) != nil {
				return fmt.Errorf("refactor: %s is already declared in %s: %w",
					newName, fileNameOf(work.fileSet, file.Pos()), ErrConflict)
			}
		}
		for ident, used := range pkg.TypesInfo.Uses {
			if ident.Name == newName && used.Parent() == types.Universe {
				return fmt.Errorf("refactor: package %s uses the predeclared %s: %w", packagePath, newName, ErrConflict)
			}
		}
	}
	return nil
}

// checkShadowing rejects newName when a declaration between a bare reference and the package scope would capture it.
func checkShadowing(work *workspace, uses []identUse, newName string) error {
	for _, use := range uses {
		if use.defines {
			continue
		}
		packageScope := use.pkg.Types.Scope()
		inner := packageScope.Innermost(use.ident.Pos())
		if inner == nil {
			continue
		}
		scope, object := inner.LookupParent(newName, use.ident.Pos())
		if object == nil || scope == packageScope || scope == types.Universe {
			continue
		}
		// Selectors and composite literal keys are not resolved through scopes, so nothing can shadow them.
		if parents := buildParentMapsFor(work, use); parents.selectorOf[use.ident] != nil || parents.isKey[use.ident] {
			continue
		}
		position := work.fileSet.Position(use.ident.Pos())
		return fmt.Errorf("refactor: %s is shadowed by a local declaration at %s: %w", newName, position, ErrConflict)
	}
	return nil
}

// buildParentMapsFor indexes the parents in the file that holds use.
func buildParentMapsFor(work *workspace, use identUse) *parentMaps {
	file := work.fileOf(use.pkg, use.ident.Pos())
	if file == nil {
		return &parentMaps{}
	}
	return buildParentMaps(file)
}

// checkMethodRename rejects a name already in the method set and a rename that breaks a used interface.
func checkMethodRename(work *workspace, sym symbols.Symbol, method *types.Func, signature *types.Signature, newName string) error {
	receiverType := signature.Recv().Type()
	if pointer, isPointer := receiverType.(*types.Pointer); isPointer {
		receiverType = pointer.Elem()
	}
	if existing, _, _ := types.LookupFieldOrMethod(receiverType, true, method.Pkg(), newName); existing != nil {
		return fmt.Errorf("refactor: %s already has a member named %s: %w", receiverType, newName, ErrConflict)
	}
	if types.IsInterface(receiverType) {
		return nil
	}
	named, isNamed := types.Unalias(receiverType).(*types.Named)
	if !isNamed {
		return nil
	}
	if iface, breaks := work.brokenInterface(named.Origin().Obj(), method.Name()); breaks {
		return fmt.Errorf("refactor: renaming %s stops %s from implementing %s: %w",
			sym, named.Obj().Name(), iface, ErrBreaksInterface)
	}
	return nil
}

// checkFieldRename rejects a name already used by a field of the declaring struct or a member of the named type.
func checkFieldRename(work *workspace, sym symbols.Symbol, declared *declaration, newName string) error {
	typeName, _, _ := strings.Cut(sym.Name, ".")
	if owner := work.findPackage(sym.Package); owner != nil {
		if named := owner.Scope().Lookup(typeName); named != nil {
			if existing, _, _ := types.LookupFieldOrMethod(named.Type(), true, owner, newName); existing != nil {
				return fmt.Errorf("refactor: %s already has a member named %s: %w", typeName, newName, ErrConflict)
			}
		}
	}
	// A promoted field belongs to an embedded struct, whose own field list must stay unique too.
	for _, node := range declared.path {
		structType, isStruct := node.(*ast.StructType)
		if !isStruct {
			continue
		}
		if fields, isStructType := declared.pkg.TypesInfo.TypeOf(structType).(*types.Struct); isStructType {
			for field := range fields.Fields() {
				if field.Name() == newName {
					return fmt.Errorf("refactor: the struct declaring %s already has a field %s: %w", sym, newName, ErrConflict)
				}
			}
		}
		break
	}
	return nil
}
