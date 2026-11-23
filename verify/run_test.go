package verify_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/AleksanderMako/machanised-vibe-coding/verify"
)

// Source files the Run tests write into the temp module at run time, so the repo itself stays clean.
const (
	unformattedSource = "package calc\nfunc  Ugly( )   int {\nreturn 1}\n"
	brokenBuildSource = "package calc\n\n// Broken refers to a name that does not exist.\nfunc Broken() int { return missingValue }\n"
	vetFailureSource  = "package calc\n\nimport \"fmt\"\n\n// Describe formats a string with a number verb.\nfunc Describe() string { return fmt.Sprintf(\"%d\", \"not a number\") }\n"
	failingTestSource = "package greet\n\nimport \"testing\"\n\nfunc TestBroken(t *testing.T) {\n\tt.Fatal(\"greet-failure-marker\")\n}\n"
	racyTestSource    = `package racy

import "testing"

func TestRacy(t *testing.T) {
	shared := 0
	done := make(chan struct{})
	go func() {
		shared = 1
		close(done)
	}()
	shared = 2
	<-done
	_ = shared
}
`
	racyPackageSource = "// Package racy holds a test with a data race.\npackage racy\n"
	staleModulesTxt   = "# example.com/other v1.0.0\n## explicit; go 1.20\nexample.com/other\n"
)

// allButFormat skips every step after gofmt so format tests stay fast.
var allButFormat = []verify.StepName{verify.StepVendor, verify.StepBuild, verify.StepVet, verify.StepTest}

func TestRule1_RootMustBeAModuleDirectory(t *testing.T) {
	t.Parallel()
	moduleRoot := copyFixture(t, "module")
	emptyDir := t.TempDir()
	cases := map[string]string{
		"empty root":            "",
		"missing directory":     filepath.Join(emptyDir, "does-not-exist"),
		"directory without mod": emptyDir,
		"file instead of dir":   filepath.Join(moduleRoot, "go.mod"),
	}
	for name, root := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := verify.Run(context.Background(), verify.Options{Root: root, Packages: []string{"./..."}})
			if err == nil {
				t.Errorf("Run with Root %q: want an error, got nil", root)
			}
		})
	}
}

func TestRule2_EmptyPackagesUsesChangedPackages(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	commitAll(t, root)
	writeFile(t, root, "lib/greet/greet.go", "// Package greet builds greetings.\npackage greet\n\n// Hello greets name warmly.\nfunc Hello(name string) string { return \"Hello, \" + name }\n")
	report, err := verify.Run(context.Background(), verify.Options{Root: root, Skip: allButFormat})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if want := []string{"./lib/greet"}; !slices.Equal(report.Packages, want) {
		t.Errorf("Packages: want %v, got %v", want, report.Packages)
	}
}

func TestRule2_CleanTreeFallsBackToAllPackages(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	commitAll(t, root)
	report, err := verify.Run(context.Background(), verify.Options{Root: root, Skip: allButFormat})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if want := []string{"./..."}; !slices.Equal(report.Packages, want) {
		t.Errorf("Packages: want %v, got %v", want, report.Packages)
	}
}

func TestRule3_ReportPackagesAreSortedAndUnique(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	options := verify.Options{
		Root:     root,
		Packages: []string{"./lib/greet", "./lib/calc", "./lib/greet"},
		Skip:     allButFormat,
	}
	report, err := verify.Run(context.Background(), options)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if want := []string{"./lib/calc", "./lib/greet"}; !slices.Equal(report.Packages, want) {
		t.Errorf("Packages: want %v, got %v", want, report.Packages)
	}
}

