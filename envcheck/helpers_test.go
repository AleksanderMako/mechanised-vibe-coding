package envcheck_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/AleksanderMako/machanised-vibe-coding/envcheck"
)

// fixturePath returns the absolute path of testdata/<parts...> so no test depends on the working directory inside the code under test.
func fixturePath(t *testing.T, parts ...string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join(append([]string{"testdata"}, parts...)...))
	if err != nil {
		t.Fatalf("absolute fixture path: %v", err)
	}
	return path
}

// sourceNamed returns the source called name or fails the test.
func sourceNamed(t *testing.T, sources []envcheck.Source, name string) envcheck.Source {
	t.Helper()
	for _, source := range sources {
		if source.Name == name {
			return source
		}
	}
	t.Fatalf("no source named %q in %v", name, sourceNames(sources))
	return envcheck.Source{}
}

// sourceNames lists the names of sources in order, for failure messages and set comparisons.
func sourceNames(sources []envcheck.Source) []string {
	names := make([]string, 0, len(sources))
	for _, source := range sources {
		names = append(names, source.Name)
	}
	return names
}

// assertVars checks a source holds exactly the wanted variables and values.
func assertVars(t *testing.T, source envcheck.Source, want map[string]string) {
	t.Helper()
	if len(source.Vars) != len(want) {
		t.Errorf("source %s: want %d vars %v, got %d vars %v", source.Name, len(want), want, len(source.Vars), source.Vars)
	}
	for key, wantValue := range want {
		gotValue, ok := source.Vars[key]
		if !ok {
			t.Errorf("source %s: missing key %q", source.Name, key)
			continue
		}
		if gotValue != wantValue {
			t.Errorf("source %s: key %q want %q, got %q", source.Name, key, wantValue, gotValue)
		}
	}
}

// assertContainsAll checks text mentions every one of the wanted words.
func assertContainsAll(t *testing.T, text string, wanted ...string) {
	t.Helper()
	for _, word := range wanted {
		if !strings.Contains(text, word) {
			t.Errorf("want %q to mention %q", text, word)
		}
	}
}

// assertErrorMentions checks err is not nil and its message holds every wanted word.
func assertErrorMentions(t *testing.T, err error, wanted ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("want an error mentioning %v, got nil", wanted)
	}
	assertContainsAll(t, err.Error(), wanted...)
}
