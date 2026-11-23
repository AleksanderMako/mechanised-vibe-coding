// Package report uses store only for Format and Label.
package report

import "example.com/fixture/store"

// Title formats a report title.
func Title(name string) string {
	return store.Format(store.Label(name))
}
