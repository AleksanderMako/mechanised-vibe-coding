// Command app is the only main package; everything it reaches is live.
package main

import "example.com/fixture/lib"

func main() {
	total := lib.Live()
	total += lib.Soon()
	widget := &lib.Widget{}
	total += widget.Used()
	doubled := lib.Map([]int{1, 2}, func(n int) int { return n * 2 })
	total += len(doubled)
	total += lib.WithClosure()
	println(total)
}

// marker:unusedHelper
func unusedHelper() int {
	return 0
}
