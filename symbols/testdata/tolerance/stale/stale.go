// Package stale is broken and imports nothing from lib, like a stale generated directory.
package stale

// Value has a type error on purpose: missingThing is not declared anywhere.
var Value int = missingThing

// StaleSpeaker type-checks although its package is broken, so it still implements lib.Speaker structurally.
type StaleSpeaker struct{}

// Speak makes StaleSpeaker a Speaker.
func (StaleSpeaker) Speak() string {
	return "stale"
}
