// Command app is a healthy user of lib.
package main

import (
	"fmt"

	"example.com/fixture/lib"
)

func main() {
	config := lib.Config{Name: "app"} // ref:app-config
	speaker := lib.Speaker(lib.LibSpeaker{})
	fmt.Println(lib.Hello(config.Name), speaker.Speak()) // ref:app-hello
}
