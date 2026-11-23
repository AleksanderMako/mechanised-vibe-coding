package symbols

import (
	"go/types"
	"slices"
	"strings"
)

// isInterfaceType reports whether object is a named type whose underlying type is an interface.
func isInterfaceType(object types.Object) bool {
	typeName, isTypeName := object.(*types.TypeName)
	if !isTypeName {
		return false
	}
	_, isInterface := types.Unalias(typeName.Type()).Underlying().(*types.Interface)
	return isInterface
}

// findImplementations checks every package level named type of the loaded packages against iface.
func findImplementations(loaded *loadResult, iface Symbol, fallback types.Object) []Symbol {
	listed := map[Symbol]bool{}
	for _, pkg := range loaded.packages {
		// Each test variant has its own copy of the interface; checking against that copy
		// keeps unexported methods and parameter types comparable.
		interfaceType := interfaceFor(pkg.Types, iface, fallback)
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			typeName, isTypeName := scope.Lookup(name).(*types.TypeName)
			if !isTypeName || typeName.IsAlias() {
				continue
			}
			if _, isInterface := typeName.Type().Underlying().(*types.Interface); isInterface {
				continue
			}
			candidate := typeName.Type()
			if types.Implements(candidate, interfaceType) || types.Implements(types.NewPointer(candidate), interfaceType) {
				listed[Symbol{Package: pkg.PkgPath, Name: name}] = true
			}
		}
	}
	result := make([]Symbol, 0, len(listed))
	for symbol := range listed {
		result = append(result, symbol)
	}
	slices.SortFunc(result, func(a, b Symbol) int { return strings.Compare(a.String(), b.String()) })
	return result
}

// interfaceFor returns iface as seen from pkg, or the globally resolved one when pkg cannot see it.
func interfaceFor(pkg *types.Package, iface Symbol, fallback types.Object) *types.Interface {
	object := fallback
	if declaring := findImport(pkg, iface.Package, map[*types.Package]bool{}); declaring != nil {
		if local := lookupIn(declaring, iface.Name); isInterfaceType(local) {
			object = local
		}
	}
	return types.Unalias(object.Type()).Underlying().(*types.Interface)
}
