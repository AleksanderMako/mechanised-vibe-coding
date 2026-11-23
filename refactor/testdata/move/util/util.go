// Package util is an existing destination.
package util

import "strings"

// Tidy already exists, so an exported store.tidy would clash.
func Tidy(text string) string {
	return strings.ToLower(text)
}

// Join already exists, so moving store.Join here clashes.
func Join(words []string) string {
	return strings.Join(words, ",")
}
