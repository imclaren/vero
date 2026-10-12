package main

import (
	"fmt"
	"mime"
	"regexp"
	"strings"
)

// Link schemes and file types: vero-app.toml's schemes and opens.
//
//	schemes = ["calmtasks"]          links such as calmtasks://task/1
//	opens = ["eml", "ics", "vcf"]    files, by extension
//
// vero registers them as each system does: MimeType= in the .desktop file
// on Linux, the BSDs and illumos; the registry, from the Windows
// installer; and, for a Mac app that vero makes, its Info.plist. What the
// app is asked to open reaches it through vero's libraries, whose single
// copy passes it on to the copy already running.
//
// On Windows an app never makes itself the default for a scheme that
// others handle too, such as mailto, nor for a file type: that is the
// person's choice. It registers as able to open them, which puts it in
// Windows' Default Apps and in Open with. Only a scheme of its own, such
// as calmtasks, it registers outright.

var schemeName = regexp.MustCompile(`^[a-z][a-z0-9+.-]*$`)
var extName = regexp.MustCompile(`^[a-z0-9]+$`)

// sharedSchemes are schemes that many apps handle, for which the person
// chooses the default.
var sharedSchemes = map[string]bool{"mailto": true, "http": true, "https": true, "webcal": true, "webcals": true,
	"tel": true, "sms": true, "ftp": true, "file": true, "irc": true, "magnet": true, "geo": true}

// knownTypes are the MIME types and Apple's types of extensions that Go's
// own table may not have.
var knownTypes = map[string][2]string{
	"eml":  {"message/rfc822", "com.apple.mail.email"},
	"ics":  {"text/calendar", "com.apple.ical.ics"},
	"vcf":  {"text/vcard", "public.vcard"},
	"pdf":  {"application/pdf", "com.adobe.pdf"},
	"txt":  {"text/plain", "public.plain-text"},
	"md":   {"text/markdown", "net.daringfireball.markdown"},
	"csv":  {"text/csv", "public.comma-separated-values-text"},
	"json": {"application/json", "public.json"},
	"zip":  {"application/zip", "public.zip-archive"},
	"mp3":  {"audio/mpeg", "public.mp3"},
	"m4b":  {"audio/mp4", "com.apple.protected-mpeg-4-audio-b"},
	"png":  {"image/png", "public.png"},
	"jpg":  {"image/jpeg", "public.jpeg"},
}

// checkAssoc checks schemes and opens.
func (a *App) checkAssoc() error {
	for _, s := range a.Schemes {
		if !schemeName.MatchString(s) {
			return fmt.Errorf("schemes: %q is not a scheme: lower case letters, then letters, digits, +, . or -", s)
		}
	}
	for _, e := range a.Opens {
		if !extName.MatchString(e) {
			return fmt.Errorf("opens: %q is not an extension: lower case letters and digits, without a dot", e)
		}
		if mimeOf(e) == "" {
			return fmt.Errorf("opens: vero does not know the type of .%s files", e)
		}
	}
	return nil
}

func mimeOf(ext string) string {
	if t, ok := knownTypes[ext]; ok {
		return t[0]
	}
	t := mime.TypeByExtension("." + ext)
	if i := strings.Index(t, ";"); i >= 0 {
		t = t[:i]
	}
	return t
}

// mimeTypes are what the .desktop file's MimeType= lists: each scheme as
// x-scheme-handler, and each file type.
func (a *App) mimeTypes() []string {
	var out []string
	for _, s := range a.Schemes {
		out = append(out, "x-scheme-handler/"+s)
	}
	for _, e := range a.Opens {
		out = append(out, mimeOf(e))
	}
	return out
}

