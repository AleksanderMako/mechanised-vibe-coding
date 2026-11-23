package user

import (
	"testing"

	"example.com/fixture/shapes"
)

func TestMeasure(t *testing.T) {
	square := shapes.NewSquare(5) // ref:newsquare-test-call
	if Measure(square) != 25 {
		t.Fatal("wrong area")
	}
}
