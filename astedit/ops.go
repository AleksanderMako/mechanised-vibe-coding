package astedit

import (
	"fmt"
	"go/token"
	"go/types"
	"strings"

	"github.com/dave/dst"
)

// addTableCaseOp appends one element to the table of a test function.
type addTableCaseOp struct {
	file, testFunc, caseSrc string
}

func (op *addTableCaseOp) File() string { return op.file }

func (op *addTableCaseOp) Describe() string {
	return fmt.Sprintf("AddTableCase %s in %s", op.testFunc, op.file)
}

func (op *addTableCaseOp) apply(file *dst.File) error {
	function := findFunc(file, op.testFunc)
	if function == nil {
		return fmt.Errorf("%w: func %s", ErrNotFound, op.testFunc)
	}

	var tables []*dst.CompositeLit
	dst.Inspect(function.Body, func(node dst.Node) bool {
		switch statement := node.(type) {
		case *dst.AssignStmt:
			if statement.Tok != token.DEFINE && statement.Tok != token.ASSIGN {
				break
			}
			for _, value := range statement.Rhs {
				if literal, ok := value.(*dst.CompositeLit); ok && isTableType(literal.Type) {
					tables = append(tables, literal)
				}
			}
		case *dst.RangeStmt:
			if literal, ok := statement.X.(*dst.CompositeLit); ok && isTableType(literal.Type) {
				tables = append(tables, literal)
			}
		}
		return true
	})
	switch {
	case len(tables) == 0:
		return fmt.Errorf("%w: no table in func %s", ErrNotFound, op.testFunc)
	case len(tables) > 1:
		return fmt.Errorf("%w: %d tables in func %s", ErrAmbiguous, len(tables), op.testFunc)
	}
	table := tables[0]

	element, err := parseElement(op.caseSrc)
	if err != nil {
		return err
	}
	// A map table takes key: value pairs, a slice table takes bare composite literals.
	_, isMap := table.Type.(*dst.MapType)
	if pair, isPair := element.(*dst.KeyValueExpr); isMap {
		if !isPair {
			return fmt.Errorf("%w: map table needs a key: value case, got %q", ErrBadSource, op.caseSrc)
		}
		if _, ok := pair.Value.(*dst.CompositeLit); !ok {
			return fmt.Errorf("%w: case value is not a composite literal: %q", ErrBadSource, op.caseSrc)
		}
	} else if _, ok := element.(*dst.CompositeLit); !ok {
		return fmt.Errorf("%w: case is not a composite literal: %q", ErrBadSource, op.caseSrc)
	}

	// Each element on its own line keeps the table's one-case-per-line layout.
	for _, existing := range table.Elts {
		existing.Decorations().Before = dst.NewLine
		existing.Decorations().After = dst.NewLine
	}
	element.Decorations().Before = dst.NewLine
	element.Decorations().After = dst.NewLine
	table.Elts = append(table.Elts, element)
	return nil
}

// findFunc returns the top level function name, ignoring methods.
func findFunc(file *dst.File, name string) *dst.FuncDecl {
	for _, declaration := range file.Decls {
		function, ok := declaration.(*dst.FuncDecl)
		if ok && function.Recv == nil && function.Name.Name == name && function.Body != nil {
			return function
		}
	}
	return nil
}

// isTableType accepts []struct{...}, []Name and map[...]struct{...}; slices of builtin types are not tables.
func isTableType(expression dst.Expr) bool {
	switch typed := expression.(type) {
	case *dst.ArrayType:
		if typed.Len != nil {
			return false
		}
		switch element := typed.Elt.(type) {
		case *dst.StructType, *dst.SelectorExpr:
			return true
		case *dst.Ident:
			_, builtin := types.Universe.Lookup(element.Name).(*types.TypeName)
			return !builtin
		}
	case *dst.MapType:
		_, isStruct := typed.Value.(*dst.StructType)
		return isStruct
	}
	return false
}

// addStructFieldOp inserts one field into a struct type.
type addStructFieldOp struct {
	file, typeName, fieldSrc, after string
}

func (op *addStructFieldOp) File() string { return op.file }

func (op *addStructFieldOp) Describe() string {
	return fmt.Sprintf("AddStructField %q to %s in %s", op.fieldSrc, op.typeName, op.file)
}

