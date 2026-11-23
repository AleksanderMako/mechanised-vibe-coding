// Package stale is broken and imports nothing from lib.
package stale

// Value has a type error on purpose: missingThing is not declared anywhere.
var Value int = missingThing
