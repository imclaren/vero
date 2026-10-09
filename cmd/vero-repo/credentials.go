package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// credentialsCommand is the checklist of what a release can have beyond
// what it has with no accounts at all: signing that stops the warnings,
// and the stores and listings people look in. For each it says whether
// it is set up, checks what it can on this machine, and prints the exact
// commands or settings for the rest. Nothing here is needed for a
// release; `vero-repo release` works without any of it.
func credentialsCommand(args []string) error {
	fs := flag.NewFlagSet("credentials", flag.ExitOnError)
	appPath := fs.String("app", "vero-app.toml", "the app's vero-app.toml")
	keyDir := fs.String("key", "", "the signing key's folder (default: the publisher's)")
	fs.Parse(args)
	a, err := LoadApp(*appPath)
	if err != nil {
		return err
	}
	if *keyDir == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		*keyDir = filepath.Join(dir, "vero-repo", strings.ToLower(strings.ReplaceAll(a.PublisherName(), " ", "-")))
	}

	have := func(tool string) bool { _, err := exec.LookPath(tool); return err == nil }
	identities := codesigningIdentities()
	pick := func(kind string) string {
		for _, id := range identities {
			if strings.HasPrefix(id, kind) {
				return id
			}
		}
		return ""
	}
	gh := have("gh") && exec.Command("gh", "auth", "status").Run() == nil

	type item struct {
		platform, what string
		ok             bool
		state, todo    string
	}
	var items []item
	add := func(platform, what string, ok bool, state, todo string) {
		items = append(items, item{platform, what, ok, state, todo})
	}

	// The key every release is signed with.
	add("everything", "signing key", fileExists(filepath.Join(*keyDir, privateFile)),
		"in "+*keyDir, "made on the first release; or vero-repo key --name ... --email ...")

	// macOS: a Developer ID, and notarisation.
	if a.MacOS != nil {
		id := os.Getenv("VERO_MAC_IDENTITY")
		found := pick("Developer ID Application")
		switch {
		case id != "":
			add("macOS", "Developer ID", true, "VERO_MAC_IDENTITY is set", "")
		case found != "":
			add("macOS", "Developer ID", false, "in your keychain, not set", fmt.Sprintf("export VERO_MAC_IDENTITY=%q", found))
		default:
			add("macOS", "Developer ID", false, "none in your keychain",
				"join the Apple Developer Program (developer.apple.com, US$99 a year); in Xcode, Settings > Accounts > Manage Certificates, add a Developer ID Application certificate; then run this again")
		}
		profile := os.Getenv("VERO_NOTARY_PROFILE")
		switch {
		case profile != "" && exec.Command("xcrun", "notarytool", "history", "--keychain-profile", profile).Run() == nil:
			add("macOS", "notarisation", true, "VERO_NOTARY_PROFILE works", "")
		case profile != "":
			add("macOS", "notarisation", false, "VERO_NOTARY_PROFILE is set but notarytool can't use it",
				"xcrun notarytool store-credentials "+profile+" --apple-id you@example.com --team-id TEAMID --password <an app-specific password from appleid.apple.com>")
		default:
			add("macOS", "notarisation", false, "not set",
				"xcrun notarytool store-credentials vero --apple-id you@example.com --team-id TEAMID --password <an app-specific password from appleid.apple.com>\n      export VERO_NOTARY_PROFILE=vero")
		}
		if a.MacOS.Pkg {
			inst := os.Getenv("VERO_MAC_INSTALLER")
			found := pick("Developer ID Installer")
			switch {
			case inst != "":
				add("macOS", "installer signing", true, "VERO_MAC_INSTALLER is set", "")
			case found != "":
				add("macOS", "installer signing", false, "in your keychain, not set", fmt.Sprintf("export VERO_MAC_INSTALLER=%q", found))
			default:
				add("macOS", "installer signing", false, "no Developer ID Installer certificate", "add one in Xcode as above, for the .pkg")
			}
		}
		add("macOS", "Homebrew tap", gh, ghState(gh), "vero-repo publish --homebrew   (a tap of your own on GitHub, kept up to date each release)")
	}

	// Windows: a code-signing certificate, winget, and the Store.
	if a.WPF != nil {
		cert := os.Getenv("VERO_WINDOWS_CERT")
		switch {
		case cert != "" && have("osslsigncode"):
			add("Windows", "code signing", true, "VERO_WINDOWS_CERT is set", "")
		case cert != "":
			add("Windows", "code signing", false, "VERO_WINDOWS_CERT is set, osslsigncode is missing", "brew install osslsigncode")
		default:
			add("Windows", "code signing", false, "not set; SmartScreen warns once",
				"buy a code-signing certificate (an OV one from any CA: Sectigo, DigiCert, SSL.com; EV ones skip SmartScreen at once), export it as a .pfx, then\n      export VERO_WINDOWS_CERT=/path/to/cert.pfx VERO_WINDOWS_CERT_PASSWORD=...   (and brew install osslsigncode)")
		}
		add("Windows", "winget", gh, ghState(gh), "vero-repo publish --winget   (a pull request to microsoft/winget-pkgs, each release)")
		if a.WPF.MSIX.Build {
			add("Windows", "Microsoft Store", false, "an MSIX is built; the Store signs it",
				"reserve the name in Partner Center (partner.microsoft.com, a one-off fee), put its identity_name and publisher in [wpf.msix], and upload dist/packages/*.msix there")
		}
	}

	// Android: a keystore of your own, and Google Play.
	if a.Android != nil {
		if os.Getenv("VERO_ANDROID_KEYSTORE") != "" {
			add("Android", "keystore", true, "VERO_ANDROID_KEYSTORE is set", "")
		} else {
			add("Android", "keystore", true, "vero's, beside your key: fine for installing from the site",
				"to use one you already ship with: export VERO_ANDROID_KEYSTORE=... VERO_ANDROID_KEY_ALIAS=... VERO_ANDROID_PASSWORD=...")
		}
		add("Android", "Google Play", false, "manual",
			"a Play Console account (play.google.com/console, a one-off fee); name aab in [android]; upload dist/packages/*.aab there")
	}

	// iOS: there is no way to a phone but Apple's.
	if a.IOS != nil {
		id := os.Getenv("VERO_IOS_IDENTITY")
		found := pick("Apple Distribution")
		switch {
		case id != "" && os.Getenv("VERO_IOS_PROFILE") != "":
			add("iOS", "App Store signing", true, "VERO_IOS_IDENTITY and VERO_IOS_PROFILE are set", "")
		case found != "":
			add("iOS", "App Store signing", false, "an Apple Distribution certificate is in your keychain",
				fmt.Sprintf("export VERO_IOS_IDENTITY=%q VERO_IOS_PROFILE=/path/to/profile.mobileprovision   (the profile: developer.apple.com > Profiles)", found))
		default:
			add("iOS", "App Store signing", false, "none; the Simulator app is all that's built",
				"the Apple Developer Program, as for macOS; an Apple Distribution certificate; an App Store provisioning profile; then export VERO_IOS_IDENTITY and VERO_IOS_PROFILE")
		}
		add("iOS", "TestFlight / App Store", false, "manual", "upload the .ipa with Transporter (App Store) or xcrun altool")
	}

	// Linux listings beyond your own site.
	if a.GTK != nil {
		aur := exec.Command("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", "aur@aur.archlinux.org").Run() == nil
		add("Arch", "AUR", aur, map[bool]string{true: "your SSH key is registered", false: "no account, or your SSH key isn't on it"}[aur],
			"an account at aur.archlinux.org with your SSH key; then vero-repo publish --aur   (each release)")
		if a.GTK.Flatpak.Build {
			add("Flatpak", "Flathub", false, "manual",
				"a pull request to github.com/flathub/flathub with the manifest vero built; Flathub reviews it by hand")
		}
	}

	// The rest need nothing.
	fmt.Printf("%s: beyond a release with no accounts, which the page explains to people\n\n", a.DisplayName)
	for _, it := range items {
		mark := "✗"
		if it.ok {
			mark = "✓"
		}
		fmt.Printf("%s %-10s %-20s %s\n", mark, it.platform, it.what, it.state)
		if !it.ok && it.todo != "" {
			fmt.Printf("      %s\n", it.todo)
		}
	}
	fmt.Println("\nDebian, Ubuntu, Chromebooks, Fedora, openSUSE, Arch, Alpine, Void, FreeBSD, DragonFly, NetBSD, OpenBSD, illumos, the browser, WASI and Plan 9 need nothing: the site is all there is.")
	fmt.Println("Settings exported in your shell are read by the next vero-repo release.")
	return nil
}

func ghState(ok bool) string {
	if ok {
		return "ready: gh is signed in"
	}
	return "needs gh signed in: brew install gh && gh auth login"
}

// codesigningIdentities is each identity the keychain can sign with, as
// codesign names it: "Developer ID Application: Name (TEAMID)".
func codesigningIdentities() []string {
	out, err := exec.Command("security", "find-identity", "-v", "-p", "codesigning").Output()
	if err != nil {
		return nil
	}
	var ids []string
	for _, m := range regexp.MustCompile(`"([^"]+)"`).FindAllStringSubmatch(string(out), -1) {
		ids = append(ids, m[1])
	}
	return ids
}
