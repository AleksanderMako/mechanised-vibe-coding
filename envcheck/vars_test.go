package envcheck_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/envcheck"
)

// Rules 2, 3 and 5: every tag form in declaration order, nested and embedded paths, duplicates kept.
func TestVarsReadsEveryTagFormInDeclarationOrder(t *testing.T) {
	vars, err := envcheck.Vars(fixturePath(t, "configpkg"), "Config")
	if err != nil {
		t.Fatalf("Vars: %v", err)
	}
	want := []envcheck.Var{
		{Name: "PORT", Field: "Port"},
		{Name: "APP_HOST", Field: "Host", Required: true},
		{Name: "APP_MODE", Field: "Mode", Required: false, Default: "dev"},
		{Name: "APP_TIMEOUT", Field: "Timeout", Default: "30s"},
		{Name: "DB_HOST", Field: "Database.Host", Required: true},
		{Name: "DB_PASSWORD", Field: "Database.Password"},
		{Name: "LOG_LEVEL", Field: "Logging.Level", Default: "info"},
		{Name: "SHARED_URL", Field: "Primary"},
		{Name: "SHARED_URL", Field: "Fallback", Required: true},
	}
	if !reflect.DeepEqual(vars, want) {
		t.Errorf("Vars mismatch\nwant %+v\ngot  %+v", want, vars)
	}
}

// Rule 3: a bare env part gives the upper cased field name.
func TestVarsBareEnvUsesUpperCasedFieldName(t *testing.T) {
	vars := mustVars(t)
	port := varForField(t, vars, "Port")
	if port.Name != "PORT" {
		t.Errorf("bare env on field Port: want name PORT, got %q", port.Name)
	}
}

// Rule 3: a default clears required, and default alone leaves required false.
func TestVarsDefaultClearsRequired(t *testing.T) {
	vars := mustVars(t)
	mode := varForField(t, vars, "Mode")
	if mode.Required || mode.Default != "dev" {
		t.Errorf("required plus default: want Required false Default dev, got %+v", mode)
	}
	timeout := varForField(t, vars, "Timeout")
	if timeout.Required || timeout.Default != "30s" {
		t.Errorf("default only: want Required false Default 30s, got %+v", timeout)
	}
	host := varForField(t, vars, "Host")
	if !host.Required || host.Default != "" {
		t.Errorf("required only: want Required true and empty Default, got %+v", host)
	}
}

// Rules 3 and 4: fields without an env part, untagged scalars and arg:"-" fields are skipped.
func TestVarsSkipsFieldsWithoutEnvAndDashedFields(t *testing.T) {
	vars := mustVars(t)
	for _, variable := range vars {
		switch {
		case variable.Field == "Verbose", variable.Field == "Untagged", variable.Field == "Secret":
			t.Errorf("field %s should be skipped, got %+v", variable.Field, variable)
		case strings.HasPrefix(variable.Field, "Hidden"):
			t.Errorf("nested struct under arg:\"-\" should be skipped, got %+v", variable)
		case strings.HasSuffix(variable.Field, "Ignored"):
			t.Errorf("nested field with arg:\"-\" should be skipped, got %+v", variable)
		}
	}
}

// Rule 2: nested struct fields get a dotted path, embedded structs add their type name.
func TestVarsNestedAndEmbeddedPaths(t *testing.T) {
	vars := mustVars(t)
	varForField(t, vars, "Database.Host")
	varForField(t, vars, "Database.Password")
	varForField(t, vars, "Logging.Level")
}

// Rule 5: a variable read by two fields appears twice.
func TestVarsKeepsDuplicateNames(t *testing.T) {
	vars := mustVars(t)
	count := 0
	for _, variable := range vars {
		if variable.Name == "SHARED_URL" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("SHARED_URL is read by two fields: want 2 entries, got %d in %+v", count, vars)
	}
}

// Rule 1: structs declared in test files are not visible.
func TestVarsIgnoresTestFiles(t *testing.T) {
	_, err := envcheck.Vars(fixturePath(t, "configpkg"), "TestOnlyConfig")
	assertErrorMentions(t, err, "TestOnlyConfig")
}

// Rule 1: an unknown type is an error naming it.
func TestVarsUnknownTypeIsError(t *testing.T) {
	_, err := envcheck.Vars(fixturePath(t, "configpkg"), "NoSuchConfig")
	assertErrorMentions(t, err, "NoSuchConfig")
}

// Rule 1: a top level type that is not a struct is an error naming it.
func TestVarsNonStructTypeIsError(t *testing.T) {
	_, err := envcheck.Vars(fixturePath(t, "configpkg"), "NotAStruct")
	assertErrorMentions(t, err, "NotAStruct")
}

// Rule 1: a struct declared inside a function is not top level.
func TestVarsLocalStructIsError(t *testing.T) {
	_, err := envcheck.Vars(fixturePath(t, "configpkg"), "LocalConfig")
	assertErrorMentions(t, err, "LocalConfig")
}

// Rule 1: a directory with no Go files is an error.
func TestVarsEmptyDirectoryIsError(t *testing.T) {
	if _, err := envcheck.Vars(t.TempDir(), "Config"); err == nil {
		t.Error("want an error for a directory with no Go files, got nil")
	}
}

// Rule 1: a directory that does not exist is an error.
func TestVarsMissingDirectoryIsError(t *testing.T) {
	if _, err := envcheck.Vars(fixturePath(t, "no-such-package"), "Config"); err == nil {
		t.Error("want an error for a missing directory, got nil")
	}
}

// mustVars reads the Config fixture or fails the test.
func mustVars(t *testing.T) []envcheck.Var {
	t.Helper()
	vars, err := envcheck.Vars(fixturePath(t, "configpkg"), "Config")
	if err != nil {
		t.Fatalf("Vars: %v", err)
	}
	return vars
}

// varForField returns the var with the given field path or fails the test.
func varForField(t *testing.T, vars []envcheck.Var, field string) envcheck.Var {
	t.Helper()
	for _, variable := range vars {
		if variable.Field == field {
			return variable
		}
	}
	t.Fatalf("no var for field %q in %+v", field, vars)
	return envcheck.Var{}
}