func (op *addStructFieldOp) apply(file *dst.File) error {
	structType, err := findStruct(file, op.typeName)
	if err != nil {
		return err
	}
	field, err := parseField(op.fieldSrc)
	if err != nil {
		return err
	}
	for _, existing := range structType.Fields.List {
		for _, name := range fieldNames(field) {
			if hasName(fieldNames(existing), name) {
				return fmt.Errorf("%w: field %s in %s", ErrExists, name, op.typeName)
			}
		}
	}

	insertAt := len(structType.Fields.List)
	if op.after != "" {
		insertAt = -1
		for index, existing := range structType.Fields.List {
			if hasName(fieldNames(existing), op.after) {
				insertAt = index + 1
				break
			}
		}
		if insertAt < 0 {
			return fmt.Errorf("%w: field %s in %s", ErrNotFound, op.after, op.typeName)
		}
	}

	field.Decs.Before = dst.NewLine
	field.Decs.After = dst.NewLine
	list := structType.Fields.List
	list = append(list[:insertAt], append([]*dst.Field{field}, list[insertAt:]...)...)
	structType.Fields.List = list
	return nil
}

// removeStructFieldOp deletes one field, with its comments, from a struct type.
type removeStructFieldOp struct {
	file, typeName, fieldName string
}

func (op *removeStructFieldOp) File() string { return op.file }

func (op *removeStructFieldOp) Describe() string {
	return fmt.Sprintf("RemoveStructField %s from %s in %s", op.fieldName, op.typeName, op.file)
}

func (op *removeStructFieldOp) apply(file *dst.File) error {
	structType, err := findStruct(file, op.typeName)
	if err != nil {
		return err
	}
	for index, field := range structType.Fields.List {
		if !hasName(fieldNames(field), op.fieldName) {
			continue
		}
		// In `A, B int` only the one name goes; the shared line and its comments stay for the other.
		if len(field.Names) > 1 {
			var remaining []*dst.Ident
			for _, name := range field.Names {
				if name.Name != op.fieldName {
					remaining = append(remaining, name)
				}
			}
			field.Names = remaining
			return nil
		}
		list := structType.Fields.List
		structType.Fields.List = append(list[:index:index], list[index+1:]...)
		if len(structType.Fields.List) == 0 {
			// Dropping leftover spacing and comments lets the printer write struct{}.
			structType.Fields.Decs = dst.FieldListDecorations{}
			structType.Fields.List = nil
		}
		return nil
	}
	return fmt.Errorf("%w: field %s in %s", ErrNotFound, op.fieldName, op.typeName)
}

// setDocOp replaces the doc comment of a top level symbol or method.
type setDocOp struct {
	file, symbol, doc string
}

func (op *setDocOp) File() string { return op.file }

func (op *setDocOp) Describe() string {
	return fmt.Sprintf("SetDoc %s in %s", op.symbol, op.file)
}

func (op *setDocOp) apply(file *dst.File) error {
	decorations, err := findDocDecorations(file, op.symbol)
	if err != nil {
		return err
	}
	if !replaceDoc(decorations, op.doc) {
		return errNoChange
	}
	return nil
}

// findDocDecorations returns the decorations that hold the doc comment of symbol.
func findDocDecorations(file *dst.File, symbol string) (*dst.Decorations, error) {
	var matches []*dst.Decorations
	typeName, methodName, isMethod := strings.Cut(symbol, ".")
	for _, declaration := range file.Decls {
		switch typed := declaration.(type) {
		case *dst.FuncDecl:
			if isMethod {
				if typed.Recv != nil && len(typed.Recv.List) == 1 && typed.Name.Name == methodName && typeBaseName(typed.Recv.List[0].Type) == typeName {
					matches = append(matches, &typed.Decs.Start)
				}
			} else if typed.Recv == nil && typed.Name.Name == symbol {
				matches = append(matches, &typed.Decs.Start)
			}
		case *dst.GenDecl:
			if isMethod || typed.Tok == token.IMPORT {
				continue
			}
			for _, spec := range typed.Specs {
				if specDeclares(spec, symbol) {
					matches = append(matches, specDocDecorations(typed, spec))
				}
			}
		}
	}
	switch {
	case len(matches) == 0:
		return nil, fmt.Errorf("%w: symbol %s", ErrNotFound, symbol)
	case len(matches) > 1:
		return nil, fmt.Errorf("%w: symbol %s", ErrAmbiguous, symbol)
	}
	return matches[0], nil
}

// specDeclares reports whether a type or value spec declares name.
func specDeclares(spec dst.Spec, name string) bool {
	switch typed := spec.(type) {
	case *dst.TypeSpec:
		return typed.Name.Name == name
	case *dst.ValueSpec:
		for _, ident := range typed.Names {
			if ident.Name == name {
				return true
			}
		}
	}
	return false
}

