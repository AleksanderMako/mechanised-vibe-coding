// Package dependent compiles on its own but imports the broken stale package.
package dependent

import (
	"example.com/fixture/lib"
	"example.com/fixture/stale"
)

// marker:CallsLib
func CallsLib() int {
	return lib.OnlyDependent() + stale.StaleFunc()
}
