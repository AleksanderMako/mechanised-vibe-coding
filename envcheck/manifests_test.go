package envcheck_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/envcheck"
)

// Rules 9 and 10: workload kinds are read, Service and ConfigMap are skipped, one source per container.
func TestReadManifestsMultiDocumentFile(t *testing.T) {
	path := fixturePath(t, "manifests", "app.yaml")
	sources, err := envcheck.ReadManifests(path)
	if err != nil {
		t.Fatalf("ReadManifests: %v", err)
	}
	assertSourceNames(t, sources,
		path+"#Deployment/web/migrate",
		path+"#Deployment/web/server",
		path+"#Deployment/web/sidecar",
		path+"#CronJob/nightly/report",
		path+"#Pod/debug/shell",
	)

	assertVars(t, sourceNamed(t, sources, path+"#Deployment/web/migrate"), map[string]string{
		"DB_HOST": "db.internal",
	})
	// envFrom entries add no keys; valueFrom becomes the reference marker.
	assertVars(t, sourceNamed(t, sources, path+"#Deployment/web/server"), map[string]string{
		"APP_HOST":    "0.0.0.0",
		"DB_PASSWORD": envcheck.ValueFromReference,
		"PORT":        "8080",
	})
	assertVars(t, sourceNamed(t, sources, path+"#Deployment/web/sidecar"), map[string]string{})
	assertVars(t, sourceNamed(t, sources, path+"#CronJob/nightly/report"), map[string]string{
		"REPORT_MODE": "full",
		"API_KEY":     envcheck.ValueFromReference,
	})
	assertVars(t, sourceNamed(t, sources, path+"#Pod/debug/shell"), map[string]string{
		"DEBUG": "true",
	})

	for _, source := range sources {
		if strings.Contains(source.Name, "Service") || strings.Contains(source.Name, "ConfigMap") {
			t.Errorf("skipped kinds must yield no source, got %s", source.Name)
		}
		if _, ok := source.Vars["SHOULD_NOT_APPEAR"]; ok {
			t.Errorf("ConfigMap data leaked into %s", source.Name)
		}
	}
}

// Rule 9: StatefulSet, DaemonSet and Job read the pod spec at spec.template.spec.
func TestReadManifestsOtherWorkloadKinds(t *testing.T) {
	path := fixturePath(t, "manifests", "workloads.yaml")
	sources, err := envcheck.ReadManifests(path)
	if err != nil {
		t.Fatalf("ReadManifests: %v", err)
	}
	assertSourceNames(t, sources,
		path+"#StatefulSet/store/db",
		path+"#DaemonSet/agent/collector",
		path+"#Job/once/task",
	)
	assertVars(t, sourceNamed(t, sources, path+"#StatefulSet/store/db"), map[string]string{"STORE_DIR": "/data"})
	assertVars(t, sourceNamed(t, sources, path+"#DaemonSet/agent/collector"), map[string]string{"NODE_NAME": envcheck.ValueFromReference})
	assertVars(t, sourceNamed(t, sources, path+"#Job/once/task"), map[string]string{"TASK_ID": "7"})
}

// ReadManifests takes several paths and returns the containers of all of them.
func TestReadManifestsSeveralFiles(t *testing.T) {
	appPath := fixturePath(t, "manifests", "app.yaml")
	workloadsPath := fixturePath(t, "manifests", "workloads.yaml")
	sources, err := envcheck.ReadManifests(appPath, workloadsPath)
	if err != nil {
		t.Fatalf("ReadManifests: %v", err)
	}
	if len(sources) != 8 {
		t.Errorf("want 8 sources from both files, got %d: %v", len(sources), sourceNames(sources))
	}
	sourceNamed(t, sources, appPath+"#Pod/debug/shell")
	sourceNamed(t, sources, workloadsPath+"#Job/once/task")
}

// Rule 10: the source name uses the path as given, relative paths included.
func TestReadManifestsKeepsRelativePathInName(t *testing.T) {
	path := "testdata/manifests/workloads.yaml"
	sources, err := envcheck.ReadManifests(path)
	if err != nil {
		t.Fatalf("ReadManifests: %v", err)
	}
	sourceNamed(t, sources, path+"#Job/once/task")
}

// Rule 11: a document that is not YAML is an error naming the file.
func TestReadManifestsInvalidDocumentIsError(t *testing.T) {
	_, err := envcheck.ReadManifests(fixturePath(t, "manifests", "broken.yaml"))
	assertErrorMentions(t, err, "broken.yaml")
}

// Rule 11: a workload without its pod spec path is skipped, not an error.
func TestReadManifestsSkipsWorkloadWithoutPodSpec(t *testing.T) {
	path := fixturePath(t, "manifests", "no-template.yaml")
	sources, err := envcheck.ReadManifests(path)
	if err != nil {
		t.Fatalf("ReadManifests: %v", err)
	}
	assertSourceNames(t, sources, path+"#Pod/survivor/main")
	assertVars(t, sources[0], map[string]string{"ALIVE": "yes"})
}

// A manifest file that does not exist cannot be read.
func TestReadManifestsMissingFileIsError(t *testing.T) {
	if _, err := envcheck.ReadManifests(fixturePath(t, "manifests", "absent.yaml")); err == nil {
		t.Error("want an error for a missing file, got nil")
	}
}

// assertSourceNames checks the sources have exactly the wanted names, in any order.
func assertSourceNames(t *testing.T, sources []envcheck.Source, want ...string) {
	t.Helper()
	got := sourceNames(sources)
	sort.Strings(got)
	sorted := append([]string(nil), want...)
	sort.Strings(sorted)
	if strings.Join(got, "\n") != strings.Join(sorted, "\n") {
		t.Errorf("source names mismatch\nwant %v\ngot  %v", sorted, got)
	}
}
