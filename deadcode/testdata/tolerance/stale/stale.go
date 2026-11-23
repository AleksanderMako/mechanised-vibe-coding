// Package stale is broken and imported by no main package.
package stale

// Value has a type error on purpose: missingThing is not declared anywhere.
var Value int = missingThing

// marker:StaleFunc
func StaleFunc() int {
	return 3
}
