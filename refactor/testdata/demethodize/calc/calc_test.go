package calc

import "testing"

func TestCalc(t *testing.T) {
	var adder Adder
	if adder.Sum(2, 2) != 4 {
		t.Fatal("Sum")
	}
	if adder.Square(3) != 9 {
		t.Fatal("Square")
	}
}
