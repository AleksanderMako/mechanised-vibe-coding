package symbols

import (
	"errors"
	"fmt"
	"go/types"
	"strings"
)

// methodSetMember is the member name that makes "Type.*" stand for every method of Type.
const methodSetMember = "*"

// target is one object whose uses are collected, with the symbol its references report.
type target struct {
	object types.Object
	symbol Symbol
}

// expand resolves sym to the objects it stands for: itself, or every method of Type for "Type.*".
func (l *loadResult) expand(sym Symbol) ([]target, error) {
	typeName, member, hasMember := strings.Cut(sym.Name, ".")
	if !hasMember || member != methodSetMember {
		object, err := l.resolve(sym)
		if err != nil {
			return nil, err
		}
		symbol := sym
		// A method always reports the type that declares it, also when sym names it through embedding.
		if method, isFunc := object.(*types.Func); isFunc {
			if declaring, found := declaringSymbol(method); found {
				symbol = declaring
			}
		}
		return []target{{object: object, symbol: symbol}}, nil
	}
	object, err := l.resolve(Symbol{Package: sym.Package, Name: typeName})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("symbols: %s: %w", sym, ErrNotFound)
		}
		return nil, err
	}
	typeObject, isTypeName := object.(*types.TypeName)
	if !isTypeName {
		return nil, fmt.Errorf("symbols: %s: %s is not a type: %w", sym, typeName, ErrNotFound)
	}
	targets := []target{}
	for _, method := range methodsOf(typeObject) {
		// Fall back to the asked type when the declaring type has no name, as in an embedded interface literal.
		symbol, found := declaringSymbol(method)
		if !found {
			symbol = Symbol{Package: sym.Package, Name: typeName + "." + method.Name()}
		}
		targets = append(targets, target{object: method, symbol: symbol})
	}
	return targets, nil
}

// methodsOf lists the methods of a named type: all interface methods, embedded included,
// or the methods declared with receiver T or *T, never the promoted ones.
func methodsOf(typeObject *types.TypeName) []*types.Func {
	named, isNamed := types.Unalias(typeObject.Type()).(*types.Named)
	if !isNamed {
		return nil
	}
	// An alias of an instantiation such as G[int] stands for the generic declaration.
	named = named.Origin()
	var methods []*types.Func
	if iface, isInterface := named.Underlying().(*types.Interface); isInterface {
		for index := range iface.NumMethods() {
			methods = append(methods, iface.Method(index))
		}
		return methods
	}
	for index := range named.NumMethods() {
		methods = append(methods, named.Method(index))
	}
	return methods
}

// declaringSymbol names a method by the named type that declares it, as "Type.Method",
// so a method on *T reports T and a method of an embedded interface reports that interface.
func declaringSymbol(method *types.Func) (Symbol, bool) {
	receiver := method.Origin().Signature().Recv()
	if receiver == nil {
		return Symbol{}, false
	}
	receiverType := receiver.Type()
	if pointer, isPointer := receiverType.(*types.Pointer); isPointer {
		receiverType = pointer.Elem()
	}
	named, isNamed := types.Unalias(receiverType).(*types.Named)
	if !isNamed {
		return Symbol{}, false
	}
	typeObject := named.Origin().Obj()
	if typeObject.Pkg() == nil {
		return Symbol{}, false
	}
	return Symbol{Package: typeObject.Pkg().Path(), Name: typeObject.Name() + "." + method.Name()}, true
}
