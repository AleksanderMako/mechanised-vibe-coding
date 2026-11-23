package astedit_test

import (
	"bytes"
	"context"
	"errors"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AleksanderMako/machanised-vibe-coding/astedit"
)

// copyFixture copies testdata/<name> into a fresh temp dir and returns its symlink free absolute path.
func copyFixture(t *testing.T, name string) string {
	t.Helper()
	source := filepath.Join("testdata", name)
	target := filepath.Join(t.TempDir(), name)
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, content, 0o644)
	})
	if err != nil {
		t.Fatalf("copy fixture %s: %v", name, err)
	}
	// Temp dirs on macOS sit behind a symlink; resolving it keeps path comparisons stable.
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		t.Fatalf("resolve fixture path: %v", err)
	}
	return resolved
}

// writeFile creates a file for a test that needs content the fixture cannot hold (unformatted or broken Go).
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// readFile returns the content of path or fails the test.
func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

// pinnedTime is an old modification time; a file still carrying it was not written.
var pinnedTime = time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)

// untouchedFile remembers a file's bytes after pinning its modification time.
type untouchedFile struct {
	path    string
	content string
}

// pinFiles sets an old modification time on each file so a later write, even of equal bytes, is visible.
func pinFiles(t *testing.T, paths ...string) []untouchedFile {
	t.Helper()
	var pinned []untouchedFile
	for _, path := range paths {
		if err := os.Chtimes(path, pinnedTime, pinnedTime); err != nil {
			t.Fatal(err)
		}
		pinned = append(pinned, untouchedFile{path: path, content: readFile(t, path)})
	}
	return pinned
}

// assertNotWritten fails when a pinned file changed content or was rewritten.
func assertNotWritten(t *testing.T, pinned []untouchedFile) {
	t.Helper()
	for _, file := range pinned {
		if got := readFile(t, file.path); got != file.content {
			t.Errorf("%s changed:\nbefore:\n%s\nafter:\n%s", file.path, file.content, got)
		}
		info, err := os.Stat(file.path)
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().Equal(pinnedTime) {
			t.Errorf("%s was written (modification time moved)", file.path)
		}
	}
}

// applyOK runs Apply, fails on error and checks every written file is well formed.
func applyOK(t *testing.T, root string, ops ...astedit.Op) []string {
	t.Helper()
	written, err := astedit.Apply(context.Background(), root, ops...)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	for _, path := range written {
		assertWellFormed(t, path)
	}
	return written
}

// assertWellFormed checks rule 4 (exactly one trailing newline) and that the file is gofmt output.
func assertWellFormed(t *testing.T, path string) {
	t.Helper()
	content := readFile(t, path)
	if !strings.HasSuffix(content, "\n") || strings.HasSuffix(content, "\n\n") {
		t.Errorf("%s must end with exactly one newline, ends with %q", path, content[max(0, len(content)-5):])
	}
	formatted, err := format.Source([]byte(content))
	if err != nil {
		t.Errorf("%s does not parse after the edit: %v\n%s", path, err, content)
		return
	}
	if !bytes.Equal(formatted, []byte(content)) {
		t.Errorf("%s is not gofmt formatted:\n%s", path, content)
	}
}

// assertOpError checks rule 3: the error wraps sentinel and contains the op's description.
func assertOpError(t *testing.T, err error, sentinel error, op astedit.Op) {
	t.Helper()
	if err == nil {
		t.Fatalf("want error wrapping %v, got nil", sentinel)
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("want error wrapping %q, got %v", sentinel, err)
	}
	if !strings.Contains(err.Error(), op.Describe()) {
		t.Errorf("error %q does not contain the op description %q", err, op.Describe())
	}
}

// expectFailure runs one op that must fail with sentinel and leave its file unwritten.
func expectFailure(t *testing.T, root string, op astedit.Op, sentinel error) {
	t.Helper()
	path := op.File()
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	pinned := pinFiles(t, path)
	written, err := astedit.Apply(context.Background(), root, op)
	assertOpError(t, err, sentinel, op)
	if len(written) != 0 {
		t.Errorf("want no files written on failure, got %v", written)
	}
	assertNotWritten(t, pinned)
}

