package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"filippo.io/edwards25519"
)

// The Mac app: an .app built with SwiftPM or Xcode, the worker inside it,
// signed and put on a disk image. With a Developer ID it is signed with
// that, and notarised when there is a notarytool profile; without one it is
// signed ad hoc, which runs once whoever opens it allows it.
//
// What signs it comes from the environment, never from vero-app.toml:
//
//	VERO_MAC_IDENTITY        "Developer ID Application: Name (TEAMID)"
//	VERO_MAC_INSTALLER       "Developer ID Installer: Name (TEAMID)", for a .pkg
//	VERO_NOTARY_PROFILE      a profile made with xcrun notarytool store-credentials

// MacOS is the front end for macOS.
type MacOS struct {
	// Folder is the front end's folder. A SwiftPM package names its
	// executable Product; an Xcode project names Project and Scheme.
	Folder  string `toml:"folder"`
	Product string `toml:"product"`
	Project string `toml:"project"`
	Scheme  string `toml:"scheme"`
	// Prebuild is a command run once in Folder before anything is built:
	// what downloads or builds the files Resources lists, say. Prepare is
	// run in Folder before each architecture is built, with GOARCH and CC
	// set for it: what builds a C archive of Go, say.
	Prebuild string `toml:"prebuild"`
	Prepare  string `toml:"prepare"`
	// Resources are more files for the app's Contents/Resources: helper
	// programs and their licences, say. Each program in it is signed
	// before the app around it.
	Resources []string `toml:"resources"`
	// Entitlements is a .plist of entitlements the app and its programs
	// are signed with, under the hardened runtime.
	Entitlements string `toml:"entitlements"`
	// DownloadURL is where the disk images are downloaded from, when that
	// isn't the site's macos folder: a server of your own, say. The appcast
	// and the Homebrew cask point there.
	DownloadURL string `toml:"download_url"`
	// Menubar is an app that lives in the menu bar, with no Dock icon.
	Menubar bool `toml:"menubar"`
	// Minimum is the oldest macOS it runs on: "13.0" unless it says.
	Minimum string `toml:"minimum"`
	// Category is its App Store category, which Finder shows.
	Category string `toml:"category"`
	// Pkg also makes an installer package, beside the disk image.
	Pkg bool `toml:"pkg"`
	// Feed and SparklePublicKey are what an app that updates itself with
	// Sparkle reads from its Info.plist: where the site's appcast is, and
	// the public half of the key that signs updates, which vero-repo key
	// prints. vero writes them into an Info.plist it makes; an Xcode
	// project's own Info.plist has to say them itself.
	Feed             string `toml:"feed"`
	SparklePublicKey string `toml:"sparkle_public_key"`
}

// macArches are what a Mac app is built for: one binary, for both.
var macArches = []struct{ goarch, swift string }{{"arm64", "arm64"}, {"amd64", "x86_64"}}

// macInfo is what package says about a disk image, beside it, for build:
// the versions in its Info.plist, which Sparkle compares, and the oldest
// macOS it runs on.
type macInfo struct {
	BundleVersion string `json:"bundleVersion"`
	ShortVersion  string `json:"shortVersion"`
	Minimum       string `json:"minimum"`
	App           string `json:"app"`
}

// macDisk is NAME-VERSION-macos.dmg.
func macDisk(name string) *regexp.Regexp {
	return regexp.MustCompile(`^` + regexp.QuoteMeta(name) + `-(.+)-macos\.dmg$`)
}

