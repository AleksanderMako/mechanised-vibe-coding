package refactor_test

import (
	"context"
	"errors"
	"testing"

	"github.com/AleksanderMako/machanised-vibe-coding/refactor"
	"github.com/AleksanderMako/machanised-vibe-coding/symbols"
)

func TestRule17To20_DemethodizeNamedUnusedReceiver(t *testing.T) {
	root := copyFixture(t, "demethodize")
	before := readTree(t, root)

	written, err := refactor.Demethodize(context.Background(), root, sym("calc", "Adder.Sum"), "", refactor.Options{})
	if err != nil {
		t.Fatalf("Demethodize: %v", err)
	}

	assertWritten(t, root, before, written, []string{"calc/calc.go", "calc/calc_test.go", "client/client.go"}, nil)
	assertModuleBuilds(t, root)
	assertContains(t, root, "calc/calc.go",
		"// Sum adds two numbers without using its named receiver.\nfunc Sum(x, y int) int {",
		"return Sum(1, 2) + adder.Double(1)",
	)
	assertNotContains(t, root, "calc/calc.go", "(a Adder) Sum")
	assertContains(t, root, "calc/calc_test.go", "if Sum(2, 2) != 4")
	// Identifier, pointer, selector of an identifier and dereference receivers are all dropped.
	assertContains(t, root, "client/client.go",
		"sum := calc.Sum(1, 2) + calc.Sum(3, 4) + calc.Sum(5, 6) + calc.Sum(7, 8)",
	)
}

func TestRule17_DemethodizeUnnamedReceiver(t *testing.T) {
	root := copyFixture(t, "demethodize")
	before := readTree(t, root)

	written, err := refactor.Demethodize(context.Background(), root, sym("calc", "Adder.Double"), "", refactor.Options{})
	if err != nil {
		t.Fatalf("Demethodize: %v", err)
	}

	assertWritten(t, root, before, written, []string{"calc/calc.go", "client/client.go"}, nil)
	assertModuleBuilds(t, root)
	assertContains(t, root, "calc/calc.go", "func Double(x int) int {", "return adder.Sum(1, 2) + Double(1)")
	assertNotContains(t, root, "calc/calc.go", "(Adder) Double")
	assertContains(t, root, "client/client.go", "other := calc.Double(2) + pointer.Triple(3)")
}

func TestRule17_DemethodizeBlankPointerReceiver(t *testing.T) {
	root := copyFixture(t, "demethodize")
	before := readTree(t, root)

	written, err := refactor.Demethodize(context.Background(), root, sym("calc", "Adder.Triple"), "", refactor.Options{NoBuild: true})
	if err != nil {
		t.Fatalf("Demethodize: %v", err)
	}

	assertWritten(t, root, before, written, []string{"calc/calc.go", "client/client.go"}, nil)
	assertModuleBuilds(t, root)
	assertContains(t, root, "calc/calc.go", "// Triple has a blank pointer receiver.\nfunc Triple(x int) int {")
	assertNotContains(t, root, "calc/calc.go", "(_ *Adder)")
	assertContains(t, root, "client/client.go", "adder.Double(2) + calc.Triple(3) + calc.Triple(4)")
}

func TestRule18And4_DemethodizeWithExplicitName(t *testing.T) {
	root := copyFixture(t, "demethodize")
	before := readTree(t, root)

	written, err := refactor.Demethodize(context.Background(), root, sym("calc", "Adder.Sum"), "Add", refactor.Options{})
	if err != nil {
		t.Fatalf("Demethodize: %v", err)
	}

	assertWritten(t, root, before, written, []string{"calc/calc.go", "calc/calc_test.go", "client/client.go"}, nil)
	assertModuleBuilds(t, root)
	assertContains(t, root, "calc/calc.go",
		"// Add adds two numbers without using its named receiver.\nfunc Add(x, y int) int {",
		"return Add(1, 2) + adder.Double(1)",
	)
	assertContains(t, root, "calc/calc_test.go", "if Add(2, 2) != 4")
	assertContains(t, root, "client/client.go", "sum := calc.Add(1, 2) + calc.Add(3, 4) + calc.Add(5, 6) + calc.Add(7, 8)")
	for _, file := range []string{"calc/calc.go", "calc/calc_test.go", "client/client.go"} {
		if count := identifierCount(t, root, file, "Sum"); count != 0 {
			t.Errorf("%s still has %d identifier(s) named Sum", file, count)
		}
	}
}

func TestRule23_DemethodizeExportsNameCalledFromAnotherPackage(t *testing.T) {
	root := copyFixture(t, "demethodize")
	before := readTree(t, root)

	written, err := refactor.Demethodize(context.Background(), root, sym("calc", "Adder.Sum"), "add", refactor.Options{})
	if err != nil {
		t.Fatalf("Demethodize: %v", err)
	}

	assertWritten(t, root, before, written, []string{"calc/calc.go", "calc/calc_test.go", "client/client.go"}, nil)
	assertModuleBuilds(t, root)
	assertContains(t, root, "calc/calc.go", "func Add(x, y int) int {", "return Add(1, 2) + adder.Double(1)")
	assertNotContains(t, root, "calc/calc.go", "func add(")
	assertContains(t, root, "calc/calc_test.go", "if Add(2, 2) != 4")
	assertContains(t, root, "client/client.go", "calc.Add(1, 2)")
}

