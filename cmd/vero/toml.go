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

// getBool is a setting's true or false value in a section, "" at the top.
func (t *tomlFile) getBool(section, key string) bool {
	in := section == ""
	for _, line := range strings.Split(t.text, "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "[") {
			in = strings.TrimSuffix(strings.TrimPrefix(l, "["), "]") == section
			continue
		}
		if in && regexp.MustCompile(`^`+regexp.QuoteMeta(key)+`\s*=\s*true\b`).MatchString(l) {
			return true
		}
	}
	return false
}

// setTrue adds key = true, such as menubar or tray, to the top of a file
// that does not have it, after its id, and writes it.
func (t *tomlFile) setTrue(key string) error {
	if t.text == "" || t.getBool("", key) {
		return nil
	}
	re := regexp.MustCompile(`(?m)^id\s*=.*$`)
	if loc := re.FindStringIndex(t.text); loc != nil {
		t.text = t.text[:loc[1]] + "\n" + key + " = true" + t.text[loc[1]:]
	} else {
		t.text = key + " = true\n" + t.text
	}
	return os.WriteFile(t.path, []byte(t.text), 0o644)
}

// quote is s as a TOML basic string.
func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\t", `\t`).Replace(s) + `"`
}

// write adds text to the end of the file, making the file - with the
// app's basics - when there is none.
func (t *tomlFile) write(app *App, add string) error {
	if t.text == "" {
		d := app.Description
		if d.Publisher == "" {
			d.Publisher = "Your Name <you@example.com>"
		}
		t.text = "# vero-app.toml describes this app to vero's packaging: see vero's\n" +
			"# PACKAGING.md, \"Creating app installers\". Paths are relative to this file.\n\n" +
			"name = \"" + app.Name + "\"\n" +
			"display_name = \"" + app.Display + "\"\n" +
			"id = \"" + app.ID + "\"\n" +
			menubarLine(app) +
			"summary = " + quote(d.Summary) + "\n" +
			"description = " + quote(d.Text) + "\n" +
			"publisher = " + quote(d.Publisher) + "\n" +
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

// menubarLine is the setting for an app that lives in the menu bar and
// the tray, or for a window app with a tray icon, and nothing for a plain
// window app.
func menubarLine(app *App) string {
	switch {
	case app.MenuBar:
		return "# It lives in the menu bar on the Mac and in the tray elsewhere.\nmenubar = true\n"
	case app.Tray:
		return "# A window app with an icon in the menu bar and the tray, which keeps\n# running when its window is closed.\ntray = true\n"
	}
	return ""
}
