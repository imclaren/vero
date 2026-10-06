package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"hash/crc32"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// An MSIX package, for the Microsoft Store, written in Go: a zip of the
// published WPF app and the worker, with AppxManifest.xml, the block map
// that lists a hash of every 64 KB of every file, and [Content_Types].xml.
// It isn't signed: the Store signs what it publishes. To install one
// yourself, sign it first with signtool and a certificate the PC trusts.

// msixBlock is the size of the blocks the block map hashes.
const msixBlock = 64 * 1024

// packageMSIX writes NAME-VERSION-ARCH.msix from the folder dotnet
// published the app into, for arch (x64 or arm64).
func packageMSIX(a *App, published, worker, arch, out string) (string, error) {
	m := a.WPF.MSIX
	if m.IdentityName == "" || m.Publisher == "" {
		return "", fmt.Errorf("[wpf.msix] needs identity_name and publisher, which Partner Center gives the app when you reserve its name")
	}
	var files []treeFile
	err := filepath.WalkDir(published, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(published, path)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files = append(files, treeFile{path: filepath.ToSlash(rel), data: data, mode: 0o644})
		return nil
	})
	if err != nil {
		return "", err
	}
	w, err := os.ReadFile(worker)
	if err != nil {
		return "", err
	}
	files = append(files, treeFile{path: a.Worker.Name + ".exe", data: w, mode: 0o755})
	logos, err := iconSizes(a.Path(a.Icon), 44, 50, 150)
	if err != nil {
		return "", err
	}
	files = append(files,
		treeFile{path: "Assets/Square44x44Logo.png", data: logos[44]},
		treeFile{path: "Assets/StoreLogo.png", data: logos[50]},
		treeFile{path: "Assets/Square150x150Logo.png", data: logos[150]})
	for _, f := range files {
		if strings.ContainsAny(f.path, " %#") {
			return "", fmt.Errorf("%s: an MSIX can't hold a name with a space, %% or #", f.path)
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	files = append(files, treeFile{path: "AppxManifest.xml", data: msixManifest(a, arch)})

	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	var blockMap bytes.Buffer
	blockMap.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="no"?>` + "\n" +
		`<BlockMap xmlns="http://schemas.microsoft.com/appx/2010/blockmap" HashMethod="http://www.w3.org/2001/04/xmlenc#sha256">` + "\n")
	add := func(name string, data []byte) error {
		// Stored, not compressed, with the sizes in the local header, so
		// that each block's hash is of the bytes as they are in the zip.
		h := &zip.FileHeader{Name: name, Method: zip.Store, CRC32: crc32.ChecksumIEEE(data),
			CompressedSize64: uint64(len(data)), UncompressedSize64: uint64(len(data))}
		fw, err := zw.CreateRaw(h)
		if err != nil {
			return err
		}
		_, err = fw.Write(data)
		return err
	}
	for _, f := range files {
		if err := add(f.path, f.data); err != nil {
			return "", err
		}
		fmt.Fprintf(&blockMap, `<File Name="%s" Size="%d" LfhSize="%d">`, xmlText(strings.ReplaceAll(f.path, "/", `\`)), len(f.data), 30+len(f.path))
		for i := 0; i < len(f.data); i += msixBlock {
			sum := sha256.Sum256(f.data[i:min(i+msixBlock, len(f.data))])
			fmt.Fprintf(&blockMap, `<Block Hash="%s"/>`, base64.StdEncoding.EncodeToString(sum[:]))
		}
		blockMap.WriteString("</File>\n")
	}
	blockMap.WriteString("</BlockMap>\n")
	if err := add("AppxBlockMap.xml", blockMap.Bytes()); err != nil {
		return "", err
	}
	if err := add("[Content_Types].xml", contentTypes(files)); err != nil {
		return "", err
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	path := filepath.Join(out, fmt.Sprintf("%s-%s-%s.msix", a.Name, a.Version, arch))
	return path, os.WriteFile(path, b.Bytes(), 0o644)
}

// msixVersion is the version as an MSIX has it: four numbers, the last
// 0, as the Store wants.
func msixVersion(v string) string {
	var nums []string
	for _, part := range strings.FieldsFunc(v, func(r rune) bool { return r < '0' || r > '9' }) {
		nums = append(nums, strings.TrimLeft(part, "0"))
		if len(nums) == 3 {
			break
		}
	}
	for len(nums) < 3 {
		nums = append(nums, "0")
	}
	for i, n := range nums {
		if n == "" {
			nums[i] = "0"
		}
	}
	return strings.Join(nums, ".") + ".0"
}

func msixManifest(a *App, arch string) []byte {
	m := a.WPF.MSIX
	publisherName := m.PublisherName
	if publisherName == "" {
		publisherName = a.PublisherName()
	}
	exe := a.WPF.Exe
	startup := ""
	if a.Needs.Autostart {
		startup = fmt.Sprintf(`
      <Extensions>
        <desktop:Extension Category="windows.startupTask" Executable="%s" EntryPoint="Windows.FullTrustApplication">
          <desktop:StartupTask TaskId="%sStartup" Enabled="true" DisplayName="%s"/>
        </desktop:Extension>
      </Extensions>`, xmlText(exe), xmlText(strings.ReplaceAll(a.Name, "-", "")), xmlText(a.DisplayName))
	}
	return []byte(fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<Package xmlns="http://schemas.microsoft.com/appx/manifest/foundation/windows10"
  xmlns:uap="http://schemas.microsoft.com/appx/manifest/uap/windows10"
  xmlns:desktop="http://schemas.microsoft.com/appx/manifest/desktop/windows10"
  xmlns:rescap="http://schemas.microsoft.com/appx/manifest/foundation/windows10/restrictedcapabilities"
  IgnorableNamespaces="uap desktop rescap">
  <Identity Name="%s" Publisher="%s" Version="%s" ProcessorArchitecture="%s"/>
  <Properties>
    <DisplayName>%s</DisplayName>
    <PublisherDisplayName>%s</PublisherDisplayName>
    <Logo>Assets\StoreLogo.png</Logo>
  </Properties>
  <Dependencies>
    <TargetDeviceFamily Name="Windows.Desktop" MinVersion="10.0.17763.0" MaxVersionTested="10.0.26100.0"/>
  </Dependencies>
  <Resources>
    <Resource Language="en-us"/>
  </Resources>
  <Applications>
    <Application Id="App" Executable="%s" EntryPoint="Windows.FullTrustApplication">
      <uap:VisualElements DisplayName="%s" Description="%s" BackgroundColor="transparent"
        Square150x150Logo="Assets\Square150x150Logo.png" Square44x44Logo="Assets\Square44x44Logo.png"/>%s
    </Application>
  </Applications>
  <Capabilities>
    <rescap:Capability Name="runFullTrust"/>%s
  </Capabilities>
</Package>
`, xmlText(m.IdentityName), xmlText(m.Publisher), msixVersion(a.Version), arch,
		xmlText(a.DisplayName), xmlText(publisherName), xmlText(exe), xmlText(a.DisplayName), xmlText(a.Summary), startup,
		map[bool]string{true: "\n    <Capability Name=\"internetClient\"/>"}[a.Needs.Network]))
}

