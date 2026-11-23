package refactor

import (
	"go/types"

	"golang.org/x/tools/go/packages"
)

// brokenInterface returns the name of an interface used as a type in the loaded packages
// that typeName satisfies today through a method called methodName, which an operation
// is about to rename or remove.
func (w *workspace) brokenInterface(typeName *types.TypeName, methodName string) (string, bool) {
	typePackage := typeName.Pkg()
	if typePackage == nil {
		return "", false
	}
	for _, pkg := range w.packages {
		// T must be looked up from pkg itself so it and the interface share one type universe.
		visible := findImport(pkg.Types, typePackage.Path(), map[*types.Package]bool{})
		if visible == nil {
			continue
		}
		local, isTypeName := visible.Scope().Lookup(typeName.Name()).(*types.TypeName)
		if !isTypeName {
			continue
		}
		named, isNamed := types.Unalias(local.Type()).(*types.Named)
		if !isNamed || named.TypeParams().Len() > 0 {
			continue
		}
		for _, iface := range usedInterfaces(pkg) {
			underlying := iface.Underlying().(*types.Interface)
			if !hasMethodNamed(underlying, methodName, typePackage.Path()) {
				continue
			}
			if types.Implements(named, underlying) || types.Implements(types.NewPointer(named), underlying) {
				return iface.Obj().Name(), true
			}
		}
	}
	return "", false
}

// hasMethodNamed reports whether iface has a method called name; unexported names must come from packagePath.
func hasMethodNamed(iface *types.Interface, name, packagePath string) bool {
	for method := range iface.Methods() {
		if method.Name() != name {
			continue
		}
		if method.Exported() || (method.Pkg() != nil && method.Pkg().Path() == packagePath) {
			return true
		}
	}
	return false
}

// usedInterfaces lists the named interface types that appear in a type of any expression
// or variable of pkg: in declarations, conversions, assignments and signatures.
func usedInterfaces(pkg *packages.Package) []*types.Named {
	found := map[*types.Named]bool{}
	visited := map[types.Type]bool{}
	var result []*types.Named
	var walk func(t types.Type)
	walk = func(t types.Type) {
		if t == nil || visited[t] {
			return
		}
		visited[t] = true
		switch typed := t.(type) {
		case *types.Alias:
			walk(types.Unalias(typed))
		case *types.Named:
			if _, isInterface := typed.Underlying().(*types.Interface); isInterface && !found[typed] {
				found[typed] = true
				result = append(result, typed)
			}
			for argument := range typed.TypeArgs().Types() {
				walk(argument)
			}
		case *types.Pointer:
			walk(typed.Elem())
		case *types.Slice:
			walk(typed.Elem())
		case *types.Array:
			walk(typed.Elem())
		case *types.Chan:
			walk(typed.Elem())
		case *types.Map:
			walk(typed.Key())
			walk(typed.Elem())
		case *types.Tuple:
			for variable := range typed.Variables() {
				walk(variable.Type())
			}
		case *types.Signature:
			walk(typed.Params())
			walk(typed.Results())
		}
	}
	for _, value := range pkg.TypesInfo.Types {
		walk(value.Type)
	}
	for _, object := range pkg.TypesInfo.Defs {
		if variable, isVar := object.(*types.Var); isVar {
			walk(variable.Type())
		}
	}
	for _, object := range pkg.TypesInfo.Uses {
		if variable, isVar := object.(*types.Var); isVar {
			walk(variable.Type())
		}
	}
	return result
}
