package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// Description is what vero-app.toml needs that no scan of the worker can
// supply: the words about the app, and who publishes it.
type Description struct {
	Summary, Text, Publisher string
}

// describe fills in what the flags left blank by asking, when vero add is
// at a terminal and is about to write vero-app.toml for the first time.
// Anywhere else - a script, a pipe, a file that exists - it asks nothing,
// and the file keeps its blanks for the person to fill in.
func describe(d Description, creating bool, in io.Reader, out io.Writer, terminal bool) Description {
	if !creating || !terminal || (d.Summary != "" && d.Text != "" && d.Publisher != "") {
		return d
	}
	fmt.Fprintln(out, "vero-app.toml describes the app to the packaging. Three things it needs that the worker cannot say")
	fmt.Fprintln(out, "(Enter leaves one blank, to fill in later):")
	r := bufio.NewReader(in)
	ask := func(prompt, have string) string {
		if have != "" {
			return have
		}
		fmt.Fprintf(out, "  %s: ", prompt)
		line, _ := r.ReadString('\n')
		return strings.TrimSpace(line)
	}
	d.Summary = ask("summary, one line about the app", d.Summary)
	d.Text = ask("description, a sentence or two", d.Text)
	d.Publisher = ask("publisher, as Name <email>", d.Publisher)
	return d
}

// terminal says whether standard input is a person at a keyboard.
func terminal() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
