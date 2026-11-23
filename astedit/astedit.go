// Package astedit edits Go files by symbol on a comment preserving syntax tree,
// so an edit cannot land in the wrong declaration and anchors never go stale.
package astedit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
)

// Op is one structural edit on one Go file. Only this package implements it.
type Op interface {
	// File is the path the op edits, as given to the constructor.
	File() string
	// Describe is a one line human description, for logs.
	Describe() string
	apply(f *dst.File) error
}

// Sentinel errors; errors from Apply wrap one of these and name the op.
var (
	ErrNotFound  = errors.New("target not found")
	ErrAmbiguous = errors.New("more than one target matches")
	ErrExists    = errors.New("target already exists")
	ErrBadSource = errors.New("source snippet does not parse")
)

// errNoChange is returned by an op that found its edit already in place.
var errNoChange = errors.New("no change needed")

// AddTableCase appends caseSrc as a new element of the table in testFunc.
func AddTableCase(file, testFunc, caseSrc string) Op {
	return &addTableCaseOp{file: file, testFunc: testFunc, caseSrc: caseSrc}
}

// AddStructField inserts fieldSrc into struct typeName, after the named field or at the end.
func AddStructField(file, typeName, fieldSrc, after string) Op {
	return &addStructFieldOp{file: file, typeName: typeName, fieldSrc: fieldSrc, after: after}
}

// RemoveStructField removes fieldName from struct typeName.
func RemoveStructField(file, typeName, fieldName string) Op {
	return &removeStructFieldOp{file: file, typeName: typeName, fieldName: fieldName}
}

// SetDoc replaces the doc comment of symbol ("Name" or "Type.Method") with doc.
func SetDoc(file, symbol, doc string) Op {
	return &setDocOp{file: file, symbol: symbol, doc: doc}
}

// SetConst replaces the value of constant name with valueSrc and, when doc is set, its doc comment.
func SetConst(file, name, valueSrc, doc string) Op {
	return &setConstOp{file: file, name: name, valueSrc: valueSrc, doc: doc}
}

// AddConformance adds `var _ Iface = (*TypeName)(nil)` after the type declaration.
func AddConformance(file, typeName, ifaceSrc string) Op {
	return &addConformanceOp{file: file, typeName: typeName, ifaceSrc: ifaceSrc}
}

// fileOps is one file with the ops that target it, in the order they were given.
type fileOps struct {
	path string
	ops  []Op
}

// Apply applies all ops in memory first and then writes every changed file formatted.
func Apply(ctx context.Context, root string, ops ...Op) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	groups := groupByFile(root, ops)

	// Every op runs before any write so a failure leaves the tree untouched.
	type pendingWrite struct {
		path    string
		content []byte
	}
	var writes []pendingWrite
	for _, group := range groups {
		content, changed, err := editFile(group)
		if err != nil {
			return nil, err
		}
		if changed {
			writes = append(writes, pendingWrite{path: group.path, content: content})
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var written []string
	for _, write := range writes {
		if err := writeKeepingMode(write.path, write.content); err != nil {
			return written, fmt.Errorf("astedit: write %s: %w", write.path, err)
		}
		written = append(written, write.path)
	}
	return written, nil
}

// groupByFile resolves each op's path against root and keeps first-op order of files.
func groupByFile(root string, ops []Op) []*fileOps {
	var groups []*fileOps
	byPath := map[string]*fileOps{}
	for _, op := range ops {
		path := op.File()
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		path = filepath.Clean(path)
		// An absolute root keeps returned paths absolute even when root was given relative.
		if absolute, err := filepath.Abs(path); err == nil {
			path = absolute
		}
		group, ok := byPath[path]
		if !ok {
			group = &fileOps{path: path}
			byPath[path] = group
			groups = append(groups, group)
		}
		group.ops = append(group.ops, op)
	}
	return groups
}

// editFile parses one file, runs its ops and returns the formatted result when anything changed.
func editFile(group *fileOps) ([]byte, bool, error) {
	source, err := os.ReadFile(group.path)
	if err != nil {
		return nil, false, fmt.Errorf("astedit: read %s: %w", group.path, err)
	}
	file, err := decorator.ParseFile(token.NewFileSet(), group.path, source, parser.ParseComments)
	if err != nil {
		return nil, false, fmt.Errorf("astedit: parse %s: %w", group.path, err)
	}

	changed := false
	for _, op := range group.ops {
		err := op.apply(file)
		if errors.Is(err, errNoChange) {
			continue
		}
		if err != nil {
			return nil, false, fmt.Errorf("astedit: %s: %w", op.Describe(), err)
		}
		changed = true
	}
	if !changed {
		return nil, false, nil
	}

	var printed bytes.Buffer
	if err := decorator.Fprint(&printed, file); err != nil {
		return nil, false, fmt.Errorf("astedit: print %s: %w", group.path, err)
	}
	formatted, err := format.Source(printed.Bytes())
	if err != nil {
		return nil, false, fmt.Errorf("astedit: format %s: %w", group.path, err)
	}
	// Exactly one trailing newline, whatever the printer produced.
	formatted = append(bytes.TrimRight(formatted, "\n"), '\n')
	return formatted, true, nil
}

// writeKeepingMode overwrites an existing file; os.WriteFile keeps the mode of an existing file.
func writeKeepingMode(path string, content []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, content, info.Mode().Perm())
}
