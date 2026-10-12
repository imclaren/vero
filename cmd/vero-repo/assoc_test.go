package main

import (
	"strings"
	"testing"
)

// TestAssoc checks how link schemes and file types are registered: in the
// .desktop file, the Mac's Info.plist and the Windows installer, where a
// shared scheme such as mailto is offered and never taken over.
func TestAssoc(t *testing.T) {
	a := &App{Name: "tasks", ID: "com.example.tasks", DisplayName: "tasks", Summary: "Tasks.",
		Schemes: []string{"tasks", "mailto"}, Opens: []string{"eml", "ics"}, GTK: &GTK{}, MacOS: &MacOS{Product: "Tasks"}}
	if err := a.checkAssoc(); err != nil {
		t.Fatal(err)
	}
	entry := string(desktopEntry(a, "/usr/bin/tasks"))
	for _, want := range []string{"Exec=/usr/bin/tasks %U\n", "MimeType=x-scheme-handler/tasks;x-scheme-handler/mailto;message/rfc822;text/calendar;\n"} {
		if !strings.Contains(entry, want) {
			t.Errorf("desktop entry lacks %q:\n%s", want, entry)
		}
	}
	plist := string(infoPlist(a))
	for _, want := range []string{"<key>CFBundleURLSchemes</key>", "<string>mailto</string>", "<string>com.apple.mail.email</string>", "<string>ics</string>"} {
		if !strings.Contains(plist, want) {
			t.Errorf("Info.plist lacks %q", want)
		}
	}
	nsh := a.nsisAssoc("tasks.exe")
	for _, want := range []string{
		`WriteRegStr HKCU "Software\Classes\tasks" "URL Protocol" ""`,
		`WriteRegStr HKCU "Software\com.example.tasks\Capabilities\URLAssociations" "mailto" "com.example.tasks.mailto"`,
		`WriteRegStr HKCU "Software\Classes\.eml\OpenWithProgids" "com.example.tasks.file" ""`,
		`WriteRegStr HKCU "Software\RegisteredApplications" "tasks" "Software\com.example.tasks\Capabilities"`,
		`"$\"$INSTDIR\tasks.exe$\" $\"%1$\""`,
		`DeleteRegValue HKCU "Software\Classes\.eml\OpenWithProgids" "com.example.tasks.file"`,
	} {
		if !strings.Contains(nsh, want) {
			t.Errorf("the installer's entries lack %q:\n%s", want, nsh)
		}
	}
	// mailto is offered, never taken over: no Software\Classes\mailto.
	if strings.Contains(nsh, `Software\Classes\mailto"`) {
		t.Error("the installer takes over mailto")
	}
	for _, bad := range []*App{{Schemes: []string{"Tasks:"}}, {Opens: []string{".eml"}}, {Opens: []string{"zzzq"}}} {
		if err := bad.checkAssoc(); err == nil {
			t.Errorf("%+v was accepted", bad)
		}
	}
	// An app that opens nothing keeps its entry as it was.
	plain := &App{Name: "x", ID: "a.b", GTK: &GTK{}}
	if e := string(desktopEntry(plain, "/usr/bin/x")); strings.Contains(e, "MimeType") || strings.Contains(e, "%U") {
		t.Errorf("entry:\n%s", e)
	}
}