// parsedFile is a Go file parsed with comments plus its source, for structural checks.
type parsedFile struct {
	fileSet *token.FileSet
	file    *ast.File
	source  string
}

// parseGo parses path with comments or fails the test.
func parseGo(t *testing.T, path string) parsedFile {
	t.Helper()
	source := readFile(t, path)
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, source, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v\n%s", path, err, source)
	}
	return parsedFile{fileSet: fileSet, file: file, source: source}
}

// text returns the exact source of node.
func (p parsedFile) text(node ast.Node) string {
	start := p.fileSet.Position(node.Pos()).Offset
	end := p.fileSet.Position(node.End()).Offset
	return p.source[start:end]
}

// line returns the line number where pos sits.
func (p parsedFile) line(pos token.Pos) int {
	return p.fileSet.Position(pos).Line
}

// commentLines returns the raw comment lines of a group, nil for no group.
func commentLines(group *ast.CommentGroup) []string {
	if group == nil {
		return nil
	}
	var lines []string
	for _, comment := range group.List {
		lines = append(lines, comment.Text)
	}
	return lines
}

// structType finds the top level struct named name, or nil.
func (p parsedFile) structType(name string) *ast.StructType {
	for _, decl := range p.file.Decls {
		general, ok := decl.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, spec := range general.Specs {
			typeSpec := spec.(*ast.TypeSpec)
			if typeSpec.Name.Name != name {
				continue
			}
			structure, _ := typeSpec.Type.(*ast.StructType)
			return structure
		}
	}
	return nil
}

// fieldNames lists a struct's fields; an embedded field is listed by its type text.
func (p parsedFile) fieldNames(structure *ast.StructType) []string {
	var names []string
	for _, field := range structure.Fields.List {
		if len(field.Names) == 0 {
			names = append(names, p.text(field.Type))
			continue
		}
		for _, name := range field.Names {
			names = append(names, name.Name)
		}
	}
	return names
}

// field returns the field named name in structure, or nil.
func field(structure *ast.StructType, name string) *ast.Field {
	for _, candidate := range structure.Fields.List {
		for _, ident := range candidate.Names {
			if ident.Name == name {
				return candidate
			}
		}
	}
	return nil
}

// function returns the top level func (or method when receiverType is set) named name, or nil.
func (p parsedFile) function(receiverType, name string) *ast.FuncDecl {
	for _, decl := range p.file.Decls {
		function, ok := decl.(*ast.FuncDecl)
		if !ok || function.Name.Name != name {
			continue
		}
		if receiverType == "" && function.Recv == nil {
			return function
		}
		if receiverType != "" && function.Recv != nil && strings.TrimPrefix(p.text(function.Recv.List[0].Type), "*") == receiverType {
			return function
		}
	}
	return nil
}

// genDeclOf returns the top level declaration group that declares name, and its spec.
func (p parsedFile) genDeclOf(name string) (*ast.GenDecl, ast.Spec) {
	for _, decl := range p.file.Decls {
		general, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range general.Specs {
			switch typed := spec.(type) {
			case *ast.TypeSpec:
				if typed.Name.Name == name {
					return general, spec
				}
			case *ast.ValueSpec:
				for _, ident := range typed.Names {
					if ident.Name == name {
						return general, spec
					}
				}
			}
		}
	}
	return nil, nil
}

// docOf returns the doc comment the contract assigns to symbol: the spec's in a group, else the declaration's.
func (p parsedFile) docOf(t *testing.T, symbol string) *ast.CommentGroup {
	t.Helper()
	if receiver, method, ok := strings.Cut(symbol, "."); ok {
		function := p.function(receiver, method)
		if function == nil {
			t.Fatalf("method %s not found", symbol)
		}
		return function.Doc
	}
	if function := p.function("", symbol); function != nil {
		return function.Doc
	}
	general, spec := p.genDeclOf(symbol)
	if general == nil {
		t.Fatalf("symbol %s not found", symbol)
	}
	if !general.Lparen.IsValid() {
		return general.Doc
	}
	switch typed := spec.(type) {
	case *ast.TypeSpec:
		return typed.Doc
	case *ast.ValueSpec:
		return typed.Doc
	}
	return nil
}
