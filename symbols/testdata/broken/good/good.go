// Package good loads cleanly so only its sibling causes the failure.
package good

// Fine is a symbol the tests ask about while the module is broken.
func Fine() {}

// Box is a struct so StructTags has a valid target in the broken module.
type Box struct {
	Size int
}

// Sizer is an interface so Implementations has a valid target in the broken module.
type Sizer interface {
	Size() int
}
