// Package audit already imports store.
package audit

import "example.com/fixture/store"

// Line joins words for the audit log.
func Line(words []string) string {
	return store.Join(words)
}