func TestRule22And18_DemethodizeKeepsUnexportedNameAndIgnoresUnusedInterface(t *testing.T) {
	root := copyFixture(t, "demethodize")
	before := readTree(t, root)

	written, err := refactor.Demethodize(context.Background(), root, sym("calc", "Adder.Square"), "square", refactor.Options{})
	if err != nil {
		t.Fatalf("Demethodize: %v", err)
	}

	assertWritten(t, root, before, written, []string{"calc/calc.go", "calc/calc_test.go"}, nil)
	assertModuleBuilds(t, root)
	assertContains(t, root, "calc/calc.go",
		"func square(x int) int {",
		"type Squarer interface { Square(x int) int }",
	)
	assertContains(t, root, "calc/calc_test.go", "if square(3) != 9")
}

func TestRule17_DemethodizeRejectsUsedReceiver(t *testing.T) {
	root := copyFixture(t, "demethodize")
	before := readTree(t, root)

	_, err := refactor.Demethodize(context.Background(), root, sym("calc", "Adder.Label"), "", refactor.Options{})

	assertErrorIs(t, err, refactor.ErrReceiverUsed)
	assertTreeUnchanged(t, root, before)
}

func TestRule22_DemethodizeRejectsMethodOfInterfaceUsedAsType(t *testing.T) {
	root := copyFixture(t, "demethodize")
	before := readTree(t, root)

	_, err := refactor.Demethodize(context.Background(), root, sym("calc", "Adder.Negate"), "", refactor.Options{})

	assertErrorIs(t, err, refactor.ErrBreaksInterface)
	assertTreeUnchanged(t, root, before)
}

func TestRule18_DemethodizeConflicts(t *testing.T) {
	cases := []struct {
		name    string
		method  string
		newName string
	}{
		{"default name taken by a function", "Adder.Clamp", ""},
		{"explicit name taken by a function", "Adder.Sum", "Clamp"},
		{"explicit name taken by a type", "Adder.Sum", "Adder"},
		{"explicit name taken by another function", "Adder.Sum", "Total"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := copyFixture(t, "demethodize")
			before := readTree(t, root)

			_, err := refactor.Demethodize(context.Background(), root, sym("calc", testCase.method), testCase.newName, refactor.Options{})

			assertErrorIs(t, err, refactor.ErrConflict)
			assertTreeUnchanged(t, root, before)
		})
	}
}

func TestRule18_DemethodizeRejectsInvalidName(t *testing.T) {
	for _, badName := range []string{"1sum", "two words", "func"} {
		t.Run(badName, func(t *testing.T) {
			root := copyFixture(t, "demethodize")
			before := readTree(t, root)

			_, err := refactor.Demethodize(context.Background(), root, sym("calc", "Adder.Sum"), badName, refactor.Options{})

			if err == nil {
				t.Fatal("want an error, got nil")
			}
			assertTreeUnchanged(t, root, before)
		})
	}
}

func TestRule17_DemethodizeRequiresAMethod(t *testing.T) {
	notMethods := []symbols.Symbol{sym("calc", "Clamp"), sym("calc", "Adder"), sym("calc", "Adder.Name")}
	for _, notMethod := range notMethods {
		t.Run(notMethod.Name, func(t *testing.T) {
			root := copyFixture(t, "demethodize")
			before := readTree(t, root)

			_, err := refactor.Demethodize(context.Background(), root, notMethod, "Renamed", refactor.Options{})

			if err == nil {
				t.Fatal("want an error, got nil")
			}
			assertTreeUnchanged(t, root, before)
		})
	}
}

// clientHeader starts an injected file in package client.
const clientHeader = "package client\n\nimport \"example.com/fixture/calc\"\n\n"

func TestRule20And21_DemethodizeRejectsUsesThatCannotBeRewritten(t *testing.T) {
	cases := []struct {
		name    string
		file    string
		content string
	}{
		{"call as receiver", "client/extra.go", clientHeader +
			"func makeAdder() calc.Adder { return calc.Adder{} }\n\nfunc extra() int { return makeAdder().Sum(1, 2) }\n"},
		{"channel receive as receiver", "client/extra.go", clientHeader +
			"func extra(ch chan calc.Adder) int { return (<-ch).Sum(1, 2) }\n"},
		{"slice index as receiver", "client/extra.go", clientHeader +
			"func extra(list []calc.Adder) int { return list[0].Sum(1, 2) }\n"},
		{"map index as receiver", "client/extra.go", clientHeader +
			"func extra(byName map[string]calc.Adder) int { return byName[\"a\"].Sum(1, 2) }\n"},
		{"method value in another package", "client/extra.go", clientHeader +
			"func extra() func(int, int) int {\n\tvar adder calc.Adder\n\treturn adder.Sum\n}\n"},
		{"method value in the same package", "calc/extra.go",
			"package calc\n\nfunc extra() func(int, int) int {\n\tvar adder Adder\n\treturn adder.Sum\n}\n"},
		{"method expression", "client/extra.go", clientHeader +
			"func extra() func(calc.Adder, int, int) int { return calc.Adder.Sum }\n"},
		{"pointer method expression", "client/extra.go", clientHeader +
			"func extra() func(*calc.Adder, int, int) int { return (*calc.Adder).Sum }\n"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := copyFixture(t, "demethodize")
			writeFile(t, root, testCase.file, testCase.content)
			before := readTree(t, root)

			_, err := refactor.Demethodize(context.Background(), root, sym("calc", "Adder.Sum"), "", refactor.Options{})

			if !errors.Is(err, refactor.ErrUnsupportedUse) {
				t.Fatalf("want error wrapping %q, got %v", refactor.ErrUnsupportedUse, err)
			}
			assertTreeUnchanged(t, root, before)
		})
	}
}
