package envcheck_test

import (
	"testing"

	"github.com/AleksanderMako/mechanised-vibe-coding/envcheck"
)

// Rules 6, 7 and 8: comments, export, quotes, escapes, trailing comments and duplicate keys.
func TestReadEnvFileParsesEveryLineForm(t *testing.T) {
	path := fixturePath(t, "env", "app.env")
	source, err := envcheck.ReadEnvFile(path)
	if err != nil {
		t.Fatalf("ReadEnvFile: %v", err)
	}
	if source.Name != path {
		t.Errorf("Name: want the path as given %q, got %q", path, source.Name)
	}
	assertVars(t, source, map[string]string{
		"EXPORTED":          "yes",
		"PLAIN":             "hello",
		"PADDED":            "padded value",
		"SINGLE":            "single # not a comment",
		"SINGLE_NO_ESCAPES": `keep\nliteral`,
		"DOUBLE":            "line1\nline2 \"quoted\" back\\slash",
		"HASH_IN_QUOTES":    "a # b",
		"TRAILING":          "value",
		"HASH_NO_SPACE":     "abc#def",
		"EMPTY":             "",
		"URL":               "postgres://user:pass@host/db?sslmode=disable",
		"DUP":               "second",
	})
}

// Rule 8: Name is the path exactly as the caller gave it, relative paths included.
func TestReadEnvFileKeepsRelativePathAsName(t *testing.T) {
	path := "testdata/env/app.env"
	source, err := envcheck.ReadEnvFile(path)
	if err != nil {
		t.Fatalf("ReadEnvFile: %v", err)
	}
	if source.Name != path {
		t.Errorf("Name: want %q, got %q", path, source.Name)
	}
}

// Rule 6: a line without = is an error naming its line number (line 6 of bad.env).
func TestReadEnvFileLineWithoutEqualsIsError(t *testing.T) {
	_, err := envcheck.ReadEnvFile(fixturePath(t, "env", "bad.env"))
	assertErrorMentions(t, err, "6")
}

// A file that does not exist cannot be read.
func TestReadEnvFileMissingFileIsError(t *testing.T) {
	if _, err := envcheck.ReadEnvFile(fixturePath(t, "env", "absent.env")); err == nil {
		t.Error("want an error for a missing file, got nil")
	}
}
