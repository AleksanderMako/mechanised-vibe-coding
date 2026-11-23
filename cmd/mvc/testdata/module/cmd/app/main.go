// Command app is the only main package; everything it reaches is live.
package main

import (
	"example.com/fixture/alpha"
	"example.com/fixture/beta"
	"example.com/fixture/gamma"
)

func main() {
	println(alpha.Alpha() + beta.Beta() + gamma.Gamma())
}
