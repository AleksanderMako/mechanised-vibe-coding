package deadcode_test

import (
	"bytes"
	"errors"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/AleksanderMako/machanised-vibe-coding/deadcode"
)

// referenceBinaryVariable names the environment variable holding the path of a golang.org/x/tools/cmd/deadcode binary.
const referenceBinaryVariable = "MVC_DEADCODE_REFERENCE"

// referenceLine matches one line of the reference output: "dir/file.go:LINE:COL: unreachable func: Name".
var referenceLine = regexp.MustCompile(`^(.+\.go):(\d+):(\d+): unreachable func: (.+)$`)

// oracleCase is one fixture module, optionally changed before analysis, analysed with one set of Options.
type oracleCase struct {
	label   string
	fixture string
	prepare func(t *testing.T, root string)
	options deadcode.Options
}

// oracleCases covers every fixture module with each Options it is analysed with elsewhere in the suite.
var oracleCases = []oracleCase{
	{label: "module", fixture: "module"},
	{label: "module with tests", fixture: "module", options: deadcode.Options{Tests: true}},
	{label: "libonly", fixture: "libonly"},
	{label: "libonly with tests", fixture: "libonly", options: deadcode.Options{Tests: true}},
	{label: "libonly with a test file", fixture: "libonly", options: deadcode.Options{Tests: true}, prepare: addLibonlyTest},
	{label: "initroots", fixture: "initroots"},
	{label: "markers", fixture: "markers"},
	{label: "markers with tests", fixture: "markers", options: deadcode.Options{Tests: true}},
}

// addLibonlyTest gives the libonly fixture a test so the generated test main becomes a root.
func addLibonlyTest(t *testing.T, root string) {
	writeFile(t, root, libonlyTestFile, libonlyTestContent)
}

func TestDeadMatchesTheReferenceTool(t *testing.T) {
	referenceBinary := os.Getenv(referenceBinaryVariable)
	if referenceBinary == "" {
		t.Skipf("set %s to the path of a golang.org/x/tools/cmd/deadcode v0.53.0 binary to compare with it (rule 13)", referenceBinaryVariable)
	}
	for _, testCase := range oracleCases {
		t.Run(testCase.label, func(t *testing.T) {
			root := copyFixture(t, testCase.fixture)
			if testCase.prepare != nil {
				testCase.prepare(t, root)
			}
			want, hasRoots := runReference(t, referenceBinary, root, testCase.options.Tests)
			got, err := deadcode.Dead(t.Context(), root, testCase.options)
			if !hasRoots {
				// The reference refuses a module without main packages; Dead reports that as ErrNoRoots (rule 3).
				if !errors.Is(err, deadcode.ErrNoRoots) {
					t.Fatalf("the reference finds no main packages, so Dead must wrap ErrNoRoots, got %v and%s", err, describeFuncs(got))
				}
				return
			}
			if err != nil {
				t.Fatalf("Dead: %v", err)
			}
			assertSameFuncSet(t, got, want)
		})
	}
}

// runReference runs the reference binary in root and returns its dead functions, or false when it found no main packages.
func runReference(t *testing.T, referenceBinary, root string, tests bool) ([]deadcode.Func, bool) {
	t.Helper()
	arguments := []string{"-generated"}
	if tests {
		arguments = append(arguments, "-test")
	}
	arguments = append(arguments, "./...")
	command := exec.Command(referenceBinary, arguments...)
	command.Dir = root
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if strings.Contains(stderr.String(), "no main packages") {
			return nil, false
		}
		t.Fatalf("reference %s: %v\n%s%s", strings.Join(arguments, " "), err, stdout.String(), stderr.String())
	}
	modulePath := readModulePath(t, root)
	var funcs []deadcode.Func
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if line == "" {
			continue
		}
		funcs = append(funcs, parseReferenceLine(t, root, modulePath, line))
	}
	return funcs, true
}

// parseReferenceLine turns one reference line into a Func, keeping the reference's spelling of the name.
func parseReferenceLine(t *testing.T, root, modulePath, line string) deadcode.Func {
	t.Helper()
	parts := referenceLine.FindStringSubmatch(line)
	if parts == nil {
		t.Fatalf("cannot parse reference output line %q", line)
	}
	file := filepath.ToSlash(parts[1])
	lineNumber, err := strconv.Atoi(parts[2])
	if err != nil {
		t.Fatalf("line number in %q: %v", line, err)
	}
	return deadcode.Func{
		Package: importPathOf(t, root, modulePath, file),
		Name:    parts[4],
		File:    file,
		Line:    lineNumber,
	}
}

// importPathOf maps a file's directory to its import path under modulePath, adding _test for an external test package.
func importPathOf(t *testing.T, root, modulePath, file string) string {
	t.Helper()
	importPath := modulePath
	if directory := path.Dir(file); directory != "." {
		importPath = modulePath + "/" + directory
	}
	// The reference reports a dead function of "package p_test" under the p_test path, so mirror that.
	parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, filepath.FromSlash(file)), nil, parser.PackageClauseOnly)
	if err != nil {
		t.Fatalf("read package clause of %s: %v", file, err)
	}
	if strings.HasSuffix(parsed.Name.Name, "_test") {
		importPath += "_test"
	}
	return importPath
}

// readModulePath returns the module path declared in root/go.mod.
func readModulePath(t *testing.T, root string) string {
	t.Helper()
	for _, line := range strings.Split(readFile(t, root, "go.mod"), "\n") {
		if modulePath, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.Trim(strings.TrimSpace(modulePath), `"`)
		}
	}
	t.Fatalf("no module line in %s/go.mod", root)
	return ""
}

// assertSameFuncSet compares got and want as sets of Package, Name, File and Line, showing what is missing on each side.
func assertSameFuncSet(t *testing.T, got, want []deadcode.Func) {
	t.Helper()
	missingFromDead := subtractFuncs(want, got)
	missingFromReference := subtractFuncs(got, want)
	if len(missingFromDead) == 0 && len(missingFromReference) == 0 {
		return
	}
	t.Errorf("Dead differs from the reference tool\nlisted by the reference but not by Dead:%s\nlisted by Dead but not by the reference:%s",
		describeFuncs(missingFromDead), describeFuncs(missingFromReference))
}

// subtractFuncs returns the funcs of from that are not in remove, sorted like Dead.
func subtractFuncs(from, remove []deadcode.Func) []deadcode.Func {
	removed := make(map[deadcode.Func]bool, len(remove))
	for _, function := range remove {
		removed[function] = true
	}
	var left []deadcode.Func
	for _, function := range from {
		if !removed[function] {
			left = append(left, function)
		}
	}
	return sortLikeDead(left)
}
