// Package elsewhere declares an interface whose method shapes.Foreign only looks like it implements.
package elsewhere

// Marker has the same method name as shapes.Foreign.isForeign, but unexported names belong to their package.
type Marker interface {
	isForeign()
}
