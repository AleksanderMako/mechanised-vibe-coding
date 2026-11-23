// Command mvc exposes the packages of this module as subcommands; it parses
// flags, calls one package function and prints the result, nothing more.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "mvc: not implemented")
	os.Exit(2)
}
