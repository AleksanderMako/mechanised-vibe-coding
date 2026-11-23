// Package stale is broken and imports nothing from lib, so refactorings of lib ignore it.
package stale

// Summer has the method lib.Adder has; were stale considered, demethodizing Adder.Sum would break it.
type Summer interface {
	Sum() int
}

// Total uses Summer as a type.
func Total(summer Summer) int {
	return summer.Sum()
}

// Value has a type error on purpose: missingThing is not declared anywhere.
var Value int = missingThing
