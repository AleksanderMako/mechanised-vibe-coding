// Package calc declares the methods the demethodize tests turn into functions.
package calc

// Adder carries a name that most of its methods ignore.
type Adder struct {
	Name string
}

// Sum adds two numbers without using its named receiver.
func (a Adder) Sum(x, y int) int {
	return x + y
}

// Double has an unnamed receiver.
func (Adder) Double(x int) int {
	return x * 2
}

// Triple has a blank pointer receiver.
func (_ *Adder) Triple(x int) int {
	return x * 3
}

// Label uses its receiver.
func (a Adder) Label() string {
	return a.Name
}

// Negate is the method Negater asks for.
func (a Adder) Negate(x int) int {
	return -x
}

// Negater is used as a type below, so Adder must keep satisfying it.
type Negater interface {
	Negate(x int) int
}

var _ Negater = Adder{}

// Square is the method Squarer asks for.
func (Adder) Square(x int) int {
	return x * x
}

// Squarer is never used as a type.
type Squarer interface {
	Square(x int) int
}

// Clamp is a package level function whose name a method also uses.
func Clamp(x int) int {
	if x < 0 {
		return 0
	}
	return x
}

// Clamp as a method collides with the package level Clamp once demethodized.
func (Adder) Clamp(x int) int {
	return Clamp(x)
}

// Total calls Sum inside the package.
func Total() int {
	var adder Adder
	return adder.Sum(1, 2) + adder.Double(1)
}
