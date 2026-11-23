package greet

import "testing"

func TestHello(t *testing.T) {
	if got := Hello("bob"); got != "Hello, BOB!" {
		t.Fatalf("got %q", got)
	}
}
