package envcheck_test

import (
	"reflect"
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/envcheck"
)

// checkVars is a hand built config: one required, one duplicated name, one missing twice, one provided empty.
var checkVars = []envcheck.Var{
	{Name: "APP_HOST", Field: "Host", Required: true},
	{Name: "DB_HOST", Field: "Database.Host"},
	{Name: "SHARED_URL", Field: "Primary"},
	{Name: "SHARED_URL", Field: "Fallback"},
	{Name: "API_KEY", Field: "APIKey", Required: true},
	{Name: "API_KEY", Field: "BackupAPIKey"},
	{Name: "EMPTY_OK", Field: "EmptyOK"},
	{Name: "LOG_LEVEL", Field: "Logging.Level", Default: "info"},
}

// checkSources provide some of checkVars, and one key no var reads.
var checkSources = []envcheck.Source{
	{Name: "first.env", Vars: map[string]string{"DB_HOST": "db", "SHARED_URL": "postgres://a", "EXTRA": "x"}},
	{Name: "second.env", Vars: map[string]string{"EMPTY_OK": ""}},
	{Name: "third.env", Vars: map[string]string{"DB_HOST": "db2", "SHARED_URL": "postgres://b"}},
}

// Rules 12 and 14: missing vars, each once, with every checked source listed in input order.
func TestCheckReportsMissingVarsOnce(t *testing.T) {
	missing, _ := envcheck.Check(checkVars, checkSources)
	wantSources := []string{"first.env", "second.env", "third.env"}
	byName := map[string]envcheck.Missing{}
	for _, entry := range missing {
		if _, seen := byName[entry.Var.Name]; seen {
			t.Errorf("rule 14: %s appears more than once in missing", entry.Var.Name)
		}
		byName[entry.Var.Name] = entry
		if !reflect.DeepEqual(entry.Sources, wantSources) {
			t.Errorf("missing %s: want Sources %v, got %v", entry.Var.Name, wantSources, entry.Sources)
		}
	}
	if len(byName) != 3 {
		t.Errorf("want APP_HOST, API_KEY and LOG_LEVEL missing, got %+v", missing)
	}
	for _, name := range []string{"APP_HOST", "API_KEY", "LOG_LEVEL"} {
		if _, ok := byName[name]; !ok {
			t.Errorf("want %s missing, got %+v", name, missing)
		}
	}
	// Required is kept on the Var and does not decide what is missing; a default does not either.
	if entry, ok := byName["APP_HOST"]; ok && !entry.Var.Required {
		t.Errorf("missing APP_HOST must keep Required true, got %+v", entry.Var)
	}
	if entry, ok := byName["LOG_LEVEL"]; ok && entry.Var.Required {
		t.Errorf("missing LOG_LEVEL must keep Required false, got %+v", entry.Var)
	}
}

// Rule 12: a key present with an empty value is provided.
func TestCheckEmptyValueCountsAsProvided(t *testing.T) {
	missing, provided := envcheck.Check(checkVars, checkSources)
	for _, entry := range missing {
		if entry.Var.Name == "EMPTY_OK" {
			t.Errorf("EMPTY_OK is a key in second.env and must not be missing")
		}
	}
	if !reflect.DeepEqual(provided["EMPTY_OK"], []string{"second.env"}) {
		t.Errorf("provided[EMPTY_OK]: want [second.env], got %v", provided["EMPTY_OK"])
	}
}

// Rule 13: provided maps each variable found in a source to those sources in input order.
func TestCheckProvidedListsSourcesInInputOrder(t *testing.T) {
	_, provided := envcheck.Check(checkVars, checkSources)
	want := map[string][]string{
		"DB_HOST":    {"first.env", "third.env"},
		"SHARED_URL": {"first.env", "third.env"},
		"EMPTY_OK":   {"second.env"},
	}
	if !reflect.DeepEqual(provided, want) {
		t.Errorf("provided mismatch\nwant %v\ngot  %v", want, provided)
	}
}

// Rule 12: with no sources every variable is missing and the checked list is empty.
func TestCheckWithoutSourcesEverythingIsMissing(t *testing.T) {
	vars := []envcheck.Var{{Name: "A", Field: "A"}, {Name: "B", Field: "B", Required: true}}
	missing, provided := envcheck.Check(vars, nil)
	if len(missing) != 2 {
		t.Fatalf("want 2 missing, got %+v", missing)
	}
	for _, entry := range missing {
		if len(entry.Sources) != 0 {
			t.Errorf("no sources checked, got %v for %s", entry.Sources, entry.Var.Name)
		}
	}
	if len(provided) != 0 {
		t.Errorf("want nothing provided, got %v", provided)
	}
}

// Rule 12: when every variable is provided nothing is missing.
func TestCheckNothingMissing(t *testing.T) {
	vars := []envcheck.Var{{Name: "A", Field: "A", Required: true}}
	sources := []envcheck.Source{{Name: "only.env", Vars: map[string]string{"A": "1"}}}
	missing, provided := envcheck.Check(vars, sources)
	if len(missing) != 0 {
		t.Errorf("want nothing missing, got %+v", missing)
	}
	if !reflect.DeepEqual(provided["A"], []string{"only.env"}) {
		t.Errorf("provided[A]: want [only.env], got %v", provided["A"])
	}
}
