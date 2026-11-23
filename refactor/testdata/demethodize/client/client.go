// Package client calls calc methods from another package.
package client

import "example.com/fixture/calc"

// holder lets a call go through a selector of an identifier.
type holder struct {
	inner calc.Adder
}

// Run calls the methods through every receiver form that is safe to drop.
func Run() int {
	adder := calc.Adder{Name: "a"}
	pointer := &adder
	box := holder{inner: adder}
	sum := adder.Sum(1, 2) + pointer.Sum(3, 4) + box.inner.Sum(5, 6) + (*pointer).Sum(7, 8)
	other := adder.Double(2) + pointer.Triple(3) + adder.Triple(4)
	return sum + other + len(pointer.Name) + len(box.inner.Name)
}
