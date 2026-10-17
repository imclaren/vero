package vero_test

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

// The lab page's table names every target Go builds, one per cell, so
// that somebody looking for plan9/386 can find it by searching the page.
// A new Go port, as loong64 was not long ago, would otherwise go missing
// quietly, and the table is the only place where that list is written.
func TestTheLabNamesEveryTarget(t *testing.T) {
	out, err := exec.Command("go", "tool", "dist", "list").Output()
	if err != nil {
		t.Fatalf("asking Go for its targets: %v", err)
	}

	lab, err := os.ReadFile("docs/lab.md")
	if err != nil {
		t.Fatalf("reading the lab page: %v", err)
	}
	// Only the table of targets, which is the last section of the page.
	_, table, found := strings.Cut(string(lab), "\n## Every target that Go builds\n")
	if !found {
		t.Fatal("the lab page has no section called Every target that Go builds")
	}

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
		t.Errorf("the lab page's table of targets is missing %d of Go's targets: %s\n"+
			"Go has added ports, or a row was edited. Each one belongs in its "+
			"operating system's row of docs/lab.md, in bold if "+
			"scripts/build-all.sh builds it.", len(missing), strings.Join(missing, " "))
	}
}

// Every relative link in vero's Markdown, in Markdown or in HTML, names a
// file that is there, and every anchor names a heading in its page, so
// that moving or renaming a page cannot leave a link behind.
func TestTheDocsLinksWork(t *testing.T) {
	var pages []string
	filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && path != "." && (d.Name() == ".build" || d.Name() == "dist" || d.Name() == "node_modules" || d.Name() == ".git") {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(path, ".md") {
			pages = append(pages, path)
		}
		return nil
	})
	if len(pages) < 10 {
		t.Fatalf("found only %d pages", len(pages))
	}
	link := regexp.MustCompile(`\]\(([^)\s]+)\)|(?:href|src)="([^"]+)"`)
	for _, page := range pages {
		data, _ := os.ReadFile(page)
		// Code is not a link, whatever it looks like.
		text := regexp.MustCompile("(?s)```.*?```|`[^`\n]*`").ReplaceAllString(string(data), "")
		for _, m := range link.FindAllStringSubmatch(text, -1) {
			target := m[1] + m[2]
			if strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			file, anchor, _ := strings.Cut(target, "#")
			path := page
			if file != "" {
				path = filepath.Join(filepath.Dir(page), filepath.FromSlash(file))
				if _, err := os.Stat(path); err != nil {
					t.Errorf("%s links to %s, which is not there", page, target)
					continue
				}
			}
			if anchor != "" && !slices.Contains(anchors(path), anchor) {
				t.Errorf("%s links to %s, whose page has no such heading", page, target)
			}
		}
	}
}

// anchors are the anchors that GitHub gives a page's headings.
func anchors(page string) []string {
	data, _ := os.ReadFile(page)
	var out []string
	for _, m := range regexp.MustCompile(`(?m)^#+ (.+)$`).FindAllStringSubmatch(string(data), -1) {
		slug := strings.ToLower(m[1])
		slug = regexp.MustCompile(`[^a-z0-9 _-]`).ReplaceAllString(slug, "")
		out = append(out, strings.ReplaceAll(slug, " ", "-"))
	}
	return out
}
