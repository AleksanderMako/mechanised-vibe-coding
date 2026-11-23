package mergeinfo

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
)

// EnclosingDecl returns the top level declaration of a Go file that covers the
// given 1-based line, such as "func (c *Consumer) Close", "type Page", "var x",
// "const subCampaignGroupsPerPage" or "import"; "" when the line is between declarations.
func EnclosingDecl(src []byte, line int) (string, error) {
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, "", src, parser.ParseComments)
	if err != nil {
		return "", err
	}
	for _, declaration := range file.Decls {
		first, last := declarationLines(fileSet, declaration)
		if line >= first && line <= last {
			return declarationName(declaration), nil
		}
	}
	return "", nil
}

// declarationLines gives the first and last line of a declaration, counting its doc comment.
func declarationLines(fileSet *token.FileSet, declaration ast.Decl) (int, int) {
	start := declaration.Pos()
	var doc *ast.CommentGroup
	switch typed := declaration.(type) {
	case *ast.FuncDecl:
		doc = typed.Doc
	case *ast.GenDecl:
		doc = typed.Doc
	}
	if doc != nil {
		start = doc.Pos()
	}
	return fileSet.Position(start).Line, fileSet.Position(declaration.End()).Line
}

// declarationName renders a declaration the way a reader would refer to it.
func declarationName(declaration ast.Decl) string {
	switch typed := declaration.(type) {
	case *ast.FuncDecl:
		return funcName(typed)
	case *ast.GenDecl:
		return genDeclName(typed)
	}
	return ""
}

// funcName gives "func Name" or "func (r *T) Name" with the receiver as written.
func funcName(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return "func " + function.Name.Name
	}
	receiver := function.Recv.List[0]
	receiverText := types.ExprString(receiver.Type)
	if len(receiver.Names) > 0 {
		receiverText = receiver.Names[0].Name + " " + receiverText
	}
	return "func (" + receiverText + ") " + function.Name.Name
}

// genDeclName gives "import", or the keyword with the first spec's name and "..." when grouped with more specs.
func genDeclName(declaration *ast.GenDecl) string {
	if declaration.Tok == token.IMPORT {
		return "import"
	}
	keyword := declaration.Tok.String()
	if len(declaration.Specs) == 0 {
		return keyword
	}
	name := ""
	switch spec := declaration.Specs[0].(type) {
	case *ast.TypeSpec:
		name = spec.Name.Name
	case *ast.ValueSpec:
		if len(spec.Names) > 0 {
			name = spec.Names[0].Name
		}
	}
	if len(declaration.Specs) > 1 {
		name += "..."
	}
	return keyword + " " + name
}
