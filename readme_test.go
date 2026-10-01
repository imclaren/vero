package vero_test

import (
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The README's table names every target Go builds, one per cell, so that
// somebody looking for plan9/386 can find it by searching the page.  A new
// Go port - loong64 was one, not long ago - would otherwise go missing
// quietly, and the table is the only place that list is written down.
func TestTheReadmeNamesEveryTarget(t *testing.T) {
	out, err := exec.Command("go", "tool", "dist", "list").Output()
	if err != nil {
		t.Fatalf("asking Go for its targets: %v", err)
	}

	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("reading the README: %v", err)
	}
	// Only the table at the top: the tutorial below it says `go build` a lot,
	// and a target named in passing there is not the same promise.
	table, _, _ := strings.Cut(string(readme), "## Run the macOS example")

	named := map[string]bool{}
	for _, m := range regexp.MustCompile("`([a-z0-9]+/[a-z0-9]+)`").FindAllStringSubmatch(table, -1) {
		named[m[1]] = true
	}

	var missing []string
	for _, target := range strings.Fields(string(out)) {
		if !named[target] {
			missing = append(missing, target)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("the README's table is missing %d of Go's targets: %s\n"+
			"Go has added ports, or a row was edited.  Each one belongs in the "+
			"left column of its operating system's row, in bold if "+
			"scripts/build-all.sh ships it.", len(missing), strings.Join(missing, " "))
	}
}