func TestRule4_13_PassingRunReportsEveryStepInOrder(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	report, err := verify.Run(context.Background(), verify.Options{Root: root, Packages: []string{"./..."}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertStepOrder(t, report)
	for _, name := range []verify.StepName{verify.StepFormat, verify.StepVendor, verify.StepBuild, verify.StepVet, verify.StepTest} {
		assertPassed(t, stepNamed(t, report, name))
	}
	assertSkipped(t, stepNamed(t, report, verify.StepLint))
	if !report.OK() {
		t.Errorf("OK: want true, failed step: %+v", report.Failed())
	}
	if failed := report.Failed(); failed != nil {
		t.Errorf("Failed: want nil, got %+v", *failed)
	}
}

func TestRule5_SkippedStepsKeepTheirPlaceAndDoNotStopTheRun(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	options := verify.Options{
		Root:     root,
		Packages: []string{"./lib/calc"},
		Skip:     []verify.StepName{verify.StepFormat, verify.StepBuild},
	}
	report, err := verify.Run(context.Background(), options)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertStepOrder(t, report)
	assertSkipped(t, stepNamed(t, report, verify.StepFormat))
	assertSkipped(t, stepNamed(t, report, verify.StepBuild))
	assertPassed(t, stepNamed(t, report, verify.StepVendor))
	assertPassed(t, stepNamed(t, report, verify.StepVet))
	assertPassed(t, stepNamed(t, report, verify.StepTest))
	if !report.OK() {
		t.Errorf("OK: want true when only skips happened, failed step: %+v", report.Failed())
	}
}

func TestRule5_EveryStepSkipped(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	options := verify.Options{Root: root, Packages: []string{"./..."}, Skip: slices.Clone(verify.Order)}
	report, err := verify.Run(context.Background(), options)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertStepOrder(t, report)
	for _, step := range report.Steps {
		assertSkipped(t, step)
	}
	if !report.OK() {
		t.Error("OK: want true when every step was skipped")
	}
}

func TestRule6_9_BuildFailureStopsTheRun(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	writeFile(t, root, "lib/calc/broken.go", brokenBuildSource)
	report, err := verify.Run(context.Background(), verify.Options{Root: root, Packages: []string{"./lib/calc"}})
	if err != nil {
		t.Fatalf("Run: check failures belong in the Report, got error %v", err)
	}
	assertStepOrder(t, report)
	assertPassed(t, stepNamed(t, report, verify.StepFormat))
	assertPassed(t, stepNamed(t, report, verify.StepVendor))
	build := stepNamed(t, report, verify.StepBuild)
	assertFailed(t, build)
	if !strings.Contains(build.Output, "missingValue") {
		t.Errorf("build Output should hold the compiler output naming missingValue, got:\n%s", build.Output)
	}
	for _, name := range []verify.StepName{verify.StepVet, verify.StepTest, verify.StepLint} {
		assertSkipped(t, stepNamed(t, report, name))
	}
	if report.OK() {
		t.Error("OK: want false after a build failure")
	}
	if failed := report.Failed(); failed == nil || failed.Name != verify.StepBuild {
		t.Errorf("Failed: want the build step, got %+v", failed)
	}
}

func TestRule9_BuildOnlyCoversTheGivenPackages(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	writeFile(t, root, "lib/greet/broken.go", strings.Replace(brokenBuildSource, "package calc", "package greet", 1))
	options := verify.Options{Root: root, Packages: []string{"./lib/calc"}, Skip: []verify.StepName{verify.StepVet, verify.StepTest}}
	report, err := verify.Run(context.Background(), options)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertPassed(t, stepNamed(t, report, verify.StepBuild))
}

func TestRule10_VetFailureStopsTheRun(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	writeFile(t, root, "lib/calc/describe.go", vetFailureSource)
	report, err := verify.Run(context.Background(), verify.Options{Root: root, Packages: []string{"./lib/calc"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertPassed(t, stepNamed(t, report, verify.StepBuild))
	vet := stepNamed(t, report, verify.StepVet)
	assertFailed(t, vet)
	if !strings.Contains(vet.Output, "describe.go") {
		t.Errorf("vet Output should hold the vet output naming describe.go, got:\n%s", vet.Output)
	}
	assertSkipped(t, stepNamed(t, report, verify.StepTest))
	assertSkipped(t, stepNamed(t, report, verify.StepLint))
	if failed := report.Failed(); failed == nil || failed.Name != verify.StepVet {
		t.Errorf("Failed: want the vet step, got %+v", failed)
	}
}

func TestRule11_TestFailureCarriesTheTestOutput(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	writeFile(t, root, "lib/greet/broken_test.go", failingTestSource)
	report, err := verify.Run(context.Background(), verify.Options{Root: root, Packages: []string{"./lib/greet"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertPassed(t, stepNamed(t, report, verify.StepVet))
	test := stepNamed(t, report, verify.StepTest)
	assertFailed(t, test)
	if !strings.Contains(test.Output, "greet-failure-marker") {
		t.Errorf("test Output should hold the go test output, got:\n%s", test.Output)
	}
	assertSkipped(t, stepNamed(t, report, verify.StepLint))
	if failed := report.Failed(); failed == nil || failed.Name != verify.StepTest {
		t.Errorf("Failed: want the test step, got %+v", failed)
	}
}

func TestRule11_PassingTestKeepsOutputEmpty(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	report, err := verify.Run(context.Background(), verify.Options{Root: root, Packages: []string{"./lib/greet"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	test := stepNamed(t, report, verify.StepTest)
	assertPassed(t, test)
	if test.Output != "" {
		t.Errorf("passing test step: want empty Output, got %q", test.Output)
	}
}

func TestRule11_AllTestsTestsEveryPackage(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	writeFile(t, root, "lib/greet/broken_test.go", failingTestSource)
	scoped := verify.Options{Root: root, Packages: []string{"./lib/calc"}}

	report, err := verify.Run(context.Background(), scoped)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertPassed(t, stepNamed(t, report, verify.StepTest))

	everything := scoped
	everything.AllTests = true
	report, err = verify.Run(context.Background(), everything)
	if err != nil {
		t.Fatalf("Run with AllTests: %v", err)
	}
	test := stepNamed(t, report, verify.StepTest)
	assertFailed(t, test)
	if !strings.Contains(test.Output, "greet-failure-marker") {
		t.Errorf("AllTests should have tested ./lib/greet too, got Output:\n%s", test.Output)
	}
}

func TestRule11_RaceAddsTheRaceDetector(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	writeFile(t, root, "lib/racy/racy.go", racyPackageSource)
	writeFile(t, root, "lib/racy/racy_test.go", racyTestSource)
	options := verify.Options{Root: root, Packages: []string{"./lib/racy"}}

	report, err := verify.Run(context.Background(), options)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertPassed(t, stepNamed(t, report, verify.StepTest))

	options.Race = true
	report, err = verify.Run(context.Background(), options)
	if err != nil {
		t.Fatalf("Run with Race: %v", err)
	}
	test := stepNamed(t, report, verify.StepTest)
	assertFailed(t, test)
	if !strings.Contains(test.Output, "DATA RACE") {
		t.Errorf("Race run should report the data race, got Output:\n%s", test.Output)
	}
}

func TestRule7_FormatListsUnformattedFilesSortedAndRelative(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	writeFile(t, root, "unformatted.go", strings.Replace(unformattedSource, "package calc", "package fixture", 1))
	writeFile(t, root, "lib/greet/ugly.go", strings.Replace(unformattedSource, "package calc", "package greet", 1))
	writeFile(t, root, "lib/calc/ugly.go", unformattedSource)
	report, err := verify.Run(context.Background(), verify.Options{Root: root, Packages: []string{"./..."}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	format := stepNamed(t, report, verify.StepFormat)
	assertFailed(t, format)
	want := []string{"lib/calc/ugly.go", "lib/greet/ugly.go", "unformatted.go"}
	if got := trimmedLines(format.Output); !slices.Equal(got, want) {
		t.Errorf("gofmt Output: want lines %v, got %q", want, format.Output)
	}
	for _, name := range verify.Order[1:] {
		assertSkipped(t, stepNamed(t, report, name))
	}
	if failed := report.Failed(); failed == nil || failed.Name != verify.StepFormat {
		t.Errorf("Failed: want the gofmt step, got %+v", failed)
	}
}

func TestRule7_FormatOnlyChecksTheDirectoriesOfThePackages(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	writeFile(t, root, "unformatted.go", strings.Replace(unformattedSource, "package calc", "package fixture", 1))
	writeFile(t, root, "lib/greet/ugly.go", strings.Replace(unformattedSource, "package calc", "package greet", 1))

	options := verify.Options{Root: root, Packages: []string{"./lib/calc"}, Skip: allButFormat}
	report, err := verify.Run(context.Background(), options)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertPassed(t, stepNamed(t, report, verify.StepFormat))

	writeFile(t, root, "lib/calc/ugly.go", unformattedSource)
	report, err = verify.Run(context.Background(), options)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	format := stepNamed(t, report, verify.StepFormat)
	assertFailed(t, format)
	if got, want := trimmedLines(format.Output), []string{"lib/calc/ugly.go"}; !slices.Equal(got, want) {
		t.Errorf("gofmt Output: want lines %v, got %q", want, format.Output)
	}
}

func TestRule7_FormatOfAllPackagesExcludesVendorAndTestdata(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	writeFile(t, root, "testdata/ugly.go", unformattedSource)
	writeFile(t, root, "lib/calc/testdata/ugly.go", unformattedSource)
	writeFile(t, root, "vendor/modules.txt", "")
	writeFile(t, root, "vendor/example.com/dep/ugly.go", unformattedSource)
	options := verify.Options{Root: root, Packages: []string{"./..."}, Skip: allButFormat}
	report, err := verify.Run(context.Background(), options)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertPassed(t, stepNamed(t, report, verify.StepFormat))
}

func TestRule8_VendorDriftIsSyncedAndReported(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	writeFile(t, root, "vendor/modules.txt", staleModulesTxt)
	report, err := verify.Run(context.Background(), verify.Options{Root: root, Packages: []string{"./lib/calc"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	vendorStep := stepNamed(t, report, verify.StepVendor)
	assertPassed(t, vendorStep)
	if vendorStep.Output != "vendor synced" {
		t.Errorf("vendor Output: want %q, got %q", "vendor synced", vendorStep.Output)
	}
	assertPassed(t, stepNamed(t, report, verify.StepBuild))
	if !report.OK() {
		t.Errorf("OK: want true after the vendor sync, failed step: %+v", report.Failed())
	}
	runGo(t, root, "list", "-m")
}

func TestRule8_NoVendorDirectoryPassesQuietly(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	options := verify.Options{Root: root, Packages: []string{"./lib/calc"}, Skip: []verify.StepName{verify.StepBuild, verify.StepVet, verify.StepTest}}
	report, err := verify.Run(context.Background(), options)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	vendorStep := stepNamed(t, report, verify.StepVendor)
	assertPassed(t, vendorStep)
	if vendorStep.Output != "" {
		t.Errorf("vendor Output: want empty when nothing changed, got %q", vendorStep.Output)
	}
	if _, err := os.Stat(filepath.Join(root, "vendor")); !os.IsNotExist(err) {
		t.Errorf("vendor step created a vendor directory: %v", err)
	}
}

func TestRule8_SyncVendorErrorFailsTheVendorStep(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	writeFile(t, root, "go.mod", "module example.com/fixture\n\ngo 1.26\n\nbogus directive\n")
	writeFile(t, root, "vendor/modules.txt", "")
	report, err := verify.Run(context.Background(), verify.Options{Root: root, Packages: []string{"./lib/calc"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertPassed(t, stepNamed(t, report, verify.StepFormat))
	assertFailed(t, stepNamed(t, report, verify.StepVendor))
	for _, name := range []verify.StepName{verify.StepBuild, verify.StepVet, verify.StepTest, verify.StepLint} {
		assertSkipped(t, stepNamed(t, report, name))
	}
	if failed := report.Failed(); failed == nil || failed.Name != verify.StepVendor {
		t.Errorf("Failed: want the vendor step, got %+v", failed)
	}
}

func TestRule12_LintIsSkippedWithoutAVersion(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	options := verify.Options{Root: root, Packages: []string{"./lib/calc"}, Skip: allButFormat}
	report, err := verify.Run(context.Background(), options)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	assertSkipped(t, stepNamed(t, report, verify.StepLint))
	if !report.OK() {
		t.Error("OK: a lint step skipped for lack of a version is not a failure")
	}
}

func TestRule14_CancelDuringTestStepFailsItAndSkipsTheRest(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	marker := filepath.Join(t.TempDir(), "test-started")
	slowTest := fmt.Sprintf(`package slow

import (
	"os"
	"testing"
	"time"
)

func TestWaitsForCancellation(t *testing.T) {
	if err := os.WriteFile(%q, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Minute)
}
`, marker)
	writeFile(t, root, "lib/slow/slow.go", "// Package slow holds a test that waits to be cancelled.\npackage slow\n")
	writeFile(t, root, "lib/slow/slow_test.go", slowTest)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	type outcome struct {
		report verify.Report
		err    error
	}
	finished := make(chan outcome, 1)
	go func() {
		report, err := verify.Run(ctx, verify.Options{Root: root, Packages: []string{"./lib/slow"}})
		finished <- outcome{report, err}
	}()

	if !waitForFile(t, marker, 3*time.Minute) {
		t.Fatal("the slow test never started")
	}
	cancel()
	cancelledAt := time.Now()

	var result outcome
	select {
	case result = <-finished:
	case <-time.After(60 * time.Second):
		t.Fatal("Run did not stop within 60s of the cancellation")
	}
	if waited := time.Since(cancelledAt); waited > 60*time.Second {
		t.Errorf("Run took %v to stop after cancellation", waited)
	}
	if !errors.Is(result.err, context.Canceled) {
		t.Errorf("Run error: want context.Canceled, got %v", result.err)
	}
	report := result.report
	assertStepOrder(t, report)
	assertPassed(t, stepNamed(t, report, verify.StepVet))
	test := stepNamed(t, report, verify.StepTest)
	assertFailed(t, test)
	if !strings.Contains(strings.ToLower(test.Output), "cancel") {
		t.Errorf("test Output should name the cancellation, got:\n%s", test.Output)
	}
	assertSkipped(t, stepNamed(t, report, verify.StepLint))
	if failed := report.Failed(); failed == nil || failed.Name != verify.StepTest {
		t.Errorf("Failed: want the test step, got %+v", failed)
	}
}

func TestRule14_AlreadyCancelledContextFailsTheFirstStep(t *testing.T) {
	t.Parallel()
	root := copyFixture(t, "module")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report, err := verify.Run(ctx, verify.Options{Root: root, Packages: []string{"./..."}})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run error: want context.Canceled, got %v", err)
	}
	assertStepOrder(t, report)
	failed := report.Failed()
	if failed == nil {
		t.Fatal("Failed: want the step that was cancelled, got nil")
	}
	if !strings.Contains(strings.ToLower(failed.Output), "cancel") {
		t.Errorf("failed step Output should name the cancellation, got %q", failed.Output)
	}
	seenFailure := false
	for _, step := range report.Steps {
		if seenFailure {
			assertSkipped(t, step)
		}
		if step.Name == failed.Name {
			seenFailure = true
		}
	}
}

// trimmedLines returns the non-empty lines of output with surrounding spaces removed.
func trimmedLines(output string) []string {
	var lines []string
	for _, line := range outputLines(output) {
		lines = append(lines, strings.TrimSpace(line))
	}
	return lines
}
