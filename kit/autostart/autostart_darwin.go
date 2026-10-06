//go:build darwin && !ios

package autostart

import (
	"encoding/xml"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

// A LaunchAgent, which launchd starts at sign-in. It is loaded now as well,
// so that Enable takes effect without a sign-out, and unloaded on Disable.
func path(a App) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", a.ID+".plist"), nil
}

type plist struct {
	XMLName xml.Name `xml:"plist"`
	Version string   `xml:"version,attr"`
	Dict    dict     `xml:"dict"`
}

type dict struct {
	Items []any
}

func (d dict) MarshalXML(e *xml.Encoder, start xml.StartElement) error {
	if err := e.EncodeToken(start); err != nil {
		return err
	}
	for _, it := range d.Items {
		if err := e.Encode(it); err != nil {
			return err
		}
	}
	return e.EncodeToken(start.End())
}

type key string
type str string
type array []string
type boolean bool

func (k key) MarshalXML(e *xml.Encoder, _ xml.StartElement) error {
	return e.EncodeElement(string(k), xml.StartElement{Name: xml.Name{Local: "key"}})
}
func (s str) MarshalXML(e *xml.Encoder, _ xml.StartElement) error {
	return e.EncodeElement(string(s), xml.StartElement{Name: xml.Name{Local: "string"}})
}
func (a array) MarshalXML(e *xml.Encoder, _ xml.StartElement) error {
	start := xml.StartElement{Name: xml.Name{Local: "array"}}
	if err := e.EncodeToken(start); err != nil {
		return err
	}
	for _, s := range a {
		if err := str(s).MarshalXML(e, xml.StartElement{}); err != nil {
			return err
		}
	}
	return e.EncodeToken(start.End())
}
func (b boolean) MarshalXML(e *xml.Encoder, _ xml.StartElement) error {
	name := "false"
	if b {
		name = "true"
	}
	start := xml.StartElement{Name: xml.Name{Local: name}}
	if err := e.EncodeToken(start); err != nil {
		return err
	}
	return e.EncodeToken(start.End())
}

// agent is the LaunchAgent's plist for a.
func agent(a App) ([]byte, error) {
	exec := a.Exec
	// An app bundle is opened, so that macOS treats it as the app.
	if filepath.Ext(exec[0]) == ".app" {
		exec = append([]string{"/usr/bin/open", "-a", exec[0]}, exec[1:]...)
	}
	p := plist{Version: "1.0", Dict: dict{Items: []any{
		key("Label"), str(a.ID),
		key("ProgramArguments"), array(exec),
		key("RunAtLoad"), boolean(true),
	}}}
	out, err := xml.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}
	head := []byte(xml.Header + `<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	return append(head, append(out, '\n')...), nil
}

func enable(a App) error {
	p, err := path(a)
	if err != nil {
		return err
	}
	data, err := agent(a)
	if err != nil {
		return err
	}
	if err := writeFile(p, data, 0o644); err != nil {
		return err
	}
	exec.Command("launchctl", "load", "-w", p).Run() // best effort: starts it now
	return nil
}

func disable(a App) error {
	p, err := path(a)
	if err != nil {
		return err
	}
	exec.Command("launchctl", "unload", "-w", p).Run()
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func enabled(a App) (bool, error) {
	p, err := path(a)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(p)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}