// packageMac builds the Mac app and its disk image, and a .pkg if asked,
// into out.
func packageMac(a *App, root, worker, out, ldflags, tmp string) error {
	m := a.MacOS
	// The worker, for both architectures, in one file.
	var workers []string
	for _, arch := range macArches {
		w, err := buildWorker(a, root, worker, "darwin", arch.goarch, ldflags, tmp)
		if err != nil {
			return err
		}
		workers = append(workers, w)
	}
	universal := filepath.Join(tmp, a.Worker.Name+"-darwin")
	if err := runIn("", "lipo", append([]string{"-create", "-output", universal}, workers...)...); err != nil {
		return err
	}

	if m.Prebuild != "" {
		cmd := exec.Command("sh", "-c", m.Prebuild)
		cmd.Dir, cmd.Stdout, cmd.Stderr = a.Path(m.Folder), os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("[macos] prebuild: %w", err)
		}
	}
	var app string
	var err error
	if m.Project != "" {
		app, err = buildXcodeApp(a, tmp)
	} else {
		app, err = buildSwiftPMApp(a, tmp)
	}
	if err != nil {
		return err
	}
	resources := filepath.Join(app, "Contents", "Resources")
	if err := os.MkdirAll(resources, 0o755); err != nil {
		return err
	}
	if err := copyFile(universal, filepath.Join(resources, a.Worker.Name)); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Join(resources, a.Worker.Name), 0o755); err != nil {
		return err
	}
	for _, r := range m.Resources {
		src := a.Path(filepath.Join(m.Folder, r))
		if filepath.IsAbs(r) {
			src = r
		}
		st, err := os.Stat(src)
		if err != nil {
			return fmt.Errorf("[macos] resources: %w", err)
		}
		if err := copyFile(src, filepath.Join(resources, filepath.Base(r))); err != nil {
			return err
		}
		if err := os.Chmod(filepath.Join(resources, filepath.Base(r)), st.Mode().Perm()); err != nil {
			return err
		}
	}
	info, err := readInfo(app)
	if err != nil {
		return err
	}

	// Signed inside out: every program in the app - the worker, and any
	// in Resources - then the app around them, each with the hardened
	// runtime that notarisation needs.
	identity := os.Getenv("VERO_MAC_IDENTITY")
	sign := func(path string) error {
		args := []string{"--force", "--sign", "-"}
		if identity != "" {
			args = []string{"--force", "--options", "runtime", "--timestamp", "--sign", identity}
		}
		if m.Entitlements != "" {
			args = append(args, "--entitlements", a.Path(filepath.Join(m.Folder, m.Entitlements)))
		}
		return runIn("", "codesign", append(args, path)...)
	}
	nested, err := machOFiles(filepath.Join(app, "Contents"), filepath.Join(app, "Contents", "MacOS"))
	if err != nil {
		return err
	}
	for _, f := range nested {
		if err := sign(f); err != nil {
			return err
		}
	}
	if err := sign(app); err != nil {
		return err
	}
	if identity == "" {
		fmt.Println("signed the Mac app ad hoc: set VERO_MAC_IDENTITY to sign it with your Developer ID")
	}

	// The disk image: the app, and a link to Applications to drag it to.
	stage := filepath.Join(tmp, "dmg")
	if err := os.MkdirAll(stage, 0o755); err != nil {
		return err
	}
	if err := runIn("", "ditto", app, filepath.Join(stage, filepath.Base(app))); err != nil {
		return err
	}
	if err := os.Symlink("/Applications", filepath.Join(stage, "Applications")); err != nil {
		return err
	}
	dmg := filepath.Join(out, fmt.Sprintf("%s-%s-macos.dmg", a.Name, a.Version))
	os.Remove(dmg)
	if err := runIn("", "hdiutil", "create", "-quiet", "-volname", a.DisplayName, "-srcfolder", stage, "-ov", "-format", "UDZO", dmg); err != nil {
		return err
	}
	if identity != "" {
		if err := runIn("", "codesign", "--force", "--timestamp", "--sign", identity, dmg); err != nil {
			return err
		}
	}
	if profile := os.Getenv("VERO_NOTARY_PROFILE"); profile != "" && identity != "" {
		fmt.Println("notarising the disk image, which takes a few minutes")
		if err := runIn("", "xcrun", "notarytool", "submit", dmg, "--keychain-profile", profile, "--wait"); err != nil {
			return err
		}
		if err := runIn("", "xcrun", "stapler", "staple", dmg); err != nil {
			return err
		}
	}
	info.App = filepath.Base(app)
	data, _ := json.MarshalIndent(info, "", "  ")
	if err := os.WriteFile(dmg+".json", append(data, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Println("built", dmg)

	if m.Pkg {
		pkg := filepath.Join(out, fmt.Sprintf("%s-%s-macos.pkg", a.Name, a.Version))
		args := []string{"--component", app, "--install-location", "/Applications"}
		if installer := os.Getenv("VERO_MAC_INSTALLER"); installer != "" {
			args = append(args, "--sign", installer, "--timestamp")
		}
		if err := runIn("", "pkgbuild", append(args, pkg)...); err != nil {
			return err
		}
		fmt.Println("built", pkg)
	}
	return nil
}

// buildSwiftPMApp builds a SwiftPM executable for each architecture, joins
// them, and puts the result in an .app vero makes, with an Info.plist and
// an icon from vero-app.toml.
func buildSwiftPMApp(a *App, tmp string) (string, error) {
	m := a.MacOS
	folder := a.Path(m.Folder)
	var bins []string
	for _, arch := range macArches {
		if m.Prepare != "" {
			cmd := exec.Command("sh", "-c", m.Prepare)
			cmd.Dir, cmd.Stdout, cmd.Stderr = folder, os.Stdout, os.Stderr
			cmd.Env = append(os.Environ(), "GOOS=darwin", "GOARCH="+arch.goarch, "CGO_ENABLED=1",
				"CC=clang -arch "+arch.swift, "MACOSX_DEPLOYMENT_TARGET="+macMinimum(m))
			if err := cmd.Run(); err != nil {
				return "", fmt.Errorf("[macos] prepare, for %s: %w", arch.goarch, err)
			}
		}
		scratch := filepath.Join(tmp, "swift-"+arch.swift)
		args := []string{"build", "-c", "release", "--arch", arch.swift, "--scratch-path", scratch, "--product", m.Product}
		if err := runIn(folder, "swift", args...); err != nil {
			return "", err
		}
		path, err := exec.Command("swift", append(args, "--show-bin-path")...).Output()
		if err != nil {
			return "", err
		}
		bins = append(bins, filepath.Join(strings.TrimSpace(string(path)), m.Product))
	}
	app := filepath.Join(tmp, a.DisplayName+".app")
	contents := filepath.Join(app, "Contents")
	for _, d := range []string{"MacOS", "Resources"} {
		if err := os.MkdirAll(filepath.Join(contents, d), 0o755); err != nil {
			return "", err
		}
	}
	if err := runIn("", "lipo", append([]string{"-create", "-output", filepath.Join(contents, "MacOS", m.Product)}, bins...)...); err != nil {
		return "", err
	}
	icns, err := appleIcon(a.Path(a.Icon))
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(contents, "Resources", "AppIcon.icns"), icns, 0o644); err != nil {
		return "", err
	}
	return app, os.WriteFile(filepath.Join(contents, "Info.plist"), infoPlist(a), 0o644)
}

