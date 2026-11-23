package mergeinfo_test

import (
	"strings"
	"testing"

	"github.com/AleksanderMako/machanised-vibe-coding/mergeinfo"
)

// declSource has a declaration of every kind rule 9 names; the comment after each line is its line number.
var declSource = strings.Join([]string{
	"// Package p exercises EnclosingDecl.", // 1
	"package p",                             // 2
	"",                                      // 3
	"import (",                              // 4
	"\t\"fmt\"",                             // 5
	")",                                     // 6
	"",                                      // 7
	"// Page is one page of results.",       // 8
	"// It spans two comment lines.",        // 9
	"type Page struct {",                    // 10
	"\tSize int",                            // 11
	"}",                                     // 12
	"",                                      // 13
	"// Close releases the page.",           // 14
	"func (c *Consumer) Close() error {",    // 15
	"\treturn nil",                          // 16
	"}",                                     // 17
	"",                                      // 18
	"func (Page) Zero() int { return 0 }",   // 19
	"",                                      // 20
	"func (p Page) Half() int {",            // 21
	"\treturn p.Size / 2",                   // 22
	"}",                                     // 23
	"",                                      // 24
	"var x = 1",                             // 25
	"",                                      // 26
	"const subCampaignGroupsPerPage = 50",   // 27
	"",                                      // 28
	"var (",                                 // 29
	"\tfirst  = 1",                          // 30
	"\tsecond = 2",                          // 31
	")",                                     // 32
	"",                                      // 33
	"const (",                               // 34
	"\ta = 1",                               // 35
	"\tb = 2",                               // 36
	"\tc = 3",                               // 37
	")",                                     // 38
	"",                                      // 39
	"var (",                                 // 40
	"\tonly = 3",                            // 41
	")",                                     // 42
	"",                                      // 43
	"// loose comment that documents nothing", // 44
	"",                         // 45
	"// Hello prints.",         // 46
	"func Hello() {",           // 47
	"\tfmt.Println(\"hello\")", // 48
	"}",                        // 49
	"",                         // 50
	"type Consumer struct{}",   // 51
}, "\n") + "\n"

func TestRule8_9_EnclosingDeclNamesEachKindOfDeclaration(t *testing.T) {
	t.Parallel()
	cases := []struct {
		line int
		want string
	}{
		{1, ""},
		{2, ""},
		{3, ""},
		{4, "import"},
		{5, "import"},
		{6, "import"},
		{7, ""},
		{8, "type Page"},
		{9, "type Page"},
		{10, "type Page"},
		{11, "type Page"},
		{12, "type Page"},
		{13, ""},
		{14, "func (c *Consumer) Close"},
		{15, "func (c *Consumer) Close"},
		{16, "func (c *Consumer) Close"},
		{17, "func (c *Consumer) Close"},
		{18, ""},
		{19, "func (Page) Zero"},
		{20, ""},
		{21, "func (p Page) Half"},
		{22, "func (p Page) Half"},
		{23, "func (p Page) Half"},
		{25, "var x"},
		{27, "const subCampaignGroupsPerPage"},
		{29, "var first..."},
		{31, "var first..."},
		{32, "var first..."},
		{34, "const a..."},
		{37, "const a..."},
		{38, "const a..."},
		{39, ""},
		{40, "var only"},
		{41, "var only"},
		{42, "var only"},
		{44, ""},
		{45, ""},
		{46, "func Hello"},
		{48, "func Hello"},
		{49, "func Hello"},
		{50, ""},
		{51, "type Consumer"},
	}
	for _, testCase := range cases {
		got, err := mergeinfo.EnclosingDecl([]byte(declSource), testCase.line)
		if err != nil {
			t.Fatalf("line %d: EnclosingDecl: %v", testCase.line, err)
		}
		if got != testCase.want {
			t.Errorf("line %d: want %q, got %q", testCase.line, testCase.want, got)
		}
	}
}

func TestRule9_SingleImportWithoutParentheses(t *testing.T) {
	t.Parallel()
	source := "package p\n\nimport \"fmt\"\n\nvar _ = fmt.Sprint\n"
	got, err := mergeinfo.EnclosingDecl([]byte(source), 3)
	if err != nil {
		t.Fatalf("EnclosingDecl: %v", err)
	}
	if got != "import" {
		t.Errorf("want %q, got %q", "import", got)
	}
}

func TestRule9_PlainFunctionOnItsOnlyLine(t *testing.T) {
	t.Parallel()
	source := "package p\nfunc Greet() {}\n"
	got, err := mergeinfo.EnclosingDecl([]byte(source), 2)
	if err != nil {
		t.Fatalf("EnclosingDecl: %v", err)
	}
	if got != "func Greet" {
		t.Errorf("want %q, got %q", "func Greet", got)
	}
}

func TestRule8_ParseErrorIsReturned(t *testing.T) {
	t.Parallel()
	source := "package p\n\nfunc Broken( {\n"
	if _, err := mergeinfo.EnclosingDecl([]byte(source), 3); err == nil {
		t.Error("unparsable source: want an error, got nil")
	}
}
