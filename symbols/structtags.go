package symbols

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"go/types"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
)

// structFields describes the fields of a struct type, reading the declaring file for the source text.
func structFields(loaded *loadResult, typeName *types.TypeName, structType *types.Struct) ([]FieldTag, error) {
	position := loaded.fileSet.Position(typeName.Pos())
	path, err := loaded.expandGoroot(position.Filename)
	if err != nil {
		return nil, err
	}
	// Parse the declaring file afresh so dependencies without loaded syntax work the same way.
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("symbols: parse %s: %w", path, err)
	}
	spec := findTypeSpec(fileSet, file, typeName.Name(), position.Line)
	if spec != nil {
		if literal, isStructLiteral := spec.Type.(*ast.StructType); isStructLiteral {
			return fieldsFromSyntax(fileSet, literal)
		}
	}
	// The struct comes from another named type (type B A), so no struct literal is written here.
	return fieldsFromTypes(typeName.Pkg(), structType), nil
}

// gorootPlaceholder starts the file names that export data records for standard library packages.
const gorootPlaceholder = "$GOROOT"

// expandGoroot turns a "$GOROOT/src/..." file name from export data into a real path.
func (l *loadResult) expandGoroot(path string) (string, error) {
	if !strings.HasPrefix(path, gorootPlaceholder) {
		return path, nil
	}
	// Ask the go tool in root so the toolchain that compiled the export data answers.
	command := exec.Command("go", "env", "GOROOT")
	command.Dir = l.root
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("symbols: go env GOROOT: %w", err)
	}
	return filepath.Join(strings.TrimSpace(string(output)), strings.TrimPrefix(path, gorootPlaceholder)), nil
}

// findTypeSpec returns the declaration of name on line, or nil.
func findTypeSpec(fileSet *token.FileSet, file *ast.File, name string, line int) *ast.TypeSpec {
	var found *ast.TypeSpec
	ast.Inspect(file, func(node ast.Node) bool {
		spec, isSpec := node.(*ast.TypeSpec)
		if isSpec && spec.Name.Name == name && fileSet.Position(spec.Name.Pos()).Line == line {
			found = spec
		}
		return found == nil
	})
	return found
}

// fieldsFromSyntax lists the fields of a struct literal in declaration order, one entry per name.
func fieldsFromSyntax(fileSet *token.FileSet, literal *ast.StructType) ([]FieldTag, error) {
	fields := []FieldTag{}
	for _, field := range literal.Fields.List {
		var typeText bytes.Buffer
		if err := printer.Fprint(&typeText, fileSet, field.Type); err != nil {
			return nil, fmt.Errorf("symbols: print field type: %w", err)
		}
		rawTag := ""
		if field.Tag != nil {
			unquoted, err := strconv.Unquote(field.Tag.Value)
			if err != nil {
				return nil, fmt.Errorf("symbols: field tag %s: %w", field.Tag.Value, err)
			}
			rawTag = unquoted
		}
		if len(field.Names) == 0 {
			name := embeddedName(field.Type)
			fields = append(fields, newFieldTag(name, typeText.String(), true, rawTag))
			continue
		}
		for _, name := range field.Names {
			fields = append(fields, newFieldTag(name.Name, typeText.String(), false, rawTag))
		}
	}
	return fields, nil
}

// fieldsFromTypes lists the fields from type information when no struct literal is available.
func fieldsFromTypes(pkg *types.Package, structType *types.Struct) []FieldTag {
	fields := []FieldTag{}
	qualifier := types.RelativeTo(pkg)
	for index := range structType.NumFields() {
		field := structType.Field(index)
		fields = append(fields, newFieldTag(field.Name(), types.TypeString(field.Type(), qualifier), field.Embedded(), structType.Tag(index)))
	}
	return fields
}

// embeddedName strips pointers, package qualifiers and type arguments from an embedded field type.
func embeddedName(expression ast.Expr) string {
	for {
		switch typed := expression.(type) {
		case *ast.StarExpr:
			expression = typed.X
		case *ast.SelectorExpr:
			return typed.Sel.Name
		case *ast.IndexExpr:
			expression = typed.X
		case *ast.IndexListExpr:
			expression = typed.X
		case *ast.ParenExpr:
			expression = typed.X
		case *ast.Ident:
			return typed.Name
		default:
			return ""
		}
	}
}

// newFieldTag fills a FieldTag and splits the json part of the tag.
func newFieldTag(name, typeText string, embedded bool, rawTag string) FieldTag {
	fieldTag := FieldTag{
		Field:    name,
		Type:     typeText,
		Embedded: embedded,
		Exported: ast.IsExported(name),
		Tag:      rawTag,
	}
	if jsonTag, hasJSON := reflect.StructTag(rawTag).Lookup("json"); hasJSON {
		parts := strings.Split(jsonTag, ",")
		fieldTag.JSON = parts[0]
		for _, option := range parts[1:] {
			if option == "omitempty" {
				fieldTag.OmitEmpty = true
			}
		}
	}
	return fieldTag
}
