// Command app is the only main package and imports lib only.
package main

import "example.com/fixture/lib"

func main() {
	println(lib.Used())
}
