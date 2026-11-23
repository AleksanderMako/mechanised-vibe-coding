package refactor

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/tools/go/ast/astutil"
	"golang.org/x/tools/go/packages"
)

// textEdit replaces the bytes [start, end) of a file with text.
type textEdit struct {
	start int
	end   int
	text  string
}

// editKey identifies an edit so the copies test variants produce are applied once.
type editKey struct {
	file  string
	start int
	end   int
}

// importSpec is an import a rewritten file needs; name is empty for the default name.
type importSpec struct {
	name string
	path string
}

// editSet collects byte edits and needed imports per absolute file path.
type editSet struct {
	fileSet *token.FileSet
	edits   map[string][]textEdit
	seen    map[editKey]bool
	imports map[string][]importSpec
}

func newEditSet(fileSet *token.FileSet) *editSet {
	return &editSet{
		fileSet: fileSet,
		edits:   map[string][]textEdit{},
		seen:    map[editKey]bool{},
		imports: map[string][]importSpec{},
	}
}

// replace records that the source between start and end becomes text.
func (s *editSet) replace(start, end token.Pos, text string) {
	from := s.fileSet.Position(start)
	to := s.fileSet.Position(end)
	s.replaceOffsets(from.Filename, from.Offset, to.Offset, text)
}

// replaceOffsets records an edit by byte offsets, ignoring an exact repeat.
func (s *editSet) replaceOffsets(file string, start, end int, text string) {
	key := editKey{file: file, start: start, end: end}
	if s.seen[key] {
		return
	}
	s.seen[key] = true
	s.edits[file] = append(s.edits[file], textEdit{start: start, end: end, text: text})
}

// addImport records that file needs importPath under name ("" for the default name).
func (s *editSet) addImport(file string, spec importSpec) {
	if slices.Contains(s.imports[file], spec) {
		return
	}
	s.imports[file] = append(s.imports[file], spec)
}

// files returns every file with an edit or a needed import, sorted.
func (s *editSet) files() []string {
	var result []string
	for file := range s.edits {
		result = append(result, file)
	}
	for file := range s.imports {
		if _, hasEdits := s.edits[file]; !hasEdits {
			result = append(result, file)
		}
	}
	slices.Sort(result)
	return result
}

// applyEdits applies edits to content from the end backwards; overlapping edits are a bug.
func applyEdits(file string, content []byte, edits []textEdit) ([]byte, error) {
	sorted := slices.Clone(edits)
	slices.SortFunc(sorted, func(a, b textEdit) int {
		if a.start != b.start {
			return a.start - b.start
		}
		return a.end - b.end
	})
	for index := 1; index < len(sorted); index++ {
		if sorted[index].start < sorted[index-1].end {
			return nil, fmt.Errorf("refactor: overlapping edits in %s at offset %d", file, sorted[index].start)
		}
	}
	result := bytes.Clone(content)
	for index := len(sorted) - 1; index >= 0; index-- {
		edit := sorted[index]
		if edit.start < 0 || edit.end > len(result) || edit.start > edit.end {
			return nil, fmt.Errorf("refactor: edit outside %s", file)
		}
		result = slices.Concat(result[:edit.start], []byte(edit.text), result[edit.end:])
	}
	return result, nil
}

// editsWithin returns the edits that lie inside [start, end), shifted to start at zero.
func editsWithin(edits []textEdit, start, end int) []textEdit {
	var result []textEdit
	for _, edit := range edits {
		if edit.start >= start && edit.end <= end {
			result = append(result, textEdit{start: edit.start - start, end: edit.end - start, text: edit.text})
		}
	}
	return result
}

// addImports inserts the import specs into Go source and reformats it.
func addImports(file string, content []byte, specs []importSpec) ([]byte, error) {
	if len(specs) == 0 {
		return content, nil
	}
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, file, content, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("refactor: parse rewritten %s: %w", file, err)
	}
	for _, spec := range specs {
		astutil.AddNamedImport(fileSet, parsed, spec.name, spec.path)
	}
	var buffer bytes.Buffer
	if err := format.Node(&buffer, fileSet, parsed); err != nil {
		return nil, fmt.Errorf("refactor: format %s: %w", file, err)
	}
	return buffer.Bytes(), nil
}

// buildChanges turns the edit set into file changes on the current file contents.
func buildChanges(edits *editSet) ([]*fileChange, error) {
	var changes []*fileChange
	for _, file := range edits.files() {
		change, err := readChange(file)
		if err != nil {
			return nil, err
		}
		content, err := applyEdits(file, change.original, edits.edits[file])
		if err != nil {
			return nil, err
		}
		content, err = addImports(file, content, edits.imports[file])
		if err != nil {
			return nil, err
		}
		change.content = content
		changes = append(changes, change)
	}
	return changes, nil
}

// readChange starts a change for an existing file by reading its bytes and mode.
func readChange(file string) (*fileChange, error) {
	info, err := os.Stat(file)
	if err != nil {
		return nil, fmt.Errorf("refactor: %w", err)
	}
	original, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("refactor: %w", err)
	}
	return &fileChange{path: file, existed: true, original: original, mode: info.Mode().Perm()}, nil
}

