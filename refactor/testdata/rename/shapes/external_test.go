package shapes_test

import (
	"testing"

	"example.com/fixture/shapes"
)

func TestExternal(t *testing.T) {
	if shapes.Area(1, 1) != 1 {
		t.Fatal("Area")
	}
}
