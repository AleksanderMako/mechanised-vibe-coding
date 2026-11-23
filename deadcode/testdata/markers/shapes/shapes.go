// Package shapes holds marker methods and look-alikes that break one condition of the marker rule.
package shapes

// shape is implemented by Circle and Square through their marker methods.
type shape interface {
	isShape()
}

// Circle is a shape through a value receiver marker.
type Circle struct{}

// marker:Circle.isShape
func (Circle) isShape() {}

// Square is a shape through a pointer receiver marker.
type Square struct{}

// marker:Square.isShape
func (*Square) isShape() {}

// busy would be a marker interface if the method body were empty.
type busy interface {
	isBusy()
}

// Busy has a body with a statement.
type Busy struct{ calls int }

// marker:Busy.isBusy
func (b *Busy) isBusy() {
	b.calls++
}

// Loud is a marker interface with an exported method.
type Loud interface {
	IsLoud()
}

// Speaker has an exported method.
type Speaker struct{}

// marker:Speaker.IsLoud
func (Speaker) IsLoud() {}

// sized takes a parameter.
type sized interface {
	resize(int)
}

// Box has a method with a parameter.
type Box struct{}

// marker:Box.resize
func (Box) resize(int) {}

// Foreign has a method only an interface in another package names.
type Foreign struct{}

// marker:Foreign.isForeign
func (Foreign) isForeign() {}

// Plain has a marker shaped method but no interface anywhere asks for it.
type Plain struct{}

// marker:Plain.isPlain
func (Plain) isPlain() {}

// marker:Count
func Count() int {
	return 6
}

// marker:Unused
func Unused() int {
	return 0
}

// Tested would be a marker only through the interface its test variant adds, but the plain copy is the one checked.
type Tested struct{}

// marker:Tested.isTested
func (Tested) isTested() {}
