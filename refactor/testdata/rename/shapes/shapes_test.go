package shapes

import "testing"

func TestShapes(t *testing.T) {
	if Area(2, 3) != 6 {
		t.Fatal("Area")
	}
	if Internal() != 1 || helper() != 2 {
		t.Fatal("Internal")
	}
	scaled := Rect{Width: 1, Height: 2}.Scale(3)
	if scaled.Width != 3 {
		t.Fatal("Scale")
	}
}
