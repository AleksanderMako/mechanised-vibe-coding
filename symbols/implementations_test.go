package symbols_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/AleksanderMako/machanised-vibe-coding/symbols"
)

// implementationsOf runs Implementations on a fresh copy of the main fixture and fails on error.
func implementationsOf(t *testing.T, iface symbols.Symbol, opts symbols.Options) []symbols.Symbol {
	t.Helper()
	root := copyFixture(t, "module")
	found, err := symbols.Implementations(t.Context(), root, iface, opts)
	if err != nil {
		t.Fatalf("Implementations(%s): %v", iface, err)
	}
	return found
}

func TestImplementationsFindValueAndPointerReceivers(t *testing.T) {
	// Point and Unit lack Area; Tagged embeds Square and *Circle, whose Area methods collide; Figure is an alias; ShapeSet is an interface.
	got := implementationsOf(t, sym("shapes", "Shape"), symbols.Options{})
	want := []symbols.Symbol{
		sym("shapes", "Circle"),
		sym("shapes", "Square"),
		sym("user", "Holder"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Implementations(Shape): want %v, got %v", want, got)
	}
}

func TestImplementationsOnlySearchLoadedPackages(t *testing.T) {
	got := implementationsOf(t, sym("shapes", "Shape"), symbols.Options{Patterns: []string{"./shapes"}})
	want := []symbols.Symbol{sym("shapes", "Circle"), sym("shapes", "Square")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Implementations(Shape) with Patterns ./shapes: want %v, got %v", want, got)
	}
}

func TestImplementationsOfEmptyInterfaceListEveryCandidate(t *testing.T) {
	got := implementationsOf(t, sym("shapes", "Anything"), symbols.Options{})
	want := []symbols.Symbol{
		sym("shapes", "Circle"),
		sym("shapes", "Point"),
		sym("shapes", "Square"),
		sym("shapes", "Tagged"),
		sym("shapes", "Unit"),
		sym("user", "Holder"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Implementations(Anything): want %v, got %v", want, got)
	}
}

func TestImplementationsOfStandardLibraryInterface(t *testing.T) {
	// Only Tagged embeds io.Reader.
	got := implementationsOf(t, symbols.Symbol{Package: "io", Name: "Reader"}, symbols.Options{})
	want := []symbols.Symbol{sym("shapes", "Tagged")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Implementations(io.Reader): want %v, got %v", want, got)
	}
}

func TestImplementationsRejectNonInterfaces(t *testing.T) {
	root := copyFixture(t, "module")
	for _, notInterface := range []symbols.Symbol{
		sym("shapes", "Square"),
		sym("shapes", "Unit"),
		sym("shapes", "NewSquare"),
		sym("shapes", "MaxSide"),
	} {
		found, err := symbols.Implementations(t.Context(), root, notInterface, symbols.Options{})
		if !errors.Is(err, symbols.ErrNotInterface) {
			t.Errorf("Implementations(%s): want ErrNotInterface, got %v", notInterface, err)
		}
		if found != nil {
			t.Errorf("Implementations(%s): want no results with an error, got %v", notInterface, found)
		}
	}
}
