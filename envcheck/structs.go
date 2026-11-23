package envcheck

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
)

// parsePackageStructs parses the non test Go files of pkgDir and indexes their top level struct types by name.
func parsePackageStructs(pkgDir string) (map[string]*ast.StructType, error) {
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return nil, fmt.Errorf("envcheck: read package dir: %w", err)
	}
	fileSet := token.NewFileSet()
	structTypes := map[string]*ast.StructType{}
	parsedFiles := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fileSet, filepath.Join(pkgDir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("envcheck: parse %s: %w", name, err)
		}
		parsedFiles++
		for _, declaration := range file.Decls {
			general, ok := declaration.(*ast.GenDecl)
			if !ok || general.Tok != token.TYPE {
				continue
			}
			for _, spec := range general.Specs {
				typeSpec := spec.(*ast.TypeSpec)
				if structType, ok := typeSpec.Type.(*ast.StructType); ok {
					structTypes[typeSpec.Name.Name] = structType
				}
			}
		}
	}
	if parsedFiles == 0 {
		return nil, fmt.Errorf("envcheck: no Go files in %s", pkgDir)
	}
	return structTypes, nil
}

// fieldWalker collects Vars while walking nested structs.
type fieldWalker struct {
	structTypes map[string]*ast.StructType
	visiting    map[string]bool // named structs on the current path, so a self reference cannot loop forever
	vars        []Var
}

// walk visits the fields of structType in declaration order, prefixing paths with prefix.
func (walker *fieldWalker) walk(structType *ast.StructType, prefix string) {
	for _, field := range structType.Fields.List {
		tag := fieldTag(field)
		argTag := tag.Get("arg")
		if argTag == "-" {
			continue
		}
		fieldNames := make([]string, 0, len(field.Names))
		for _, name := range field.Names {
			fieldNames = append(fieldNames, name.Name)
		}
		embedded := len(fieldNames) == 0
		if embedded {
			// An embedded field is named after its type, as in Go itself.
			fieldNames = []string{typeName(field.Type)}
		}
		nestedStruct, nestedName := walker.nestedStruct(field.Type)
		for _, fieldName := range fieldNames {
			path := joinPath(prefix, fieldName)
			if nestedStruct != nil {
				if nestedName != "" {
					walker.visiting[nestedName] = true
				}
				walker.walk(nestedStruct, path)
				if nestedName != "" {
					delete(walker.visiting, nestedName)
				}
				continue
			}
			if variable, ok := varFromTag(argTag, tag, fieldName, path); ok {
				walker.vars = append(walker.vars, variable)
			}
		}
	}
}

// nestedStruct returns the struct a field type refers to within this package, and its name when it is named.
func (walker *fieldWalker) nestedStruct(expression ast.Expr) (*ast.StructType, string) {
	switch typed := expression.(type) {
	case *ast.StructType:
		return typed, ""
	case *ast.StarExpr:
		return walker.nestedStruct(typed.X)
	case *ast.Ident:
		structType, ok := walker.structTypes[typed.Name]
		if !ok || walker.visiting[typed.Name] {
			return nil, ""
		}
		return structType, typed.Name
	}
	return nil, ""
}

// varFromTag builds a Var from the arg tag parts; ok is false when there is no env part.
func varFromTag(argTag string, tag reflect.StructTag, fieldName, path string) (Var, bool) {
	variable := Var{Field: path}
	hasEnv := false
	for _, part := range strings.Split(argTag, ",") {
		part = strings.TrimSpace(part)
		switch {
		case part == "env":
			// go-arg derives the variable name from the upper cased field name.
			variable.Name = strings.ToUpper(fieldName)
			hasEnv = true
		case strings.HasPrefix(part, "env:"):
			variable.Name = strings.TrimPrefix(part, "env:")
			hasEnv = true
		case part == "required":
			variable.Required = true
		}
	}
	if !hasEnv {
		return Var{}, false
	}
	variable.Default = tag.Get("default")
	if variable.Default != "" {
		variable.Required = false
	}
	return variable, true
}

// fieldTag returns the field's struct tag, or an empty tag when it has none or it is malformed.
func fieldTag(field *ast.Field) reflect.StructTag {
	if field.Tag == nil {
		return ""
	}
	unquoted, err := strconv.Unquote(field.Tag.Value)
	if err != nil {
		return ""
	}
	return reflect.StructTag(unquoted)
}

// typeName is the name an embedded field takes from its type expression.
func typeName(expression ast.Expr) string {
	switch typed := expression.(type) {
	case *ast.Ident:
		return typed.Name
	case *ast.StarExpr:
		return typeName(typed.X)
	case *ast.SelectorExpr:
		return typed.Sel.Name
	case *ast.IndexExpr:
		return typeName(typed.X)
	case *ast.IndexListExpr:
		return typeName(typed.X)
	}
	return ""
}

// joinPath joins a field path prefix and a field name with a dot.
func joinPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}
