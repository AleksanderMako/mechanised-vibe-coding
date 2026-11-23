package shapes

import "testing"

func TestArea(t *testing.T) {
	tests := []struct {
		name string
		side int
		want int
	}{
		{name: "one", side: 1, want: 1},
		{name: "two", side: 2, want: 4},
	}
	for _, tt := range tests {
		if got := NewSquare(tt.side).Area(); got != tt.want {
			t.Errorf("%s: got %d, want %d", tt.name, got, tt.want)
		}
	}
}
