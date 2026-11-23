package store

import "testing"

func TestStore(t *testing.T) {
	if Format("x") != "[x]" {
		t.Fatal("Format")
	}
	if clean(" z ") != "z" {
		t.Fatal("clean")
	}
}
