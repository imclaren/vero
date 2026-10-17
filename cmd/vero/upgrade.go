package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// upgrade brings an app made with vero 0.13 or later up to the vero this
// tool is, or to --version: go.mod and the kit, the Swift package's pin,
// the Windows project, vero's files copied into each front end, the guide
// for coding agents, the request
// types vero gen writes, and vero-app.toml's dependencies in the form that
// works them out. It says what it changed, and where the app's own code
// must change, which it never does itself.
func upgrade(args []string) error {
	fs := flag.NewFlagSet("upgrade", flag.ExitOnError)
	dir := fs.String("dir", ".", "the app's folder")
	version := fs.String("version", "", "the vero to upgrade to, such as v0.18.0; this tool's own version when empty")
	force := fs.Bool("force", false, "replace vero's files even where they have been edited")
	fs.Parse(args)
	root, err := filepath.Abs(*dir)
	if err != nil {
		return err
	}
	u := &upgrader{dir: root, force: *force}
	if err := u.goModule(*version); err != nil {
		return err
	}
	app, err := analyseWith(root, true)
	if err != nil {
		return err
	}
	u.app = app
	steps := []func() error{u.copyFiles, u.agents, u.windowsProject, u.swiftPin, u.toml, u.gen, u.xcodeFile, u.clashes}
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	if len(u.edited) > 0 {
		return fmt.Errorf("these files of vero's have been changed in this app, so they were left alone:\n  %s\n"+
			"An app changes what vero's libraries do through their settings, or by subclassing, and keeps its own rules for agents in a file of its own, such as CLAUDE.md, rather than in AGENTS.md. Move the change there, then run vero upgrade again, or run it with --force to replace them",
			strings.Join(u.edited, "\n  "))
	}
	fmt.Println(u.report())
	return nil
}

type upgrader struct {
	dir     string
	app     *App
	force   bool
	changed []string
	todo    []string
	edited  []string
	from    string
}

func (u *upgrader) did(format string, a ...any) {
	u.changed = append(u.changed, fmt.Sprintf(format, a...))
}
func (u *upgrader) must(format string, a ...any) { u.todo = append(u.todo, fmt.Sprintf(format, a...)) }

