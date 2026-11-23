// Package store is the source package of the move tests.
package store

import (
	"fmt"
	"strings"
)

// Label is a display label.
type Label string

// Format renders a label for display.
func Format(label Label) string {
	return fmt.Sprintf("[%s]", label)
}

// Banner stays in store and calls Format.
func Banner(title string) string {
	return Format(Label(title))
}

// suffix is appended by Shout and used nowhere else.
const suffix = "!"

// Shout upper cases text and appends suffix.
func Shout(text string) string {
	return strings.ToUpper(text) + suffix
}

// clean trims surrounding spaces.
func clean(text string) string {
	return strings.TrimSpace(text)
}

// Keep stays in store and calls clean.
func Keep(text string) string {
	return clean(text)
}

// tidy lower cases text.
func tidy(text string) string {
	return strings.ToLower(text)
}

// Tidied calls tidy, its only user.
func Tidied(text string) string {
	return tidy(text)
}

// Join joins words with spaces.
func Join(words []string) string {
	return strings.Join(words, " ")
}
