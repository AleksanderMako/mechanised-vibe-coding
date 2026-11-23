// Package consumer imports store from outside.
package consumer

import "example.com/fixture/store"

// Use calls every exported store function.
func Use() []string {
	return []string{
		store.Format(store.Label("a")),
		store.Banner("b"),
		store.Shout("c"),
		store.Keep(" d "),
		store.Tidied("E"),
		store.Join([]string{"f", "g"}),
		store.Pad("h"),
	}
}
