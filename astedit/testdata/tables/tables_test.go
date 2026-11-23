package tables

import "testing"

func TestAssigned(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		// first case
		{name: "one", want: 1},
		{name: "two", want: 2},
	}
	for _, tt := range tests {
		_ = tt
	}
}

type addCase struct {
	a, b, want int
}

func TestNamedType(t *testing.T) {
	var cases []addCase
	cases = []addCase{
		{a: 1, b: 2, want: 3},
	}
	_ = cases
}

func TestRanged(t *testing.T) {
	for _, tc := range []struct{ in string }{
		{in: "a"},
	} {
		_ = tc
	}
}

func TestMap(t *testing.T) {
	cases := map[string]struct{ want int }{
		"first": {want: 1},
	}
	_ = cases
}

func TestNestedBlock(t *testing.T) {
	if testing.Short() {
		tests := []struct{ in int }{
			{in: 1},
		}
		_ = tests
	}
}

func TestTwoTables(t *testing.T) {
	first := []struct{ a int }{
		{a: 1},
	}
	second := []struct{ b int }{
		{b: 2},
	}
	_, _ = first, second
}

func TestNoTable(t *testing.T) {
	values := make([]int, 2)
	_ = values
}

type suite struct{}

func (suite) TestMethod(t *testing.T) {
	tests := []struct{ a int }{
		{a: 1},
	}
	_ = tests
}
