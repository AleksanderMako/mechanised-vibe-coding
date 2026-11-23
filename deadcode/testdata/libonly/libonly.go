// Package libonly has no main package; only tests can give it roots.
package libonly

// marker:Helper
func Helper() int {
	return 1
}

// marker:Unused
func Unused() int {
	return 2
}