// contentTypes is [Content_Types].xml: each file's type, by extension, and
// by name for the manifest, the block map and anything without one.
func contentTypes(files []treeFile) []byte {
	known := map[string]string{
		"exe": "application/x-msdownload", "dll": "application/x-msdownload", "png": "image/png",
		"json": "application/json", "xml": "text/xml", "pri": "application/octet-stream",
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`)
	seen := map[string]bool{}
	var overrides []string
	for _, f := range files {
		if f.path == "AppxManifest.xml" {
			continue
		}
		ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(f.path), "."))
		if ext == "" {
			overrides = append(overrides, f.path)
			continue
		}
		if !seen[ext] {
			seen[ext] = true
			t := known[ext]
			if t == "" {
				t = "application/octet-stream"
			}
			fmt.Fprintf(&b, `<Default Extension="%s" ContentType="%s"/>`, xmlText(ext), t)
		}
	}
	for _, o := range overrides {
		fmt.Fprintf(&b, `<Override PartName="/%s" ContentType="application/octet-stream"/>`, xmlText(o))
	}
	b.WriteString(`<Override PartName="/AppxManifest.xml" ContentType="application/vnd.ms-appx.manifest+xml"/>`)
	b.WriteString(`<Override PartName="/AppxBlockMap.xml" ContentType="application/vnd.ms-appx.blockmap+xml"/>`)
	b.WriteString("</Types>")
	return []byte(b.String())
}
