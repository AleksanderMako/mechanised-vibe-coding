// Package lib is healthy; one of its functions is called only from an excluded package.
package lib

// marker:Used
func Used() int {
	return 1
}

// marker:OnlyDependent
func OnlyDependent() int {
	return 2
}
