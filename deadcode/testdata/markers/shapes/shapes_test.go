package shapes

import "testing"

// tested exists only in the test variant of shapes.
type tested interface {
	isTested()
}

func TestCount(t *testing.T) {
	if Count() != 6 {
		t.Fatal("Count changed")
	}
}
