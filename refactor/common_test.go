package refactor_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/editset"
	"github.com/AleksanderMako/mechanised-vibe-coding/refactor"
	"github.com/AleksanderMako/mechanised-vibe-coding/symbols"
)

// operation runs one of the three refactorings against a fixture root.
type operation struct {
	name    string
	fixture string
	run     func(root string) ([]string, error)
}

// validOperations are requests that succeed on an intact fixture copy, run with opts.
func validOperations(opts refactor.Options) []operation {
	return []operation{
		{"Rename", "rename", func(root string) ([]string, error) {
			return refactor.Rename(context.Background(), root, sym("shapes", "Area"), "Surface", opts)
		}},
		{"Move", "move", func(root string) ([]string, error) {
			return refactor.Move(context.Background(), root, []symbols.Symbol{sym("store", "Pad")}, importPath("util"), "util.go", opts)
		}},
		{"Demethodize", "demethodize", func(root string) ([]string, error) {
			return refactor.Demethodize(context.Background(), root, sym("calc", "Adder.Sum"), "", opts)
		}},
	}
}

func TestRule1_RootWithoutGoModIsAnError(t *testing.T) {
	for _, op := range validOperations(refactor.Options{}) {
		t.Run(op.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, "main.go", "package main\n\nfunc main() {}\n")
			before := readTree(t, root)

			if _, err := op.run(root); err == nil {
				t.Fatal("want an error for a root without go.mod, got nil")
			}
			assertTreeUnchanged(t, root, before)
		})
	}
}

func TestRule1_UnknownSymbolWrapsErrNotFound(t *testing.T) {
	cases := []operation{
		{"Rename unknown name", "rename", func(root string) ([]string, error) {
			return refactor.Rename(context.Background(), root, sym("shapes", "Missing"), "Other", refactor.Options{})
		}},
		{"Rename unknown package", "rename", func(root string) ([]string, error) {
			return refactor.Rename(context.Background(), root, sym("nowhere", "Area"), "Other", refactor.Options{})
		}},
		{"Move unknown name", "move", func(root string) ([]string, error) {
			return refactor.Move(context.Background(), root, []symbols.Symbol{sym("store", "Missing")}, importPath("util"), "util.go", refactor.Options{})
		}},
		{"Demethodize unknown method", "demethodize", func(root string) ([]string, error) {
			return refactor.Demethodize(context.Background(), root, sym("calc", "Adder.Missing"), "", refactor.Options{})
		}},
	}
	for _, op := range cases {
		t.Run(op.name, func(t *testing.T) {
			root := copyFixture(t, op.fixture)
			before := readTree(t, root)

			_, err := op.run(root)

			assertErrorIs(t, err, symbols.ErrNotFound)
			assertTreeUnchanged(t, root, before)
		})
	}
}

func TestRule1_TypeErrorInModuleWritesNothing(t *testing.T) {
	// Stage 9 made failing on any broken package the Strict behaviour.
	for _, op := range validOperations(refactor.Options{Strict: true}) {
		t.Run(op.name, func(t *testing.T) {
			root := copyFixture(t, op.fixture)
			writeFile(t, root, "broken/broken.go", "package broken\n\nvar count int = \"not a number\"\n")
			before := readTree(t, root)

			_, err := op.run(root)

			if err == nil {
				t.Fatal("want an error for a module with a type error, got nil")
			}
			assertNotBuildError(t, err)
			assertTreeUnchanged(t, root, before)
		})
	}
}

// Moving Format into audit makes audit import store for Label while store imports audit for Format.
// The static checks cannot see that import cycle, so only the build fails.
func moveFormatIntoAudit(root string, opts refactor.Options) ([]string, error) {
	return refactor.Move(context.Background(), root, []symbols.Symbol{sym("store", "Format")}, importPath("audit"), "audit.go", opts)
}

// cycleChangedFiles are the files the Format move into audit rewrites.
var cycleChangedFiles = []string{"audit/audit.go", "consumer/consumer.go", "report/report.go", "store/store.go", "store/store_test.go"}

func TestRule2And3_BuildFailureRestoresFilesAndStillReportsThem(t *testing.T) {
	root := copyFixture(t, "move")
	before := readTree(t, root)

	written, err := moveFormatIntoAudit(root, refactor.Options{})

	var buildErr *editset.BuildError
	if !errors.As(err, &buildErr) {
		t.Fatalf("want *editset.BuildError, got %T: %v", err, err)
	}
	assertTreeUnchanged(t, root, before)
	assertReturnedPathsShape(t, written)
	if want := absolutePaths(root, cycleChangedFiles); !slices.Equal(written, want) {
		t.Errorf("returned files after a failed build: want %v, got %v", want, written)
	}
}

func TestRule2And16_NoBuildKeepsChangesThatDoNotBuild(t *testing.T) {
	root := copyFixture(t, "move")
	before := readTree(t, root)

	written, err := moveFormatIntoAudit(root, refactor.Options{NoBuild: true})
	if err != nil {
		t.Fatalf("Move with NoBuild: %v", err)
	}

	// The import cycle is the caller's risk with NoBuild, so the module is not built here.
	assertWritten(t, root, before, written, cycleChangedFiles, nil)
	assertContains(t, root, "audit/audit.go", "func Format(label")
	assertContains(t, root, "store/store.go", "return audit.Format(Label(title))")
}
