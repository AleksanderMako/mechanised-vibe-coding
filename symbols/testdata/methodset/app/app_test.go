package app

import (
	"testing"

	"example.com/fixture/store"
)

func TestRun(t *testing.T) {
	_ = store.Open().Exec("t") // ref:test-exec
}
