package symbols

import (
	"bytes"
	"cmp"
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"
)

// referencePosition is the identity used to drop the duplicates test variants produce.
type referencePosition struct {
	file   string
	line   int
	column int
}

// findReferences walks every file of every loaded package for identifiers using target.
func findReferences(loaded *loadResult, target types.Object) ([]Reference, error) {
	targetKey := loaded.keyOf(target)
	seen := map[referencePosition]bool{}
	sourceLines := map[string][][]byte{}
	references := []Reference{}
	for _, pkg := range loaded.packages {
		matches := matchingIdents(loaded, pkg, targetKey)
		if len(matches) == 0 {
			continue
		}
		for _, file := range pkg.Syntax {
			fileReferences, err := referencesInFile(loaded, pkg, file, matches, seen, sourceLines)
			if err != nil {
				return nil, err
			}
			references = append(references, fileReferences...)
		}
	}
	slices.SortFunc(references, func(a, b Reference) int {
		return cmp.Or(
			cmp.Compare(a.Package, b.Package),
			cmp.Compare(a.File, b.File),
			cmp.Compare(a.Line, b.Line),
			cmp.Compare(a.Column, b.Column),
		)
	})
	return references, nil
}

// matchingIdents returns the identifiers of pkg that use the target object.
// Uses never holds a declaring identifier, so declarations are left out by construction.
func matchingIdents(loaded *loadResult, pkg *packages.Package, targetKey objectKey) map[*ast.Ident]types.Object {
	matches := map[*ast.Ident]types.Object{}
	for ident, object := range pkg.TypesInfo.Uses {
		if object.Name() != targetKey.name {
			continue
		}
		if loaded.keyOf(object) == targetKey {
			matches[ident] = object
		}
	}
	return matches
}

// referencesInFile classifies the matching identifiers of one file using their enclosing nodes.
func referencesInFile(loaded *loadResult, pkg *packages.Package, file *ast.File, matches map[*ast.Ident]types.Object,
	seen map[referencePosition]bool, sourceLines map[string][][]byte) ([]Reference, error) {
	var references []Reference
	var readErr error
	var stack []ast.Node
	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return false
		}
		if ident, isIdent := node.(*ast.Ident); isIdent {
			if object, matched := matches[ident]; matched {
				reference, err := makeReference(loaded, pkg, ident, object, stack, seen, sourceLines)
				if err != nil && readErr == nil {
					readErr = err
				}
				if reference != nil {
					references = append(references, *reference)
				}
			}
		}
		stack = append(stack, node)
		return true
	})
	return references, readErr
}

// makeReference builds the Reference for ident, or returns nil when that position was already reported.
func makeReference(loaded *loadResult, pkg *packages.Package, ident *ast.Ident, object types.Object, parents []ast.Node,
	seen map[referencePosition]bool, sourceLines map[string][][]byte) (*Reference, error) {
	position := loaded.fileSet.Position(ident.Pos())
	key := referencePosition{file: position.Filename, line: position.Line, column: position.Column}
	if seen[key] {
		return nil, nil
	}
	seen[key] = true
	lines, err := linesOf(position.Filename, sourceLines)
	if err != nil {
		return nil, err
	}
	context := ""
	if position.Line >= 1 && position.Line <= len(lines) {
		context = strings.TrimSpace(string(lines[position.Line-1]))
	}
	return &Reference{
		Package: pkg.PkgPath,
		File:    position.Filename,
		Line:    position.Line,
		Column:  position.Column,
		Kind:    classify(ident, object, parents),
		Test:    strings.HasSuffix(position.Filename, "_test.go"),
		Context: context,
	}, nil
}

// linesOf reads a file once and keeps its lines for later references in it.
func linesOf(path string, cache map[string][][]byte) ([][]byte, error) {
	if lines, cached := cache[path]; cached {
		return lines, nil
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("symbols: read %s: %w", path, err)
	}
	lines := bytes.Split(content, []byte("\n"))
	cache[path] = lines
	return lines, nil
}

// classify decides the Kind of one use from the nodes enclosing the identifier, innermost last.
func classify(ident *ast.Ident, object types.Object, parents []ast.Node) Kind {
	_, isType := object.(*types.TypeName)
	var node ast.Node = ident
	index := len(parents) - 1
	// Look through x.Name so the selector stands for the identifier.
	if index >= 0 {
		if selector, isSelector := parents[index].(*ast.SelectorExpr); isSelector && selector.Sel == ident {
			node = selector
			index--
		}
	}
	// Look through an explicit instantiation such as F[int] so F[int](x) still counts as a call.
	if index >= 0 && isGeneric(object) && isInstantiationOf(parents[index], node) {
		node = parents[index]
		index--
	}
	if index >= 0 {
		if call, isCall := parents[index].(*ast.CallExpr); isCall && call.Fun == node {
			if isType {
				return KindConversion
			}
			return KindCall
		}
	}
	if isType && isEmbeddedType(node, parents[:index+1]) {
		return KindEmbed
	}
	if isType {
		return KindType
	}
	return KindValue
}

// isGeneric reports whether object is a function or type with type parameters.
func isGeneric(object types.Object) bool {
	switch typed := object.(type) {
	case *types.Func:
		signature, isSignature := typed.Type().(*types.Signature)
		return isSignature && signature.TypeParams().Len() > 0
	case *types.TypeName:
		named, isNamed := typed.Type().(*types.Named)
		return isNamed && named.TypeParams().Len() > 0
	}
	return false
}

// isInstantiationOf reports whether parent is node[...] with node as the indexed expression.
func isInstantiationOf(parent ast.Node, node ast.Node) bool {
	switch typed := parent.(type) {
	case *ast.IndexExpr:
		return typed.X == node
	case *ast.IndexListExpr:
		return typed.X == node
	}
	return false
}

// isEmbeddedType reports whether node, possibly behind * or [...], is the type of a field
// without names in a struct or interface; parents holds the enclosing nodes, innermost last.
func isEmbeddedType(node ast.Node, parents []ast.Node) bool {
	index := len(parents) - 1
	for index >= 0 {
		star, isStar := parents[index].(*ast.StarExpr)
		if isStar && star.X == node {
			node = star
			index--
			continue
		}
		if isInstantiationOf(parents[index], node) {
			node = parents[index]
			index--
			continue
		}
		break
	}
	// The chain must be Field, then FieldList, then a struct or interface type.
	if index < 2 {
		return false
	}
	field, isField := parents[index].(*ast.Field)
	if !isField || len(field.Names) != 0 || field.Type != node {
		return false
	}
	if _, isFieldList := parents[index-1].(*ast.FieldList); !isFieldList {
		return false
	}
	switch parents[index-2].(type) {
	case *ast.StructType, *ast.InterfaceType:
		return true
	}
	return false
}
