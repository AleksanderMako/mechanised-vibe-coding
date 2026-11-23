package main_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/deadcode"
)

// mainCalling returns cmd/app/main.go calling only the given fixture packages, each exporting a capitalised function.
func mainCalling(packages ...string) string {
	var imports, calls []string
	for _, name := range packages {
		imports = append(imports, "\t\"example.com/fixture/"+name+"\"\n")
		calls = append(calls, name+"."+strings.ToUpper(name[:1])+name[1:]+"()")
	}
	return "// Command app is the only main package; everything it reaches is live.\npackage main\n\nimport (\n" +
		strings.Join(imports, "") + ")\n\nfunc main() {\n\tprintln(" + strings.Join(calls, " + ") + ")\n}\n"
}

// funcNames returns the Package.Name keys of functions.
func funcNames(functions []deadcode.Func) []string {
	var names []string
	for _, function := range functions {
		names = append(names, function.Key())
	}
	return names
}

// newHistoryRepo commits the module with main calling all three packages, then with gamma dropped,
// and leaves beta dropped too as an uncommitted change in the working tree.
func newHistoryRepo(t *testing.T) string {
	t.Helper()
	repo := copyModule(t)
	runGit(t, repo, "init", "-q", "-b", "main")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-q", "-m", "all packages live")
	writeFile(t, repo, "cmd/app/main.go", mainCalling("alpha", "beta"))
	runGit(t, repo, "commit", "-q", "-am", "drop gamma")
	writeFile(t, repo, "cmd/app/main.go", mainCalling("alpha"))
	return repo
}

// Rules 3 and 4: dead functions found exit 1; -json decodes into []deadcode.Func.
func TestDeadListJSON(t *testing.T) {
	module := copyModule(t)
	result := runMVC(t, module, "dead", "list", "-root", module, "-json")
	expectExit(t, result, 1)
	var functions []deadcode.Func
	decodeJSON(t, result, &functions)
	if !slices.Contains(funcNames(functions), "example.com/fixture/shapes.Abandoned") {
		t.Errorf("dead = %v, want shapes.Abandoned among them", funcNames(functions))
	}
}

// Rule 5: dead list text prints one line per function.
func TestDeadListText(t *testing.T) {
	module := copyModule(t)
	result := runMVC(t, module, "dead", "list", "-root", module)
	expectExit(t, result, 1)
	if !strings.Contains(result.stdout, "Abandoned") {
		t.Errorf("Abandoned not printed:\n%s", result.describe())
	}
}

// Rule 10: one positional analyses the working tree as AFTER, including uncommitted changes.
func TestDeadOrphanedAgainstWorkingTree(t *testing.T) {
	repo := newHistoryRepo(t)
	result := runMVC(t, repo, "dead", "orphaned", "-root", repo, "-json", "HEAD~1")
	expectExit(t, result, 1)
	var diff deadcode.Diff
	decodeJSON(t, result, &diff)
	newlyDead := funcNames(diff.NewlyDead)
	for _, want := range []string{"example.com/fixture/beta.Beta", "example.com/fixture/gamma.Gamma"} {
		if !slices.Contains(newlyDead, want) {
			t.Errorf("newly dead = %v, want %s among them", newlyDead, want)
		}
	}
}

// Rule 10: two positionals compare the two refs and ignore the working tree.
func TestDeadOrphanedBetweenTwoRefs(t *testing.T) {
	repo := newHistoryRepo(t)
	result := runMVC(t, repo, "dead", "orphaned", "-root", repo, "-json", "HEAD~1", "HEAD")
	expectExit(t, result, 1)
	var diff deadcode.Diff
	decodeJSON(t, result, &diff)
	newlyDead := funcNames(diff.NewlyDead)
	if !slices.Contains(newlyDead, "example.com/fixture/gamma.Gamma") {
		t.Errorf("newly dead = %v, want gamma.Gamma", newlyDead)
	}
	if slices.Contains(newlyDead, "example.com/fixture/beta.Beta") {
		t.Errorf("newly dead = %v, beta.Beta is only dead in the uncommitted working tree", newlyDead)
	}
}

// Rule 3: an invalid ref is an ordinary error.
func TestDeadOrphanedInvalidRefIsError(t *testing.T) {
	repo := newHistoryRepo(t)
	expectOrdinaryError(t, runMVC(t, repo, "dead", "orphaned", "-root", repo, "no-such-ref"))
}

// Rule 1: dead list defaults -root to the current directory.
func TestDeadListDefaultRoot(t *testing.T) {
	module := copyModule(t)
	result := runMVC(t, module, "dead", "list", "-json")
	expectExit(t, result, 1)
	var functions []deadcode.Func
	decodeJSON(t, result, &functions)
	if len(functions) == 0 {
		t.Error("no dead functions found from the current directory")
	}
}
