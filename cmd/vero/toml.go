package main

import (
	"os"
	"regexp"
	"strings"
)

// tomlFile is vero-app.toml read just far enough to find a section and a
// top-level or section setting, and to add sections to the end. (vero-repo
// reads it fully; this only has to not break it.)
type tomlFile struct {
	path string
	text string
}

func readToml(path string) *tomlFile {
	data, _ := os.ReadFile(path)
	return &tomlFile{path: path, text: string(data)}
}

// has says whether a section ([name], or any [name.x]) is in the file.
func (t *tomlFile) has(section string) bool {
	return regexp.MustCompile(`(?m)^\[` + regexp.QuoteMeta(section) + `(\.[^\]]*)?\]`).MatchString(t.text)
}

// get is a setting's string value in a section, "" at the top.
func (t *tomlFile) get(section, key string) string {
	lines := strings.Split(t.text, "\n")
	in := section == ""
	for _, line := range lines {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "[") {
			in = strings.TrimSuffix(strings.TrimPrefix(l, "["), "]") == section
			continue
		}
		if !in {
			continue
		}
		if m := regexp.MustCompile(`^` + regexp.QuoteMeta(key) + `\s*=\s*"([^"]*)"`).FindStringSubmatch(l); m != nil {
			return m[1]
		}
	}
	return ""
}

// write adds text to the end of the file, making the file - with the
// app's basics - when there is none.
func (t *tomlFile) write(app *App, add string) error {
	if t.text == "" {
		t.text = "# vero-app.toml describes this app to vero's packaging: see vero's\n" +
			"# PACKAGING.md, \"Creating app installers\". Paths are relative to this file.\n\n" +
			"name = \"" + app.Name + "\"\n" +
			"display_name = \"" + app.Display + "\"\n" +
			"id = \"" + app.ID + "\"\n" +
			"summary = \"\"\n" +
			"description = \"\"\n" +
			"publisher = \"Your Name <you@example.com>\"\n" +
			"homepage = \"\"\n" +
			"licence = \"\"\n" +
			"icon = \"icon.png\"\n\n" +
			"[worker]\n" +
			"package = \"" + app.Worker + "\"\n" +
			"name = \"" + app.WorkerName + "\"\n"
	}
	if !strings.HasSuffix(t.text, "\n") {
		t.text += "\n"
	}
	t.text += "\n" + strings.TrimSpace(add) + "\n"
	return os.WriteFile(t.path, []byte(t.text), 0o644)
}