// isValidName reports whether name can be declared: an identifier that is not blank.
func isValidName(name string) bool {
	return token.IsIdentifier(name) && name != "_"
}

// exportedForm upper cases the first letter of name.
func exportedForm(name string) string {
	first, size := utf8.DecodeRuneInString(name)
	return string(unicode.ToUpper(first)) + name[size:]
}

// pathTo returns the nodes enclosing ident, innermost first.
func pathTo(file *ast.File, ident *ast.Ident) ([]ast.Node, bool) {
	return astutil.PathEnclosingInterval(file, ident.Pos(), ident.End())
}

// docFirstWordEdit rewrites the first word of doc when it equals oldName.
func docFirstWordEdit(edits *editSet, doc *ast.CommentGroup, oldName, newName string) {
	if doc == nil || len(doc.List) == 0 {
		return
	}
	comment := doc.List[0]
	text := comment.Text
	index := 2 // skip the comment opener, // or /*
	for index < len(text) && strings.ContainsRune(" \t\r\n*", rune(text[index])) {
		index++
	}
	end := index
	for end < len(text) {
		r, size := utf8.DecodeRuneInString(text[end:])
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			break
		}
		end += size
	}
	if text[index:end] != oldName {
		return
	}
	start := comment.Pos() + token.Pos(index)
	edits.replace(start, start+token.Pos(len(oldName)), newName)
}

// docOf returns the doc comments of the declaration enclosing a declaring identifier.
func docOf(path []ast.Node) []*ast.CommentGroup {
	var docs []*ast.CommentGroup
	for index, node := range path {
		switch typed := node.(type) {
		case *ast.FuncDecl:
			return append(docs, typed.Doc)
		case *ast.Field:
			return append(docs, typed.Doc)
		case *ast.TypeSpec:
			docs = append(docs, typed.Doc)
			return append(docs, groupDoc(path, index)...)
		case *ast.ValueSpec:
			docs = append(docs, typed.Doc)
			return append(docs, groupDoc(path, index)...)
		}
	}
	return docs
}

// groupDoc returns the doc of the GenDecl above path[index] when it declares a single spec.
func groupDoc(path []ast.Node, index int) []*ast.CommentGroup {
	if index+1 < len(path) {
		if group, isGroup := path[index+1].(*ast.GenDecl); isGroup && len(group.Specs) == 1 {
			return []*ast.CommentGroup{group.Doc}
		}
	}
	return nil
}

// importNameIn returns the name file uses for importPath; found is false when it is not imported.
// A dot import yields the empty name, meaning no qualifier is needed.
func importNameIn(pkg *packages.Package, file *ast.File, importPath string) (name string, found bool) {
	for _, spec := range file.Imports {
		if strings.Trim(spec.Path.Value, "`\"") != importPath {
			continue
		}
		if spec.Name != nil {
			switch spec.Name.Name {
			case "_":
				continue
			case ".":
				return "", true
			}
			return spec.Name.Name, true
		}
		if pkgName, isPkgName := pkg.TypesInfo.Implicits[spec].(*types.PkgName); isPkgName {
			return pkgName.Name(), true
		}
		return path.Base(importPath), true
	}
	return "", false
}

// qualifierFor returns "name." for importPath in file, recording the import when it is missing.
func qualifierFor(edits *editSet, pkg *packages.Package, file *ast.File, importPath, packageName string) string {
	name, found := importNameIn(pkg, file, importPath)
	if found {
		if name == "" {
			return ""
		}
		return name + "."
	}
	fileName := edits.fileSet.Position(file.Pos()).Filename
	spec := importSpec{path: importPath}
	// A package whose name differs from its directory needs an explicit name to be found by readers and goimports.
	if packageName != path.Base(importPath) {
		spec.name = packageName
	}
	edits.addImport(fileName, spec)
	return packageName + "."
}

// parentMaps indexes the selector owning each Sel identifier and the call owning each Fun expression.
type parentMaps struct {
	selectorOf map[*ast.Ident]*ast.SelectorExpr
	callOf     map[ast.Expr]*ast.CallExpr
	isKey      map[*ast.Ident]bool // identifier is a key in a composite literal
}

func buildParentMaps(file *ast.File) *parentMaps {
	maps := &parentMaps{
		selectorOf: map[*ast.Ident]*ast.SelectorExpr{},
		callOf:     map[ast.Expr]*ast.CallExpr{},
		isKey:      map[*ast.Ident]bool{},
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.SelectorExpr:
			maps.selectorOf[typed.Sel] = typed
		case *ast.CallExpr:
			maps.callOf[typed.Fun] = typed
		case *ast.CompositeLit:
			for _, element := range typed.Elts {
				if pair, isPair := element.(*ast.KeyValueExpr); isPair {
					if key, isIdent := pair.Key.(*ast.Ident); isIdent {
						maps.isKey[key] = true
					}
				}
			}
		}
		return true
	})
	return maps
}

// offsetOf returns the byte offset of pos in its file.
func offsetOf(fileSet *token.FileSet, pos token.Pos) int {
	return fileSet.Position(pos).Offset
}

// fileNameOf returns the absolute file name holding pos.
func fileNameOf(fileSet *token.FileSet, pos token.Pos) string {
	return fileSet.Position(pos).Filename
}