// specDocDecorations picks the spec's own doc inside a ( ) group and the declaration's doc otherwise.
func specDocDecorations(declaration *dst.GenDecl, spec dst.Spec) *dst.Decorations {
	if !declaration.Lparen {
		return &declaration.Decs.Start
	}
	switch typed := spec.(type) {
	case *dst.TypeSpec:
		return &typed.Decs.Start
	case *dst.ValueSpec:
		return &typed.Decs.Start
	}
	return &declaration.Decs.Start
}

// setConstOp replaces the value and optionally the doc of a top level constant.
type setConstOp struct {
	file, name, valueSrc, doc string
}

func (op *setConstOp) File() string { return op.file }

func (op *setConstOp) Describe() string {
	return fmt.Sprintf("SetConst %s = %s in %s", op.name, op.valueSrc, op.file)
}

func (op *setConstOp) apply(file *dst.File) error {
	var foundDecl *dst.GenDecl
	var foundSpec *dst.ValueSpec
	for _, declaration := range file.Decls {
		general, ok := declaration.(*dst.GenDecl)
		if !ok || general.Tok != token.CONST {
			continue
		}
		for _, spec := range general.Specs {
			if specDeclares(spec, op.name) {
				foundDecl, foundSpec = general, spec.(*dst.ValueSpec)
			}
		}
	}
	if foundSpec == nil {
		return fmt.Errorf("%w: const %s", ErrNotFound, op.name)
	}
	if len(foundSpec.Names) > 1 {
		return fmt.Errorf("%w: const %s is declared together with other names", ErrAmbiguous, op.name)
	}

	value, err := parseExpression(op.valueSrc)
	if err != nil {
		return err
	}
	foundSpec.Values = []dst.Expr{value}
	if op.doc != "" {
		replaceDoc(specDocDecorations(foundDecl, foundSpec), op.doc)
	}
	return nil
}

// addConformanceOp adds a compile time interface check after a type declaration.
type addConformanceOp struct {
	file, typeName, ifaceSrc string
}

func (op *addConformanceOp) File() string { return op.file }

func (op *addConformanceOp) Describe() string {
	return fmt.Sprintf("AddConformance %s implements %s in %s", op.typeName, op.ifaceSrc, op.file)
}

func (op *addConformanceOp) apply(file *dst.File) error {
	typeDecl, _ := findTypeSpec(file, op.typeName)
	if typeDecl == nil {
		return fmt.Errorf("%w: type %s", ErrNotFound, op.typeName)
	}

	scratch, err := parseSnippetFile(fmt.Sprintf("package p\n\nvar _ %s = (*%s)(nil)\n", op.ifaceSrc, op.typeName))
	if err != nil {
		return err
	}
	if len(scratch.Decls) != 1 {
		return fmt.Errorf("%w: interface %q", ErrBadSource, op.ifaceSrc)
	}
	conformance := scratch.Decls[0].(*dst.GenDecl)
	wantSpec := conformance.Specs[0].(*dst.ValueSpec)
	wantIface, wantValue := exprText(wantSpec.Type), exprText(wantSpec.Values[0])

	if hasConformance(file, wantIface, wantValue) {
		return errNoChange
	}

	insertAt := -1
	for index, declaration := range file.Decls {
		if declaration == typeDecl {
			insertAt = index + 1
			break
		}
	}
	conformance.Decs.Before = dst.EmptyLine
	conformance.Decs.After = dst.EmptyLine
	if insertAt < len(file.Decls) {
		file.Decls[insertAt].Decorations().Before = dst.EmptyLine
	}
	decls := file.Decls
	file.Decls = append(decls[:insertAt:insertAt], append([]dst.Decl{conformance}, decls[insertAt:]...)...)
	return nil
}

// hasConformance looks for an existing top level `var _ Iface = (*T)(nil)` with the same text.
func hasConformance(file *dst.File, wantIface, wantValue string) bool {
	for _, declaration := range file.Decls {
		general, ok := declaration.(*dst.GenDecl)
		if !ok || general.Tok != token.VAR {
			continue
		}
		for _, spec := range general.Specs {
			valueSpec := spec.(*dst.ValueSpec)
			if len(valueSpec.Names) != 1 || valueSpec.Names[0].Name != "_" || valueSpec.Type == nil || len(valueSpec.Values) != 1 {
				continue
			}
			if exprText(valueSpec.Type) == wantIface && exprText(valueSpec.Values[0]) == wantValue {
				return true
			}
		}
	}
	return false
}