// plistAssoc is the Info.plist's CFBundleURLTypes and CFBundleDocumentTypes.
func (a *App) plistAssoc() string {
	var b strings.Builder
	if len(a.Schemes) > 0 {
		b.WriteString("\t<key>CFBundleURLTypes</key>\n\t<array>\n\t\t<dict>\n")
		fmt.Fprintf(&b, "\t\t\t<key>CFBundleURLName</key>\n\t\t\t<string>%s</string>\n", xmlText(a.ID))
		b.WriteString("\t\t\t<key>CFBundleURLSchemes</key>\n\t\t\t<array>\n")
		for _, s := range a.Schemes {
			fmt.Fprintf(&b, "\t\t\t\t<string>%s</string>\n", s)
		}
		b.WriteString("\t\t\t</array>\n\t\t</dict>\n\t</array>\n")
	}
	if len(a.Opens) > 0 {
		b.WriteString("\t<key>CFBundleDocumentTypes</key>\n\t<array>\n")
		for _, e := range a.Opens {
			b.WriteString("\t\t<dict>\n")
			fmt.Fprintf(&b, "\t\t\t<key>CFBundleTypeName</key>\n\t\t\t<string>%s file</string>\n", strings.ToUpper(e))
			b.WriteString("\t\t\t<key>CFBundleTypeRole</key>\n\t\t\t<string>Viewer</string>\n")
			if t, ok := knownTypes[e]; ok {
				fmt.Fprintf(&b, "\t\t\t<key>LSItemContentTypes</key>\n\t\t\t<array>\n\t\t\t\t<string>%s</string>\n\t\t\t</array>\n", t[1])
			}
			fmt.Fprintf(&b, "\t\t\t<key>CFBundleTypeExtensions</key>\n\t\t\t<array>\n\t\t\t\t<string>%s</string>\n\t\t\t</array>\n", e)
			b.WriteString("\t\t</dict>\n")
		}
		b.WriteString("\t</array>\n")
	}
	return b.String()
}

// nsisAssoc is the Windows installer's registry entries for schemes and
// opens, as two NSIS macros: VeroAssociate, which the install runs, and
// VeroDissociate, which the uninstall runs. exe is the app's program.
func (a *App) nsisAssoc(exe string) string {
	q := func(s string) string { return strings.ReplaceAll(s, `"`, `$\"`) }
	caps := `Software\` + a.ID + `\Capabilities`
	command := `"$\"$INSTDIR\` + q(exe) + `$\" $\"%1$\""`
	var in, out strings.Builder
	reg := func(key, name, value string) {
		fmt.Fprintf(&in, "  WriteRegStr HKCU \"%s\" \"%s\" %s\n", key, name, value)
	}
	quoted := func(s string) string { return `"` + q(s) + `"` }
	progID := func(kind string) string { return a.ID + "." + kind }
	handler := func(id, description string) {
		key := `Software\Classes\` + id
		reg(key, "", quoted(description))
		reg(key+`\DefaultIcon`, "", `"$INSTDIR\`+q(exe)+`,0"`)
		reg(key+`\shell\open\command`, "", command)
		fmt.Fprintf(&out, "  DeleteRegKey HKCU \"%s\"\n", key)
	}
	reg(caps, "ApplicationName", quoted(a.DisplayName))
	reg(caps, "ApplicationDescription", quoted(a.Summary))
	for _, s := range a.Schemes {
		if sharedSchemes[s] {
			handler(progID(s), a.DisplayName+" "+s+" link")
		} else {
			// The app's own scheme, registered outright.
			key := `Software\Classes\` + s
			reg(key, "", quoted("URL:"+a.DisplayName+" link"))
			reg(key, "URL Protocol", `""`)
			reg(key+`\DefaultIcon`, "", `"$INSTDIR\`+q(exe)+`,0"`)
			reg(key+`\shell\open\command`, "", command)
			fmt.Fprintf(&out, "  DeleteRegKey HKCU \"%s\"\n", key)
			handler(progID(s), a.DisplayName+" "+s+" link")
		}
		reg(caps+`\URLAssociations`, s, quoted(progID(s)))
	}
	if len(a.Opens) > 0 {
		handler(progID("file"), a.DisplayName+" file")
		for _, e := range a.Opens {
			reg(`Software\Classes\.`+e+`\OpenWithProgids`, progID("file"), `""`)
			reg(caps+`\FileAssociations`, "."+e, quoted(progID("file")))
			fmt.Fprintf(&out, "  DeleteRegValue HKCU \"Software\\Classes\\.%s\\OpenWithProgids\" \"%s\"\n", e, progID("file"))
		}
	}
	reg(`Software\RegisteredApplications`, a.DisplayName, quoted(caps))
	fmt.Fprintf(&out, "  DeleteRegKey HKCU \"Software\\%s\"\n", a.ID)
	fmt.Fprintf(&out, "  DeleteRegValue HKCU \"Software\\RegisteredApplications\" \"%s\"\n", q(a.DisplayName))
	// Windows reads the associations again.
	notify := "  System::Call 'shell32::SHChangeNotify(i 0x08000000, i 0, p 0, p 0)'\n"
	return "; Written by vero-repo: the app's link schemes and file types.\n" +
		"!macro VeroAssociate\n" + in.String() + notify + "!macroend\n\n" +
		"!macro VeroDissociate\n" + out.String() + notify + "!macroend\n"
}