func (u *upgrader) report() string {
	var b strings.Builder
	if len(u.changed) == 0 {
		b.WriteString("Nothing needed changing.\n")
	} else {
		b.WriteString("Changed:\n")
		for _, c := range u.changed {
			b.WriteString("  - " + c + "\n")
		}
	}
	if len(u.todo) > 0 {
		b.WriteString("\nTo do in the app's own code:\n")
		for _, t := range u.todo {
			b.WriteString("  - " + t + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// goModule moves go.mod to the vero asked for, and the kit with it, unless
// go.mod replaces vero with a checkout, which is used as it is.
func (u *upgrader) goModule(version string) error {
	mod, err := os.ReadFile(filepath.Join(u.dir, "go.mod"))
	if err != nil {
		return fmt.Errorf("run this in your app's folder, the one with its go.mod: %v", err)
	}
	have := func(path string) string {
		m := regexp.MustCompile(`(?m)^\s*(?:require\s+)?` + regexp.QuoteMeta(path) + `\s+(v\S+)`).FindSubmatch(mod)
		if m == nil {
			return ""
		}
		return string(m[1])
	}
	u.from = have("github.com/imclaren/vero")
	if u.from == "" {
		return fmt.Errorf("go.mod does not require github.com/imclaren/vero, so this is not a vero app")
	}
	if regexp.MustCompile(`(?m)^\s*(?:replace\s+)?github\.com/imclaren/vero\s+=>`).Match(mod) {
		u.did("go.mod replaces vero with a checkout, which is used as it is")
		return nil
	}
	if version == "" {
		version = toolVersion()
	}
	if version == "" {
		version = "latest"
	}
	modules := []string{"github.com/imclaren/vero@" + version}
	if have("github.com/imclaren/vero/kit") != "" {
		modules = append(modules, "github.com/imclaren/vero/kit@"+version)
	}
	if err := run(u.dir, "go", append([]string{"get"}, modules...)...); err != nil {
		return err
	}
	if err := run(u.dir, "go", "mod", "tidy"); err != nil {
		return err
	}
	mod, _ = os.ReadFile(filepath.Join(u.dir, "go.mod"))
	if now := have("github.com/imclaren/vero"); now != u.from {
		u.did("go.mod: vero %s to %s", u.from, now)
	}
	return nil
}

// The files vero copies into each front end, and records, so that an edit
// to one is noticed before it is replaced.
const recordName = ".vero-files.json"

func (u *upgrader) record() map[string]string { return readRecord(u.dir) }

func (u *upgrader) saveRecord(rec map[string]string) error { return writeRecord(u.dir, rec) }

// readRecord is the app's record of vero's files: each one's path, and the
// hash of what vero last wrote there.
func readRecord(dir string) map[string]string {
	rec := map[string]string{}
	data, err := os.ReadFile(filepath.Join(dir, recordName))
	if err == nil {
		json.Unmarshal(data, &rec)
	}
	return rec
}

func writeRecord(dir string, rec map[string]string) error {
	data, _ := json.MarshalIndent(rec, "", "  ")
	return os.WriteFile(filepath.Join(dir, recordName), append(data, '\n'), 0o644)
}

func hashOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// veroFiles are vero's files for each front end the app has: where each
// goes in the app, and where it is in vero.
func (u *upgrader) veroFiles() map[string]string {
	out := map[string]string{}
	if folder := u.app.toml.get("gtk", "folder"); folder != "" {
		out[filepath.Join(folder, "vero.py")] = "bindings/python/vero.py"
		out[filepath.Join(folder, "vero_gtk.py")] = "bindings/python/vero_gtk.py"
	}
	if folder := u.app.toml.get("wpf", "folder"); folder != "" {
		out[filepath.Join(folder, "Vero.cs")] = "bindings/csharp/Vero.cs"
		out[filepath.Join(folder, "VeroWpf.cs")] = "bindings/csharp/VeroWpf.cs"
		// The web view's pieces, for an app that uses WebView2.
		if proj := u.csproj(); proj != "" {
			if data, _ := os.ReadFile(proj); bytes.Contains(data, []byte("Microsoft.Web.WebView2")) {
				out[filepath.Join(folder, "VeroWebView.cs")] = "bindings/csharp/VeroWebView.cs"
			}
		}
	}
	return out
}

// copyFiles copies vero's files afresh from the vero go.mod now requires,
// except a file that has been changed since vero last wrote it.
func (u *upgrader) copyFiles() error {
	src, err := output(u.dir, "go", "list", "-m", "-f", "{{.Dir}}", "github.com/imclaren/vero")
	if err != nil {
		return err
	}
	src = strings.TrimSpace(src)
	rec := u.record()
	files := u.veroFiles()
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		fresh, err := os.ReadFile(filepath.Join(src, filepath.FromSlash(files[rel])))
		if err != nil {
			return err
		}
		path, key := filepath.Join(u.dir, rel), filepath.ToSlash(rel)
		old, err := os.ReadFile(path)
		existed, recorded := err == nil, rec[key]
		switch {
		case existed && bytes.Equal(old, fresh):
			rec[key] = hashOf(fresh)
			continue
		case existed && recorded != "" && recorded != hashOf(old) && !u.force:
			u.edited = append(u.edited, rel)
			continue
		}
		if err := os.WriteFile(path, fresh, 0o644); err != nil {
			return err
		}
		rec[key] = hashOf(fresh)
		switch {
		case !existed:
			u.did("%s: added", rel)
		case recorded == "":
			u.did("%s: replaced; vero had no record of its copy, so look at git diff for changes of the app's own", rel)
		default:
			u.did("%s: updated", rel)
		}
	}
	return u.saveRecord(rec)
}

func (u *upgrader) csproj() string {
	folder := u.app.toml.get("wpf", "folder")
	if folder == "" {
		return ""
	}
	matches, _ := filepath.Glob(filepath.Join(u.dir, folder, "*.csproj"))
	if len(matches) == 0 {
		return ""
	}
	return matches[0]
}

// windowsProject moves the Windows app to .NET 10, with Windows Forms for
// the tray icon, which VeroWpf.cs needs.
func (u *upgrader) windowsProject() error {
	proj := u.csproj()
	if proj == "" {
		return nil
	}
	data, err := os.ReadFile(proj)
	if err != nil {
		return err
	}
	text := string(data)
	rel, _ := filepath.Rel(u.dir, proj)
	re := regexp.MustCompile(`<TargetFramework>net(\d+)\.0-windows([^<]*)</TargetFramework>`)
	if m := re.FindStringSubmatch(text); m != nil && m[1] != "10" {
		text = re.ReplaceAllString(text, "<TargetFramework>net10.0-windows$2</TargetFramework>")
		u.did("%s: .NET %s to .NET 10", rel, m[1])
	}
	if !strings.Contains(text, "<UseWindowsForms>") {
		text = strings.Replace(text, "<UseWPF>true</UseWPF>", "<UseWPF>true</UseWPF>\n    <UseWindowsForms>true</UseWindowsForms>", 1)
		u.did("%s: Windows Forms, for the tray icon", rel)
		if regexp.MustCompile(`<ImplicitUsings>enable</ImplicitUsings>`).MatchString(text) {
			u.must("%s has ImplicitUsings on, which with Windows Forms makes names such as Application and Button ambiguous: turn it off, and add the usings each file needs", rel)
		}
	}
	if text != string(data) {
		return os.WriteFile(proj, []byte(text), 0o644)
	}
	return nil
}

// swiftPin moves the Mac app's dependency on vero's Swift package to Up
// to Next Major from the vero go.mod requires: in Package.swift for a
// Swift package, or in the Xcode project.
func (u *upgrader) swiftPin() error {
	folder := u.app.toml.get("macos", "folder")
	if folder == "" {
		return nil
	}
	// A replace with a checkout has no version, and the pin is left alone.
	v, err := output(u.dir, "go", "list", "-m", "-f", "{{if .Replace}}{{.Replace.Version}}{{else}}{{.Version}}{{end}}", "github.com/imclaren/vero")
	if err != nil {
		return err
	}
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(v) {
		return nil // a checkout, or a pseudo-version, which Swift cannot pin
	}
	if pkg := filepath.Join(u.dir, folder, "Package.swift"); fileExists(pkg) {
		data, _ := os.ReadFile(pkg)
		re := regexp.MustCompile(`(\.package\(url:\s*"https://github\.com/imclaren/vero(?:\.git)?",\s*)[^)]*\)`)
		text := re.ReplaceAllString(string(data), `${1}from: "`+v+`")`)
		if text != string(data) {
			u.did("%s/Package.swift: vero's Swift package from %s", folder, v)
			return os.WriteFile(pkg, []byte(text), 0o644)
		}
		return nil
	}
	projects, _ := filepath.Glob(filepath.Join(u.dir, folder, "*.xcodeproj", "project.pbxproj"))
	for _, p := range projects {
		data, _ := os.ReadFile(p)
		re := regexp.MustCompile(`(?s)(repositoryURL = "https://github\.com/imclaren/vero(?:\.git)?";\s*requirement = \{).*?(\};)`)
		text := re.ReplaceAllString(string(data), "${1}\n\t\t\t\tkind = upToNextMajorVersion;\n\t\t\t\tminimumVersion = "+v+";\n\t\t\t${2}")
		if text != string(data) {
			rel, _ := filepath.Rel(u.dir, p)
			u.did("%s: vero's Swift package Up to Next Major from %s; Xcode resolves it when the project next opens", filepath.Dir(rel), v)
			if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

// toml rewrites vero-app.toml's dependencies in the form that works them
// out, which vero-repo knows.
func (u *upgrader) toml() error {
	if u.app.toml.get("gtk", "folder") == "" {
		return nil
	}
	path, err := veroRepoPath(os.Getenv("VERO_DIR"))
	if err != nil {
		return err
	}
	cmd := exec.Command(path, "upgrade-toml", "--app", filepath.Join(u.dir, "vero-app.toml"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("vero-repo upgrade-toml: %v\n%s", err, out)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			u.did("vero-app.toml: %s", line)
		}
	}
	u.app.toml = readToml(filepath.Join(u.dir, "vero-app.toml"))
	return nil
}

// gen writes the front ends' request types, which the rest checks.
func (u *upgrader) gen() error {
	before := map[string][]byte{}
	for _, o := range genOutputs(u.app) {
		before[o.path], _ = os.ReadFile(filepath.Join(u.dir, o.path))
	}
	if err := gen([]string{"--dir", u.dir}); err != nil {
		u.must("vero gen did not finish (%v): run it once the worker builds", err)
		return nil
	}
	for _, o := range genOutputs(u.app) {
		now, _ := os.ReadFile(filepath.Join(u.dir, o.path))
		if !bytes.Equal(before[o.path], now) {
			u.did("%s: the worker's requests and state, from vero gen", o.path)
		}
	}
	return nil
}

// xcodeFile adds WorkerAPI.swift to an Xcode project whose folders are not
// synchronised with the disk, as a file in the app's group and its target.
func (u *upgrader) xcodeFile() error {
	folder, scheme := u.app.toml.get("macos", "folder"), u.app.toml.get("macos", "scheme")
	project := u.app.toml.get("macos", "project")
	if folder == "" || project == "" || scheme == "" {
		return nil
	}
	pbx := filepath.Join(u.dir, folder, project, "project.pbxproj")
	data, err := os.ReadFile(pbx)
	if err != nil {
		return nil
	}
	text, added, err := addSwiftFile(string(data), scheme, "WorkerAPI.swift")
	if err != nil {
		u.must("add %s/%s/WorkerAPI.swift to the Xcode project by hand: %v", folder, scheme, err)
		return nil
	}
	if !added {
		return nil
	}
	u.did("%s: WorkerAPI.swift added to the %s target", filepath.Join(folder, project), scheme)
	return os.WriteFile(pbx, []byte(text), 0o644)
}

// addSwiftFile adds name to an Xcode project, in the group whose path is
// group and in the Sources of the target of that name. It does nothing in
// a project whose folders are synchronised with the disk, or that has the
// file already.
func addSwiftFile(text, group, name string) (string, bool, error) {
	if strings.Contains(text, "PBXFileSystemSynchronizedRootGroup") || strings.Contains(text, "path = "+name+";") {
		return text, false, nil
	}
	id := func(seed string) string {
		for i := 0; ; i++ {
			sum := sha256.Sum256([]byte(fmt.Sprintf("vero %s %d", seed, i)))
			s := strings.ToUpper(hex.EncodeToString(sum[:12]))
			if !strings.Contains(text, s) {
				return s
			}
		}
	}
	fileID, buildID := id("file "+name), id("build "+name)
	insert := func(marker, line string) error {
		i := strings.Index(text, marker)
		if i < 0 {
			return fmt.Errorf("no %q", strings.TrimSpace(marker))
		}
		text = text[:i] + line + text[i:]
		return nil
	}
	if err := insert("/* End PBXBuildFile section */", fmt.Sprintf("\t\t%s /* %s in Sources */ = {isa = PBXBuildFile; fileRef = %s /* %s */; };\n", buildID, name, fileID, name)); err != nil {
		return text, false, err
	}
	if err := insert("/* End PBXFileReference section */", fmt.Sprintf("\t\t%s /* %s */ = {isa = PBXFileReference; lastKnownFileType = sourcecode.swift; path = %s; sourceTree = \"<group>\"; };\n", fileID, name, name)); err != nil {
		return text, false, err
	}
	// The group: the PBXGroup block whose path is group.
	groupRe := regexp.MustCompile(`(?s)(\t\t[0-9A-F]{24} /\* [^*]* \*/ = \{\n\t\t\tisa = PBXGroup;\n\t\t\tchildren = \(\n)(.*?)(\t\t\t\);\n[^}]*?path = "?` + regexp.QuoteMeta(group) + `"?;)`)
	m := groupRe.FindStringSubmatchIndex(text)
	if m == nil {
		return text, false, fmt.Errorf("no group with the path %s", group)
	}
	text = text[:m[5]] + fmt.Sprintf("\t\t\t\t%s /* %s */,\n", fileID, name) + text[m[5]:]
	// The target's Sources phase: the target named group, its build phase
	// whose comment is Sources.
	targetRe := regexp.MustCompile(`(?s)[0-9A-F]{24} /\* ` + regexp.QuoteMeta(group) + ` \*/ = \{\n\t\t\tisa = PBXNativeTarget;.*?buildPhases = \((.*?)\);`)
	t := targetRe.FindStringSubmatch(text)
	if t == nil {
		return text, false, fmt.Errorf("no target called %s", group)
	}
	phase := regexp.MustCompile(`([0-9A-F]{24}) /\* Sources \*/`).FindStringSubmatch(t[1])
	if phase == nil {
		return text, false, fmt.Errorf("the %s target has no Sources phase", group)
	}
	phaseRe := regexp.MustCompile(`(?s)` + phase[1] + ` /\* Sources \*/ = \{\n\t\t\tisa = PBXSourcesBuildPhase;.*?files = \(\n(.*?)(\t\t\t\);)`)
	p := phaseRe.FindStringSubmatchIndex(text)
	if p == nil {
		return text, false, fmt.Errorf("no Sources phase %s", phase[1])
	}
	text = text[:p[4]] + fmt.Sprintf("\t\t\t\t%s /* %s in Sources */,\n", buildID, name) + text[p[4]:]
	return text, true, nil
}

// clashes names each type the app defines that vero gen's files or vero's
// Swift package now define too, which the app must delete in favour of
// theirs.
func (u *upgrader) clashes() error {
	folder := u.app.toml.get("macos", "folder")
	if folder == "" {
		return nil
	}
	generated := map[string]bool{"JSONValue": true, "EmptyReply": true}
	var genFile string
	for _, o := range genOutputs(u.app) {
		if o.lang != "swift" {
			continue
		}
		genFile = filepath.Join(u.dir, o.path)
		data, _ := os.ReadFile(genFile)
		for _, m := range regexp.MustCompile(`(?m)^(?:struct|enum|typealias) (\w+)`).FindAllSubmatch(data, -1) {
			generated[string(m[1])] = true
		}
	}
	// Only a type at the top of a file can clash: one inside another type
	// is that type's own.
	decl := regexp.MustCompile(`(?m)^(?:public |private |fileprivate |internal |final )*(?:struct|enum|class|typealias) (\w+)`)
	found := map[string][]string{}
	var files []string
	err := filepath.WalkDir(filepath.Join(u.dir, folder), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() && (d.Name() == ".build" || d.Name() == "build" || strings.HasSuffix(d.Name(), ".xcodeproj")) {
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !strings.HasSuffix(p, ".swift") || p == genFile {
			return nil
		}
		data, _ := os.ReadFile(p)
		rel, _ := filepath.Rel(u.dir, p)
		for _, m := range decl.FindAllSubmatchIndex(data, -1) {
			name := string(data[m[2]:m[3]])
			if generated[name] {
				line := bytes.Count(data[:m[0]], []byte("\n")) + 1
				if found[rel] == nil {
					files = append(files, rel)
				}
				found[rel] = append(found[rel], fmt.Sprintf("%s (line %d)", name, line))
			}
		}
		return nil
	})
	for _, f := range files {
		what := fmt.Sprintf("%d types", len(found[f]))
		if len(found[f]) == 1 {
			what = "a type"
		}
		u.must("%s defines %s that vero gen's WorkerAPI.swift or vero's Swift package now defines too, so the app does not build until each is resolved: %s. "+
			"Where one is the app's own copy of the worker's type, delete it and use the generated one, which has the same JSON keys; where it is something else of the same name, such as a view, rename it",
			f, what, strings.Join(found[f], ", "))
	}
	return err
}