// buildXcodeApp builds an Xcode project's scheme for both architectures,
// unsigned, since vero signs it once the worker is inside.
func buildXcodeApp(a *App, tmp string) (string, error) {
	m := a.MacOS
	data := filepath.Join(tmp, "xcode")
	err := runIn(a.Path(m.Folder), "xcodebuild", "-quiet", "-project", m.Project, "-scheme", m.Scheme,
		"-configuration", "Release", "-derivedDataPath", data, "ARCHS=arm64 x86_64", "ONLY_ACTIVE_ARCH=NO",
		"CODE_SIGNING_ALLOWED=NO", "build")
	if err != nil {
		return "", err
	}
	apps, _ := filepath.Glob(filepath.Join(data, "Build", "Products", "Release", "*.app"))
	if len(apps) != 1 {
		return "", fmt.Errorf("xcodebuild made %d apps, not one", len(apps))
	}
	return apps[0], nil
}

func macMinimum(m *MacOS) string {
	if m.Minimum != "" {
		return m.Minimum
	}
	return "13.0"
}

// infoPlist is the Info.plist of an app vero makes.
func infoPlist(a *App) []byte {
	m := a.MacOS
	category := m.Category
	if category == "" {
		category = "public.app-category.utilities"
	}
	keys := [][2]string{
		{"CFBundleDevelopmentRegion", "en"},
		{"CFBundleExecutable", m.Product},
		{"CFBundleIconFile", "AppIcon"},
		{"CFBundleIdentifier", a.ID},
		{"CFBundleInfoDictionaryVersion", "6.0"},
		{"CFBundleName", a.DisplayName},
		{"CFBundleDisplayName", a.DisplayName},
		{"CFBundlePackageType", "APPL"},
		{"CFBundleShortVersionString", a.Version},
		{"CFBundleVersion", a.buildNumber()},
		{"LSApplicationCategoryType", category},
		{"LSMinimumSystemVersion", macMinimum(m)},
		{"NSHumanReadableCopyright", a.PublisherName()},
	}
	if m.Feed != "" {
		keys = append(keys, [2]string{"SUFeedURL", m.Feed})
	}
	if m.SparklePublicKey != "" {
		keys = append(keys, [2]string{"SUPublicEDKey", m.SparklePublicKey})
	}
	var b bytes.Buffer
	b.WriteString(xml.Header + `<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n<plist version=\"1.0\">\n<dict>\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "\t<key>%s</key>\n\t<string>%s</string>\n", k[0], xmlText(k[1]))
	}
	b.WriteString("\t<key>NSHighResolutionCapable</key>\n\t<true/>\n")
	if m.Menubar {
		b.WriteString("\t<key>LSUIElement</key>\n\t<true/>\n")
	}
	b.WriteString("</dict>\n</plist>\n")
	return b.Bytes()
}

// readInfo is what an app's Info.plist says of its versions, read with
// plutil, which every Mac has.
func readInfo(app string) (macInfo, error) {
	var info macInfo
	plist := filepath.Join(app, "Contents", "Info.plist")
	out, err := exec.Command("plutil", "-convert", "json", "-o", "-", plist).Output()
	if err != nil {
		return info, fmt.Errorf("reading %s: %w", plist, err)
	}
	var keys map[string]any
	if err := json.Unmarshal(out, &keys); err != nil {
		return info, err
	}
	str := func(k string) string { s, _ := keys[k].(string); return s }
	info.BundleVersion, info.ShortVersion, info.Minimum = str("CFBundleVersion"), str("CFBundleShortVersionString"), str("LSMinimumSystemVersion")
	if info.BundleVersion == "" {
		return info, fmt.Errorf("%s has no CFBundleVersion, which updates are compared by", plist)
	}
	return info, nil
}

// appleIcon is an .icns of the icon, made of PNGs at the sizes Finder and
// the Dock use, as macOS has read them since 10.7.
func appleIcon(file string) ([]byte, error) {
	types := []struct {
		kind string
		size int
	}{{"icp4", 16}, {"icp5", 32}, {"icp6", 64}, {"ic07", 128}, {"ic08", 256}, {"ic09", 512}, {"ic10", 1024}, {"ic11", 32}, {"ic12", 64}, {"ic13", 256}, {"ic14", 512}}
	var sizes []int
	for _, t := range types {
		sizes = append(sizes, t.size)
	}
	pngs, err := iconSizes(file, sizes...)
	if err != nil {
		return nil, err
	}
	var body bytes.Buffer
	for _, t := range types {
		body.WriteString(t.kind)
		binary.Write(&body, binary.BigEndian, uint32(8+len(pngs[t.size])))
		body.Write(pngs[t.size])
	}
	var out bytes.Buffer
	out.WriteString("icns")
	binary.Write(&out, binary.BigEndian, uint32(8+body.Len()))
	out.Write(body.Bytes())
	return out.Bytes(), nil
}

// runIn runs a program in dir, its output shown.
func runIn(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir, cmd.Stdout, cmd.Stderr = dir, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// sparkleFile is the key that signs Mac updates for Sparkle, beside the
// OpenPGP key: an Ed25519 seed, which Sparkle's own keys are too.
const sparkleFile = "sparkle.sec"

// sparkleKey signs Mac updates as Sparkle checks them: with Ed25519, from
// a seed, or - for a key Sparkle's own older tools made, which kept only
// the expanded key - from that.
type sparkleKey struct {
	seed     ed25519.PrivateKey // nil for an expanded key
	expanded []byte             // the scalar, then the prefix: 64 bytes
	public   []byte
}

// sign is the signature Sparkle's sign_update would give data.
func (k *sparkleKey) sign(data []byte) []byte {
	if k.seed != nil {
		return ed25519.Sign(k.seed, data)
	}
	// Ed25519 with the expanded key, as RFC 8032 signs once it has it.
	s, _ := edwards25519.NewScalar().SetBytesWithClamping(k.expanded[:32])
	h := sha512.New()
	h.Write(k.expanded[32:])
	h.Write(data)
	r, _ := edwards25519.NewScalar().SetUniformBytes(h.Sum(nil))
	R := (&edwards25519.Point{}).ScalarBaseMult(r).Bytes()
	h.Reset()
	h.Write(R)
	h.Write(k.public)
	h.Write(data)
	c, _ := edwards25519.NewScalar().SetUniformBytes(h.Sum(nil))
	S := edwards25519.NewScalar().MultiplyAdd(c, s, r)
	return append(R, S.Bytes()...)
}

// parseSparkleKey reads a Sparkle private key as base64: a 32-byte seed,
// as vero and Sparkle's generate_keys -x keep one; a seed and its public
// key; or an expanded key, as older Sparkle kept them.
func parseSparkleKey(text string) (*sparkleKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(text))
	if err != nil {
		return nil, errors.New("a Sparkle private key is base64")
	}
	switch len(raw) {
	case ed25519.SeedSize:
		key := ed25519.NewKeyFromSeed(raw)
		return &sparkleKey{seed: key, public: key.Public().(ed25519.PublicKey)}, nil
	case 64:
		if key := ed25519.NewKeyFromSeed(raw[:32]); bytes.Equal(key[32:], raw[32:]) {
			return &sparkleKey{seed: key, public: key[32:]}, nil
		}
		s, err := edwards25519.NewScalar().SetBytesWithClamping(raw[:32])
		if err != nil {
			return nil, err
		}
		pub := (&edwards25519.Point{}).ScalarBaseMult(s).Bytes()
		return &sparkleKey{expanded: raw, public: pub}, nil
	}
	return nil, fmt.Errorf("a Sparkle private key is 32 or 64 bytes, not %d", len(raw))
}

// PublicBase64 is SUPublicEDKey: what an app's Info.plist says.
func (k *sparkleKey) PublicBase64() string { return base64.StdEncoding.EncodeToString(k.public) }

// sparkle is the key that signs Mac updates: sparkle.sec in the key's
// folder - imported with vero-repo key --import-sparkle, or made the first
// time it's needed.
func (s *signer) sparkle() (*sparkleKey, error) {
	if s.dir == "" {
		return nil, errors.New("the signing key has no folder")
	}
	file := filepath.Join(s.dir, sparkleFile)
	data, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		_, key, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(file, []byte(base64.StdEncoding.EncodeToString(key.Seed())+"\n"), 0o600); err != nil {
			return nil, err
		}
		k := &sparkleKey{seed: key, public: key.Public().(ed25519.PublicKey)}
		fmt.Printf("made %s, which signs Mac updates: keep it with private.asc\n", file)
		fmt.Printf("its public key, for sparkle_public_key in vero-app.toml or SUPublicEDKey in Info.plist: %s\n", k.PublicBase64())
		return k, nil
	}
	if err != nil {
		return nil, err
	}
	k, err := parseSparkleKey(string(data))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", file, err)
	}
	return k, nil
}

// importSparkle keeps an existing Sparkle private key in dir, so that apps
// already installed - which check updates against its public half - keep
// updating.
func importSparkle(dir, from string) error {
	file := filepath.Join(dir, sparkleFile)
	if _, err := os.Stat(file); err == nil {
		return fmt.Errorf("%s already has a Sparkle key, which isn't replaced", dir)
	}
	data, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	k, err := parseSparkleKey(string(data))
	if err != nil {
		return fmt.Errorf("%s: %w", from, err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(file, []byte(strings.TrimSpace(string(data))+"\n"), 0o600); err != nil {
		return err
	}
	fmt.Printf("kept the Sparkle key in %s; its public key is %s\n", file, k.PublicBase64())
	return nil
}

// buildMac puts new disk images in site/macos, keeps the newest keep, and
// writes the Sparkle appcast and the Homebrew cask from them.
func buildMac(site string, newDmgs []string, keep int, url string, a *App, s *signer) (map[string]Download, error) {
	dir := filepath.Join(site, "macos")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	for _, dmg := range newDmgs {
		if !macDisk(a.Name).MatchString(filepath.Base(dmg)) {
			continue
		}
		info, err := os.ReadFile(dmg + ".json")
		if err != nil {
			return nil, fmt.Errorf("%s has no %s beside it, which vero-repo package writes", dmg, filepath.Base(dmg)+".json")
		}
		if err := copyFile(dmg, filepath.Join(dir, filepath.Base(dmg))); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(dmg)+".json"), info, 0o644); err != nil {
			return nil, err
		}
	}
	files, version, err := keepNewest(dir, macDisk(a.Name), keep)
	if err != nil {
		return nil, err
	}
	// What keepNewest removed takes its note with it.
	notes, _ := filepath.Glob(filepath.Join(dir, "*.dmg.json"))
	for _, n := range notes {
		if _, err := os.Stat(strings.TrimSuffix(n, ".json")); os.IsNotExist(err) {
			os.Remove(n)
		}
	}
	if len(files) == 0 {
		return nil, nil
	}
	key, err := s.sparkle()
	if err != nil {
		return nil, err
	}
	url = strings.TrimRight(url, "/")
	downloads := url + "/macos"
	if a.MacOS != nil && a.MacOS.DownloadURL != "" {
		downloads = strings.TrimRight(a.MacOS.DownloadURL, "/")
	}
	var items []appcastItem
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			return nil, err
		}
		var info macInfo
		note, err := os.ReadFile(filepath.Join(dir, f+".json"))
		if err != nil || json.Unmarshal(note, &info) != nil {
			return nil, fmt.Errorf("%s has no readable %s", f, f+".json")
		}
		st, _ := os.Stat(filepath.Join(dir, f))
		items = append(items, appcastItem{
			Title: a.DisplayName + " " + version[f], Date: st.ModTime().UTC().Format("Mon, 02 Jan 2006 15:04:05 +0000"),
			Version: info.BundleVersion, ShortVersion: info.ShortVersion, Minimum: info.Minimum,
			Enclosure: appcastEnclosure{URL: downloads + "/" + f, Length: len(data), Type: "application/octet-stream",
				Signature: base64.StdEncoding.EncodeToString(key.sign(data))},
		})
	}
	cast := appcast{Version: "2.0", Sparkle: "http://www.andymatuschak.org/xml-namespaces/sparkle", Channel: appcastChannel{
		Title: a.DisplayName, Link: url, Description: a.Summary, Language: "en", Items: items}}
	data, err := xml.MarshalIndent(cast, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "appcast.xml"), append([]byte(xml.Header), append(data, '\n')...), 0o644); err != nil {
		return nil, err
	}
	newest := files[0]
	sum, err := fileSHA256(filepath.Join(dir, newest))
	if err != nil {
		return nil, err
	}
	var info macInfo
	note, _ := os.ReadFile(filepath.Join(dir, newest+".json"))
	json.Unmarshal(note, &info)
	if err := writeCask(site, downloads, url, a, version[newest], sum, info); err != nil {
		return nil, err
	}
	return map[string]Download{"universal": {Version: version[newest], URL: "macos/" + newest}}, nil
}

type appcast struct {
	XMLName xml.Name       `xml:"rss"`
	Version string         `xml:"version,attr"`
	Sparkle string         `xml:"xmlns:sparkle,attr"`
	Channel appcastChannel `xml:"channel"`
}

type appcastChannel struct {
	Title       string        `xml:"title"`
	Link        string        `xml:"link"`
	Description string        `xml:"description"`
	Language    string        `xml:"language"`
	Items       []appcastItem `xml:"item"`
}

type appcastItem struct {
	Title        string           `xml:"title"`
	Date         string           `xml:"pubDate"`
	Version      string           `xml:"sparkle:version"`
	ShortVersion string           `xml:"sparkle:shortVersionString"`
	Minimum      string           `xml:"sparkle:minimumSystemVersion,omitempty"`
	Enclosure    appcastEnclosure `xml:"enclosure"`
}

type appcastEnclosure struct {
	URL       string `xml:"url,attr"`
	Length    int    `xml:"length,attr"`
	Type      string `xml:"type,attr"`
	Signature string `xml:"sparkle:edSignature,attr"`
}

// writeCask writes a Homebrew cask for the newest disk image, which a tap
// of the publisher's can carry, or homebrew-cask itself.
func writeCask(site, downloads, url string, a *App, version, sum string, info macInfo) error {
	dir := filepath.Join(site, "homebrew")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	app := info.App
	if app == "" {
		app = a.DisplayName + ".app"
	}
	homepage := a.Homepage
	if homepage == "" {
		homepage = url
	}
	cask := fmt.Sprintf(`cask %q do
  version %q
  sha256 %q

  url "%s/%s-#{version}-macos.dmg"
  name %q
  desc %q
  homepage %q

  app %q
end
`, a.Name, version, sum, downloads, a.Name, a.DisplayName, strings.TrimSuffix(a.Summary, "."), homepage, app)
	return os.WriteFile(filepath.Join(dir, a.Name+".rb"), []byte(cask), 0o644)
}

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// machOFiles are the programs and libraries under dir, outside skip (the
// app's own executable, which signing the app signs): what has to be
// signed before the app around them.
func machOFiles(dir, skip string) ([]string, error) {
	var out []string
	err := filepath.Walk(dir, func(path string, st os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if st.IsDir() {
			if path == skip || strings.HasSuffix(path, ".framework") {
				return filepath.SkipDir
			}
			return nil
		}
		if !st.Mode().IsRegular() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		var magic [4]byte
		if n, _ := f.Read(magic[:]); n == 4 && isMachO(magic) {
			out = append(out, path)
		}
		return nil
	})
	return out, err
}

func isMachO(m [4]byte) bool {
	switch binary.BigEndian.Uint32(m[:]) {
	case 0xfeedface, 0xfeedfacf, 0xcefaedfe, 0xcffaedfe, 0xcafebabe, 0xbebafeca:
		return true
	}
	return false
}
