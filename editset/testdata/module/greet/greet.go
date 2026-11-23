// Package greet builds greetings.
package greet

import (
	"fmt"
	"strings"
)

// Hello returns a greeting for name.
func Hello(name string) string {
	return fmt.Sprintf("Hello, %s!", strings.ToUpper(name))
}
