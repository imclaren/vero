package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strconv"
	"testing"
)

// contraction matches the contractions that the pages and scripts vero
// writes for people to read must not use, since apps put those pages in
// front of their own readers.
var contraction = regexp.MustCompile(`(?i)\b(\w+n't|it's|there's|that's|what's|you're|we're|they're|you've|we've|you'll|we'll|i'm)\b`)

// TestWriting checks every string in site.go, which holds the install page,
// its notes and install.sh, for contractions.
func TestWriting(t *testing.T) {
	fs := token.NewFileSet()
	f, err := parser.ParseFile(fs, "site.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		s, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		for _, m := range contraction.FindAllString(s, -1) {
			t.Errorf("%s: %q is a contraction", fs.Position(lit.Pos()), m)
		}
		return true
	})
}
