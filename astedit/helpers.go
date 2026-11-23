package astedit

import (
	"bytes"
	"fmt"
	"go/parser"
	"go/scanner"
	"go/token"
	"strings"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
)

// parseSnippetFile parses a scratch file built around a caller snippet.
func parseSnippetFile(source string) (*dst.File, error) {
	file, err := decorator.ParseFile(token.NewFileSet(), "snippet.go", source, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadSource, err)
	}
	return file, nil
}

// parseElement parses caseSrc as the single element of a composite literal.
func parseElement(caseSrc string) (dst.Expr, error) {
	file, err := parseSnippetFile("package p\n\nvar _ = T{\n" + withTrailingComma(caseSrc) + "\n}\n")
	if err != nil {
		return nil, err
	}
	literal, ok := singleValue(file).(*dst.CompositeLit)
	if !ok || len(literal.Elts) != 1 {
		return nil, fmt.Errorf("%w: %q is not exactly one composite literal element", ErrBadSource, caseSrc)
	}
	return literal.Elts[0], nil
}

// withTrailingComma puts a comma after the last token, before any trailing comment, so the element can sit on its own line.
func withTrailingComma(source string) string {
	fileSet := token.NewFileSet()
	content := []byte(source)
	var tokenScanner scanner.Scanner
	tokenScanner.Init(fileSet.AddFile("snippet.go", -1, len(content)), content, nil, scanner.ScanComments)
	lastEnd, lastToken := -1, token.ILLEGAL
	for {
		position, kind, literal := tokenScanner.Scan()
		if kind == token.EOF {
			break
		}
		// Comments and the semicolons the scanner inserts at line ends are not part of the element.
		if kind == token.COMMENT || (kind == token.SEMICOLON && literal == "\n") {
			continue
		}
		offset := fileSet.Position(position).Offset
		lastEnd, lastToken = offset+tokenLength(kind, literal), kind
	}
	if lastEnd < 0 || lastToken == token.COMMA {
		return source
	}
	return source[:lastEnd] + "," + source[lastEnd:]
}

// tokenLength is the source length of a scanned token.
func tokenLength(kind token.Token, literal string) int {
	if literal != "" {
		return len(literal)
	}
	return len(kind.String())
}

// parseField parses fieldSrc as exactly one struct field.
func parseField(fieldSrc string) (*dst.Field, error) {
	file, err := parseSnippetFile("package p\n\ntype T struct {\n" + fieldSrc + "\n}\n")
	if err != nil {
		return nil, err
	}
	declaration, ok := file.Decls[0].(*dst.GenDecl)
	if !ok || len(declaration.Specs) != 1 {
		return nil, fmt.Errorf("%w: %q", ErrBadSource, fieldSrc)
	}
	structType, ok := declaration.Specs[0].(*dst.TypeSpec).Type.(*dst.StructType)
	if !ok || len(structType.Fields.List) != 1 {
		return nil, fmt.Errorf("%w: %q is not exactly one struct field", ErrBadSource, fieldSrc)
	}
	return structType.Fields.List[0], nil
}

// parseExpression parses valueSrc as one Go expression.
func parseExpression(valueSrc string) (dst.Expr, error) {
	file, err := parseSnippetFile("package p\n\nvar _ = " + valueSrc + "\n")
	if err != nil {
		return nil, err
	}
	value := singleValue(file)
	if value == nil {
		return nil, fmt.Errorf("%w: %q is not one expression", ErrBadSource, valueSrc)
	}
	return value, nil
}

// singleValue returns the only value of a scratch file holding exactly one `var _ = x`.
func singleValue(file *dst.File) dst.Expr {
	if len(file.Decls) != 1 {
		return nil
	}
	declaration, ok := file.Decls[0].(*dst.GenDecl)
	if !ok || len(declaration.Specs) != 1 {
		return nil
	}
	spec, ok := declaration.Specs[0].(*dst.ValueSpec)
	if !ok || len(spec.Values) != 1 {
		return nil
	}
	return spec.Values[0]
}

// exprText prints an expression with all whitespace removed, for layout blind comparison.
func exprText(expression dst.Expr) string {
	scratch := &dst.File{
		Name: dst.NewIdent("p"),
		Decls: []dst.Decl{&dst.GenDecl{
			Tok:   token.VAR,
			Specs: []dst.Spec{&dst.ValueSpec{Names: []*dst.Ident{dst.NewIdent("_")}, Values: []dst.Expr{dst.Clone(expression).(dst.Expr)}}},
		}},
	}
	var printed bytes.Buffer
	if err := decorator.Fprint(&printed, scratch); err != nil {
		return ""
	}
	text := printed.String()
	if index := strings.Index(text, "var _ = "); index >= 0 {
		text = text[index+len("var _ = "):]
	}
	return strings.Join(strings.Fields(text), "")
}

