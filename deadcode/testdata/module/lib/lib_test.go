package lib_test

import (
	"testing"

	"example.com/fixture/lib"
)

func TestOnlyTests(t *testing.T) {
	if lib.OnlyTests() != 7 {
		t.Fatal("OnlyTests changed")
	}
}

func BenchmarkOnlyBenchmark(b *testing.B) {
	for b.Loop() {
		lib.OnlyBenchmark()
	}
}

func FuzzOnlyFuzz(f *testing.F) {
	// Called directly, not inside f.Fuzz, because rta does not follow the reflective fuzz call.
	lib.OnlyFuzz()
	f.Fuzz(func(t *testing.T, n int) {})
}

func ExampleOnlyExample() {
	println(lib.OnlyExample())
	// Output:
}
