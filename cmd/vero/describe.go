package main

import (
	"os/exec"
	"strings"
)

// Description is what vero-app.toml needs that no scan of the worker can
// supply: the words about the app, and who publishes it.
type Description struct {
	Summary, Text, Publisher string
}

// describe fills in what the flags left blank, so that vero add never has
// to ask: the summary is the app's name, the description its summary, and
// the publisher whoever git says you are. It returns what it filled in,
// for vero add to say so.
func describe(d Description, display string, git func(key string) string) (Description, []string) {
	var guessed []string
	if d.Summary == "" {
		d.Summary = display
		guessed = append(guessed, "summary")
	}
	if d.Text == "" {
		d.Text = d.Summary
		guessed = append(guessed, "description")
	}
	if d.Publisher == "" {
		if name, email := git("user.name"), git("user.email"); name != "" && email != "" {
			d.Publisher = name + " <" + email + ">"
			guessed = append(guessed, "publisher")
		}
	}
	return d, guessed
}

// gitConfig is a setting from git's own configuration, or "" when there
// is none.
func gitConfig(key string) string {
	out, err := exec.Command("git", "config", "--get", key).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