// findTypeSpec returns the top level type spec named name and the declaration holding it.
func findTypeSpec(file *dst.File, name string) (*dst.GenDecl, *dst.TypeSpec) {
	for _, declaration := range file.Decls {
		general, ok := declaration.(*dst.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, spec := range general.Specs {
			typeSpec := spec.(*dst.TypeSpec)
			if typeSpec.Name.Name == name {
				return general, typeSpec
			}
		}
	}
	return nil, nil
}

// findStruct returns the struct type of the top level type name, or ErrNotFound.
func findStruct(file *dst.File, name string) (*dst.StructType, error) {
	_, typeSpec := findTypeSpec(file, name)
	if typeSpec == nil {
		return nil, fmt.Errorf("%w: type %s", ErrNotFound, name)
	}
	structType, ok := typeSpec.Type.(*dst.StructType)
	if !ok {
		return nil, fmt.Errorf("%w: type %s is not a struct", ErrNotFound, name)
	}
	return structType, nil
}

// fieldNames lists the names a field declares; an embedded field is named after its type.
func fieldNames(field *dst.Field) []string {
	if len(field.Names) > 0 {
		var names []string
		for _, name := range field.Names {
			names = append(names, name.Name)
		}
		return names
	}
	if name := typeBaseName(field.Type); name != "" {
		return []string{name}
	}
	return nil
}

// typeBaseName strips pointers, package qualifiers and type arguments: *pkg.Name[T] gives Name.
func typeBaseName(expression dst.Expr) string {
	switch typed := expression.(type) {
	case *dst.Ident:
		return typed.Name
	case *dst.StarExpr:
		return typeBaseName(typed.X)
	case *dst.SelectorExpr:
		return typed.Sel.Name
	case *dst.IndexExpr:
		return typeBaseName(typed.X)
	case *dst.IndexListExpr:
		return typeBaseName(typed.X)
	case *dst.ParenExpr:
		return typeBaseName(typed.X)
	}
	return ""
}

// hasName reports whether names contains name.
func hasName(names []string, name string) bool {
	for _, candidate := range names {
		if candidate == name {
			return true
		}
	}
	return false
}

// splitDoc separates decorations into the part kept as is and the trailing doc comment block.
func splitDoc(decorations dst.Decorations) (kept, doc []string) {
	start := len(decorations)
	for index := len(decorations) - 1; index >= 0; index-- {
		entry := decorations[index]
		if isComment(entry) {
			start = index
			continue
		}
		// A single newline after a block comment is a line break, not a blank line ending the doc.
		if entry == "\n" && index > 0 && strings.HasPrefix(decorations[index-1], "/*") && index+1 < len(decorations) && isComment(decorations[index+1]) {
			continue
		}
		break
	}
	return append([]string(nil), decorations[:start]...), append([]string(nil), decorations[start:]...)
}

// isComment reports whether a decoration entry is a comment rather than spacing.
func isComment(entry string) bool {
	return strings.HasPrefix(entry, "//") || strings.HasPrefix(entry, "/*")
}

// isDirective reports whether a comment is a //go: style directive, which is kept on doc changes.
func isDirective(entry string) bool {
	return strings.HasPrefix(entry, "//go:") || strings.HasPrefix(entry, "//line ")
}

// replaceDoc swaps the doc comment block in decorations for doc, keeping directives and earlier comments.
func replaceDoc(decorations *dst.Decorations, doc string) (changed bool) {
	kept, oldDoc := splitDoc(*decorations)
	var directives []string
	for _, entry := range oldDoc {
		if isDirective(entry) {
			directives = append(directives, entry)
		}
	}

	var newDoc []string
	if doc != "" {
		for _, line := range strings.Split(strings.TrimRight(doc, "\n"), "\n") {
			if line == "" {
				newDoc = append(newDoc, "//")
			} else {
				newDoc = append(newDoc, "// "+line)
			}
		}
	}
	newDoc = append(newDoc, directives...)

	result := append(kept, newDoc...)
	changed = strings.Join(result, "\x00") != strings.Join(*decorations, "\x00")
	decorations.Replace(result...)
	return changed
}
