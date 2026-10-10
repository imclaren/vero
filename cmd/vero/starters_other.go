package main

// The other starters: Android, iOS, the browser, WASI and Plan 9. On iOS
// and in a browser the worker runs inside the app, so those two ask for
// the worker as a package with a Serve function, which PORTING.md explains.

func init() {
	starters["android"] = &starter{folder: "android", note: "an Android activity in Kotlin; needs the Android SDK, a JDK and kotlinc (vero's scripts/setup-android.sh)", files: map[string]string{
		"README.md": androidReadme, "AndroidManifest.xml": androidManifest, "src/{{pkgPath .ID}}/MainActivity.kt": androidActivity, "build.sh": androidBuild, ".gitignore": buildIgnore}, bindings: []string{"bindings/kotlin/Vero.kt"}, toml: "# The Android front end, which vero add wrote: its build makes an aligned,\n# unsigned .apk, which vero signs.\n[android]\nfolder = \"android\"\nbuild = \"./build.sh --build\"\napk = \".build/app-aligned.apk\"\n"}
	starters["ios"] = &starter{folder: "ios", note: "a SwiftUI app for the iOS Simulator, with the worker compiled into it; needs Xcode", limited: true, files: map[string]string{
		"README.md": iosReadme, "Sources/{{pascal .Name}}/App.swift": iosApp, "archive/archive.go": iosArchive, "archive/shim.go": iosShim, "build.sh": iosBuild, ".gitignore": buildIgnore}, toml: "# The iOS front end, which vero add wrote: its build makes an app for the\n# Simulator; an .ipa needs your Apple identity (vero-repo credentials).\n[ios]\nfolder = \"ios\"\nbuild = \"./build.sh --build\"\nsimulator_app = \".build/{{pascal .Name}}.app\"\n"}
	starters["web"] = &starter{folder: "web", note: "a page, with the worker compiled into the same wasm; needs only Go", limited: true, files: map[string]string{
		"README.md": webReadme, "main.go": webMain, "index.html": webIndex, "serve.go": webServe, "build.sh": webBuild, ".gitignore": webIgnore}, toml: "# The browser front end, which vero add wrote: the site serves what its\n# build makes.\n[web]\nfolder = \"web\"\nbuild = \"./build.sh\"\nfiles = [\"index.html\", \"main.wasm\", \"wasm_exec.js\"]\n"}
	starters["wasi"] = &starter{folder: "wasi", note: "a terminal front end that runs the worker as worker.wasm under wasmtime, which answers one request at a time; needs Go and wasmtime", files: map[string]string{
		"README.md": wasiReadme, "main.go": wasiMain, "build.sh": wasiBuild, ".gitignore": wasiIgnore}, toml: "# The WASI front end, which vero add wrote: the worker built as worker.wasm,\n# bundled with the front end for each desktop.\n[wasi]\nfrontend = \"wasi\"\nworker = \"{{.Worker}}\"\n"}
	starters["plan9"] = &starter{folder: "plan9", note: "a rio window drawn with libdraw, in Go; its own module, since it needs a Plan 9 drawing library; needs Go, and a Plan 9 to run it on (vero's scripts/run-plan9.sh)", files: map[string]string{
		"README.md": plan9Readme, "main.go": plan9Main, "go.mod": plan9Mod, ".gitignore": plan9Ignore}, toml: "# The Plan 9 front end, which vero add wrote: bundled with the worker and\n# an install script.\n[plan9]\nfrontend = \"plan9\"\n"}
}

// --- Android ---------------------------------------------------------------

const androidReadme = `# {{.Display}} for Android

An Android front end in Kotlin, written by ` + "`vero add`" + ` for the worker in
` + "`../{{.Worker}}`" + `. It shows the state the worker pushes and offers each of its
requests; make it your own from there. The worker ships inside the APK as a
native library, which is the one place Android runs a program from.

    ./build.sh            builds and runs it in the emulator
    ./build.sh --build    builds the APK only

It needs the Android SDK, a JDK and kotlinc: vero's ` + "`scripts/setup-android.sh`" + `
installs them. ` + "`Vero.kt`" + ` is vero's Kotlin binding, copied here by vero add.
`

const androidManifest = `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="{{.ID}}">
    <uses-permission android:name="android.permission.INTERNET" />
    <!-- extractNativeLibs puts the worker on disk as a real file, which is
         what makes it executable. -->
    <application
        android:label="{{.Display}}"
        android:extractNativeLibs="true"
        android:theme="@android:style/Theme.Material.Light.NoActionBar">
        <activity android:name=".MainActivity" android:exported="true">
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
                <category android:name="android.intent.category.LAUNCHER" />
            </intent-filter>
        </activity>
    </application>
</manifest>
`

const androidActivity = `// {{.Display}}: an Android front end for the worker in ../{{.Worker}}.
// Written by vero add: the worker's state as rows, and a control for each
// request. Make it your own.
package {{.ID}}

import android.app.Activity
import android.graphics.Color
import android.os.Bundle
import android.view.ViewGroup.LayoutParams.MATCH_PARENT
import android.view.ViewGroup.LayoutParams.WRAP_CONTENT
import android.view.WindowInsets
import android.widget.Button
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import com.imclaren.vero.NotRunning
import com.imclaren.vero.Vero
import com.imclaren.vero.VeroException
import org.json.JSONArray
import org.json.JSONObject
import kotlin.concurrent.thread

class MainActivity : Activity() {
    private lateinit var vero: Vero
    private lateinit var tree: LinearLayout
    private lateinit var problem: TextView
    private val entries = mutableMapOf<String, List<Triple<String, String, EditText>>>()

    // The requests the worker handles, and what each takes.
    private val requests = listOf(
{{- range .Requests}}
        "{{.Name}}" to listOf({{range $i, $f := .Fields}}{{if $i}}, {{end}}"{{$f.JSON}}" to "{{$f.Kind}}"{{end}}),
{{- end}}
    )

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val page = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(32, 48, 32, 32)
            setOnApplyWindowInsetsListener { view, insets ->
                val bars = insets.getInsets(WindowInsets.Type.systemBars())
                view.setPadding(32, bars.top + 48, 32, bars.bottom + 32)
                insets
            }
        }
        tree = LinearLayout(this).apply { orientation = LinearLayout.VERTICAL }
        page.addView(tree)
        for ((name, fields) in requests) {
            val row = LinearLayout(this).apply { orientation = LinearLayout.HORIZONTAL }
            row.addView(Button(this).apply { text = name; setOnClickListener { send(name) } })
            entries[name] = fields.map { (key, kind) ->
                val box = EditText(this).apply { hint = "$key ($kind)"; layoutParams = LinearLayout.LayoutParams(0, WRAP_CONTENT, 1f) }
                row.addView(box)
                Triple(key, kind, box)
            }
            page.addView(row)
        }
        problem = TextView(this).apply { setTextColor(Color.GRAY) }
        page.addView(problem)
        setContentView(ScrollView(this).apply { addView(page, MATCH_PARENT, WRAP_CONTENT) })

        vero = Vero(Vero.workerIn(applicationInfo.nativeLibraryDir))
        thread {
            val status = try { vero.latest() } catch (notYet: NotRunning) { null }
            status?.let { runOnUiThread { apply(it) } }
            vero.events { event -> event?.let { runOnUiThread { apply(it) } } }
        }
    }

    override fun onDestroy() {
        thread { vero.stop() }
        super.onDestroy()
    }

    // Redraws the state as rows: scalars as "key: value", lists and nested
    // objects indented under their key.
    private fun apply(status: JSONObject) {
        tree.removeAllViews()
        rows(status, 0)
    }

    private fun rows(value: Any?, depth: Int) {
        when (value) {
            is JSONObject -> for (key in value.keys()) {
                val v = value.get(key)
                if (v is JSONObject || v is JSONArray) { row("$key:", depth, true); rows(v, depth + 1) } else row("$key: $v", depth)
            }
            is JSONArray -> for (i in 0 until value.length()) {
                val v = value.get(i)
                if (v is JSONObject) {
                    row(v.keys().asSequence().filter { v.get(it) !is JSONObject && v.get(it) !is JSONArray }.joinToString(", ") { "$it: ${v.get(it)}" }, depth)
                } else row(v.toString(), depth)
            }
        }
    }

    private fun row(text: String, depth: Int, bold: Boolean = false) {
        tree.addView(TextView(this).apply {
            this.text = text
            setPadding(depth * 32, 4, 0, 4)
            if (bold) setTypeface(typeface, android.graphics.Typeface.BOLD)
        })
    }

    // Sends a request with what its entries hold, and draws the reply.
    private fun send(name: String) {
        val data = JSONObject()
        for ((key, kind, box) in entries[name] ?: emptyList()) {
            val t = box.text.toString()
            when (kind) {
                "int" -> data.put(key, t.toLongOrNull() ?: 0)
                "float" -> data.put(key, t.toDoubleOrNull() ?: 0.0)
                "bool" -> data.put(key, t.lowercase() in listOf("1", "true", "yes"))
                else -> data.put(key, t)
            }
        }
        thread {
            try {
                val reply = vero.call(name, data)
                runOnUiThread { problem.text = ""; reply?.let { apply(it) } }
            } catch (e: VeroException) {
                runOnUiThread { problem.text = e.message }
            }
        }
    }
}
`

const androidBuild = `#!/bin/sh
# Builds {{.Display}} for Android and runs it in the emulator.
#
#   ./build.sh            build, install, launch
#   ./build.sh --build    build only
#
# No Gradle: the pieces are the ones it would call. What this needs from
# the Android SDK is in vero's scripts/setup-android.sh.
set -e
cd "$(dirname "$0")"

# Homebrew keeps its JDK off the PATH, and macOS's own java is a stub that
# only says so; kotlinc and d8 need a real one.
if [ -z "${JAVA_HOME:-}" ] && [ -x /opt/homebrew/opt/openjdk/bin/java ]; then
    export JAVA_HOME=/opt/homebrew/opt/openjdk
fi
[ -n "${JAVA_HOME:-}" ] && PATH="$JAVA_HOME/bin:$PATH"

SDK=${ANDROID_HOME:-$HOME/Library/Android/sdk}
API=${ANDROID_API:-35}
ABI=arm64-v8a
PKG={{.ID}}
BUILD=.build

TOOLS=$(ls -d "$SDK"/build-tools/* 2>/dev/null | sort -V | tail -1)
JAR="$SDK/platforms/android-$API/android.jar"
[ -x "$TOOLS/aapt2" ] || { echo "no build-tools in $SDK - run vero's scripts/setup-android.sh" >&2; exit 1; }
[ -f "$JAR" ] || { echo "no android-$API platform in $SDK - run vero's scripts/setup-android.sh" >&2; exit 1; }

rm -rf "$BUILD"
mkdir -p "$BUILD/lib/$ABI" "$BUILD/classes" "$BUILD/dex"

echo "building the worker"
# Android runs executables from the app's native library folder, and only
# files named lib*.so go there, so the worker is libworker.so. Pure Go, so
# no NDK is needed.
CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -o "$BUILD/lib/$ABI/libworker.so" ../{{.Worker}}

echo "compiling the front end"
kotlinc -nowarn -classpath "$JAR" Vero.kt src/{{pkgPath .ID}}/MainActivity.kt \
    -d "$BUILD/classes" 2>&1 | grep -v "^warning:" || true

echo "dexing"
STDLIB=$(dirname "$(readlink "$(command -v kotlinc)" || command -v kotlinc)")/../lib/kotlin-stdlib.jar
[ -f "$STDLIB" ] || STDLIB=/opt/homebrew/opt/kotlin/libexec/lib/kotlin-stdlib.jar
# d8's and apksigner's warnings about Kotlin metadata and Java's native
# access are noise; their output is shown only when they fail.
quietly() { if ! "$@" >"$BUILD/tool.log" 2>&1; then cat "$BUILD/tool.log" >&2; exit 1; fi; }
quietly "$TOOLS/d8" --lib "$JAR" --min-api 24 --output "$BUILD/dex" "$STDLIB" $(find "$BUILD/classes" -name '*.class')

echo "packaging"
"$TOOLS/aapt2" link -I "$JAR" --manifest AndroidManifest.xml --min-sdk-version 24 --target-sdk-version "$API" -o "$BUILD/app.apk"
( cd "$BUILD" && zip -q app.apk classes.dex -j dex/classes.dex && zip -q app.apk "lib/$ABI/libworker.so" )

# A debug key, made once and kept.
KEYSTORE=$HOME/.android/debug.keystore
if [ ! -f "$KEYSTORE" ]; then
    mkdir -p "$(dirname "$KEYSTORE")"
    keytool -genkeypair -keystore "$KEYSTORE" -storepass android -keypass android \
        -alias androiddebugkey -dname "CN=Android Debug,O=Android,C=US" -keyalg RSA -keysize 2048 -validity 10000 >/dev/null
fi
"$TOOLS/zipalign" -f 4 "$BUILD/app.apk" "$BUILD/app-aligned.apk"
quietly "$TOOLS/apksigner" sign --ks "$KEYSTORE" --ks-pass pass:android --out "$BUILD/{{.Name}}.apk" "$BUILD/app-aligned.apk"
echo "  $BUILD/{{.Name}}.apk"

[ "$1" = "--build" ] && exit 0
ADB="$SDK/platform-tools/adb"
"$ADB" wait-for-device
"$ADB" install -r "$BUILD/{{.Name}}.apk"
"$ADB" shell am start -n "$PKG/.MainActivity" >/dev/null
echo "running on $("$ADB" shell getprop ro.product.model | tr -d '\r')"
`

// --- iOS -------------------------------------------------------------------

const iosReadme = `# {{.Display}} for iOS

A SwiftUI app, written by ` + "`vero add`" + ` for the worker in ` + "`../{{.Worker}}`" + `, built
for the iOS Simulator. It shows the state the worker pushes and offers each
of its requests; make it your own from there.

iOS does not let an app start a program, so the worker is compiled into
the app and runs on a goroutine. {{if .ServeImport}}It is the same code the other systems
start as a program, from ` + "`{{.ServeImport}}`" + `, which ` + "`archive/archive.go`" + ` runs.{{else}}That needs the worker as a package with a
` + "`Serve(in io.Reader, out io.Writer) error`" + ` function that sets up the same
worker as its ` + "`main`" + ` does, with ` + "`vero.WorkerOptions{In: in, Out: out}`" + `:
` + "`archive/archive.go`" + ` says where to call it. Until then the app runs a
placeholder worker with an empty state.{{end}}

    ./build.sh            builds, installs in the Simulator and launches
    ./build.sh --build    builds only

Shipping to phones needs an Apple Developer account: an Xcode project
that uses ` + "`Sources/{{pascal .Name}}/App.swift`" + ` and links the archive, signed with
your team, through TestFlight or the App Store.
`

const iosArchive = `//go:build ios

// Package main builds, with shim.go, the C archive the app links: the
// worker, run on a goroutine inside the app, since iOS lets no app start
// a program. shim.go is vero's cshim/main.go, which build.sh copies here;
// it has the package's main.
//
{{if .ServeImport -}}
// The worker is {{.ServeImport}}: the same code the other systems start
// as a program.
package main

import (
	"github.com/imclaren/vero"

	worker "{{.ServeImport}}"
)

func init() {
	vero.ServeInProcess(worker.Serve)
}
{{- else -}}
// Make a package of your worker with a Serve function - the body of its
// main, taking vero.WorkerOptions{In: in, Out: out} - and call it below.
package main

import (
	"io"

	"github.com/imclaren/vero"
)

func init() {
	// Replace this placeholder with your worker's Serve.
	vero.ServeInProcess(func(in io.Reader, out io.Writer) error {
		w := vero.NewWorker(vero.WorkerOptions{In: in, Out: out, Version: "0.0.0"})
		vero.NewState(w, struct{}{})
		return w.Serve()
	})
}
{{- end}}
`

const iosShim = `//go:build ignore

// shim.go is replaced by vero's cshim/main.go when build.sh runs: the C
// functions the Swift side calls, and the package's main.
package main
`

const iosBuild = `#!/bin/sh
# Builds {{.Display}} for the iOS Simulator and runs it there.
#
#   ./build.sh            builds, installs and launches
#   ./build.sh --build    builds only
set -e
cd "$(dirname "$0")"

VERO=$(cd .. && go list -m -f '{{"{{"}}.Dir{{"}}"}}' github.com/imclaren/vero)
BUILD=.build
APP="$BUILD/{{pascal .Name}}.app"
BUNDLE_ID={{.ID}}
DEVICE=${DEVICE:-"iPhone 17 Pro"}
MIN_IOS=17.0
SDK=$(xcrun --sdk iphonesimulator --show-sdk-path)
TARGET=arm64-apple-ios$MIN_IOS-simulator
mkdir -p "$BUILD"

echo "building libvero.a for the Simulator"
# GOOS=ios builds for a device; the flags point the same build at the
# Simulator SDK instead, which is what makes this runnable without a phone.
# install rather than cp: the module cache is read-only, and a copy made
# from it on the last run would refuse the next.
install -m 644 "$VERO/cshim/main.go" archive/shim.go
CGO_ENABLED=1 GOOS=ios GOARCH=arm64 \
    CC="$(xcrun --sdk iphonesimulator --find clang)" \
    CGO_CFLAGS="-isysroot $SDK -target $TARGET" \
    CGO_LDFLAGS="-isysroot $SDK -target $TARGET" \
    go build -buildmode=c-archive -o "$BUILD/libvero.a" ./archive

echo "building the app"
# CVero as a Clang module, so that ` + "`import Vero`" + ` finds the archive's symbols;
# then vero's Swift package as a module of its own, so that the app imports
# it exactly as it would through SwiftPM. Not libVero.a: the Mac's
# filesystem does not tell that from the Go archive's libvero.a.
install -m 644 "$VERO/Sources/CVero/include/CVero.h" "$BUILD/"
cat > "$BUILD/module.modulemap" <<MAP
module CVero {
    header "CVero.h"
    export *
}
MAP
xcrun --sdk iphonesimulator swiftc -emit-module -emit-library -static -module-name Vero \
    -target "$TARGET" -sdk "$SDK" -O \
    -Xcc -fmodule-map-file="$PWD/$BUILD/module.modulemap" -I "$BUILD" \
    "$VERO"/Sources/Vero/*.swift \
    -emit-module-path "$BUILD/Vero.swiftmodule" -o "$BUILD/libVeroSwift.a"
rm -rf "$APP" && mkdir -p "$APP"
xcrun --sdk iphonesimulator swiftc -parse-as-library -target "$TARGET" -sdk "$SDK" -O \
    -Xcc -fmodule-map-file="$PWD/$BUILD/module.modulemap" -I "$BUILD" \
    Sources/{{pascal .Name}}/*.swift \
    -L "$BUILD" -lVeroSwift -lvero \
    -Xlinker -syslibroot -Xlinker "$SDK" \
    -o "$APP/{{pascal .Name}}"
cat > "$APP/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>$BUNDLE_ID</string>
<key>CFBundleName</key><string>{{.Display}}</string>
<key>CFBundleExecutable</key><string>{{pascal .Name}}</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>CFBundleVersion</key><string>1</string>
<key>CFBundleShortVersionString</key><string>1.0</string>
<key>MinimumOSVersion</key><string>$MIN_IOS</string>
<key>UILaunchScreen</key><dict/>
<key>UIDeviceFamily</key><array><integer>1</integer></array>
</dict></plist>
PLIST
echo "  $APP"
[ "$1" = "--build" ] && exit 0

xcrun simctl boot "$DEVICE" 2>/dev/null || true
open -a Simulator
xcrun simctl install "$DEVICE" "$APP"
xcrun simctl launch "$DEVICE" "$BUNDLE_ID"
`

const iosApp = `import SwiftUI
import Vero

// The shapes the worker pushes and takes, as in ../{{.Worker}}. A view of
// your own reads vero.state through these.
{{range $name, $s := .Types}}struct {{$s.Name}}: Decodable {
{{range $s.Fields}}    let {{camel .Name}}: {{swiftType .}}?
{{end}}    private enum CodingKeys: String, CodingKey {
{{range $s.Fields}}        case {{camel .Name}} = "{{.JSON}}"
{{end}}    }
}

{{end}}
/// Anything JSON, for the starter's generic rows.
enum JSONValue: Codable {
    case string(String), number(Double), bool(Bool), null, array([JSONValue]), object([String: JSONValue])
    init(from decoder: Decoder) throws {
        let c = try decoder.singleValueContainer()
        if c.decodeNil() { self = .null }
        else if let b = try? c.decode(Bool.self) { self = .bool(b) }
        else if let n = try? c.decode(Double.self) { self = .number(n) }
        else if let s = try? c.decode(String.self) { self = .string(s) }
        else if let a = try? c.decode([JSONValue].self) { self = .array(a) }
        else { self = .object(try c.decode([String: JSONValue].self)) }
    }
    func encode(to encoder: Encoder) throws {
        var c = encoder.singleValueContainer()
        switch self {
        case .string(let s): try c.encode(s)
        case .number(let n): try c.encode(n)
        case .bool(let b): try c.encode(b)
        case .null: try c.encodeNil()
        case .array(let a): try c.encode(a)
        case .object(let o): try c.encode(o)
        }
    }
}

{{range .Requests}}struct {{pascal .Name}}Request: NamedRequest {
    static let name = "{{.Name}}"
    typealias Reply = JSONValue
{{range .Fields}}    let {{camel .Name}}: {{swiftReqType .}}
{{end}}{{if .Fields}}    private enum CodingKeys: String, CodingKey {
{{range .Fields}}        case {{camel .Name}} = "{{.JSON}}"
{{end}}    }
{{end}}}
{{end}}
let requests: [(name: String, fields: [(key: String, kind: String)])] = [
{{- range .Requests}}
    ("{{.Name}}", [{{range .Fields}}("{{.JSON}}", "{{.Kind}}"), {{end}}]),
{{- end}}
]

@main
struct {{pascal .Name}}App: App {
    // No worker to name: the archive this links carries it, and vero runs
    // it on a goroutine.
    @StateObject private var vero = VeroModel<JSONValue>(compiledInWorker: [])

    var body: some Scene {
        WindowGroup {
            NavigationStack {
                StarterView().environmentObject(vero).navigationTitle("{{.Display}}")
            }
        }
    }
}

/// The state as rows, and a control for each request. Written by vero add;
/// make it your own.
struct StarterView: View {
    @EnvironmentObject var vero: VeroModel<JSONValue>
    @State private var entries: [String: [String: String]] = [:]

    var body: some View {
        List {
            if let p = vero.problem { Text(p).foregroundStyle(.red) }
            if let s = vero.state { rows(s, depth: 0) }
            ForEach(requests, id: \.name) { r in
                HStack {
                    Button(r.name) { send(r.name) }.buttonStyle(.bordered)
                    ForEach(r.fields, id: \.key) { f in
                        TextField("\(f.key) (\(f.kind))", text: Binding(
                            get: { entries[r.name]?[f.key] ?? "" },
                            set: { entries[r.name, default: [:]][f.key] = $0 }))
                    }
                }
            }
        }
    }

    @ViewBuilder private func rows(_ v: JSONValue, depth: Int) -> some View {
        switch v {
        case .object(let o):
            ForEach(o.keys.sorted(), id: \.self) { k in
                switch o[k]! {
                case .object, .array:
                    Text("\(k):").bold().padding(.leading, CGFloat(depth * 16))
                    AnyView(rows(o[k]!, depth: depth + 1))
                default:
                    Text("\(k): \(text(o[k]!))").padding(.leading, CGFloat(depth * 16))
                }
            }
        case .array(let a):
            ForEach(Array(a.enumerated()), id: \.offset) { _, item in
                if case .object(let o) = item {
                    Text(o.keys.sorted().compactMap { k in
                        if case .object = o[k]! { return nil }
                        if case .array = o[k]! { return nil }
                        return "\(k): \(text(o[k]!))"
                    }.joined(separator: ", ")).padding(.leading, CGFloat(depth * 16))
                } else {
                    Text(text(item)).padding(.leading, CGFloat(depth * 16))
                }
            }
        default:
            Text(text(v))
        }
    }

    private func text(_ v: JSONValue) -> String {
        switch v {
        case .string(let s): return s
        case .number(let n): return n == n.rounded() ? String(Int(n)) : String(n)
        case .bool(let b): return b ? "true" : "false"
        case .null: return "-"
        default: return "…"
        }
    }

    private func entry(_ name: String, _ key: String) -> String { entries[name]?[key] ?? "" }
    private func int(_ name: String, _ key: String) -> Int { Int(entry(name, key)) ?? 0 }
    private func double(_ name: String, _ key: String) -> Double { Double(entry(name, key)) ?? 0 }
    private func bool(_ name: String, _ key: String) -> Bool { ["1", "true", "yes"].contains(entry(name, key).lowercased()) }

    private func send(_ name: String) {
        switch name {
{{- range $r := .Requests}}
        case "{{$r.Name}}":
            vero.call({{pascal $r.Name}}Request({{range $i, $f := $r.Fields}}{{if $i}}, {{end}}{{camel $f.Name}}: {{if eq $f.Kind "int"}}int("{{$r.Name}}", "{{$f.JSON}}"){{else if eq $f.Kind "float"}}double("{{$r.Name}}", "{{$f.JSON}}"){{else if eq $f.Kind "bool"}}bool("{{$r.Name}}", "{{$f.JSON}}"){{else if eq $f.Kind "string"}}entry("{{$r.Name}}", "{{$f.JSON}}"){{else}}.null{{end}}{{end}}))
{{- end}}
        default:
            break
        }
    }
}
`

// --- the browser -----------------------------------------------------------

const webReadme = `# {{.Display}} in a browser

A page, written by ` + "`vero add`" + ` for the worker in ` + "`../{{.Worker}}`" + `. It shows the
state the worker pushes and offers each of its requests; make it your own.

A page cannot start a program, so the worker is compiled into the same wasm
as the page and runs on a goroutine. {{if .ServeImport}}It is the same code the other systems
start as a program, from ` + "`{{.ServeImport}}`" + `, which ` + "`main.go`" + ` runs.{{else}}That needs the worker as a package
with a ` + "`Serve(in io.Reader, out io.Writer) error`" + ` function that sets up the
same worker as its ` + "`main`" + ` does, with ` + "`vero.WorkerOptions{In: in, Out: out}`" + `:
` + "`main.go`" + ` says where to call it. Until then the page runs a placeholder
worker with an empty state.{{end}}

    ./build.sh && go run serve.go    builds main.wasm and serves the page at http://localhost:8080
`

const webBuild = `#!/bin/sh
# Builds the page's wasm, and copies Go's loader beside it.
set -e
cd "$(dirname "$0")"
GOOS=js GOARCH=wasm go build -o main.wasm .
install -m 644 "$(go env GOROOT)/lib/wasm/wasm_exec.js" .
echo "built main.wasm ($(du -h main.wasm | cut -f1)): go run serve.go"
`

const webServe = `//go:build ignore

// Serves this folder: go run serve.go
package main

import (
	"log"
	"net/http"
)

func main() {
	log.Println("http://localhost:8080")
	log.Fatal(http.ListenAndServe("localhost:8080", http.FileServer(http.Dir("."))))
}
`

const webIndex = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>{{.Display}}</title>
<style>
  body { font: 15px system-ui, sans-serif; margin: 24px; max-width: 640px; }
  #tree div { padding: 2px 0; } #tree .b { font-weight: 600; }
  .req { margin: 8px 0; } .req input { margin-left: 6px; }
  #problem { color: #777; }
</style>
</head>
<body>
<h1>{{.Display}}</h1>
<div id="tree"></div>
<div id="requests"></div>
<p id="problem"></p>
<script src="wasm_exec.js"></script>
<script>
  const go = new Go();
  WebAssembly.instantiateStreaming(fetch("main.wasm"), go.importObject).then(r => go.run(r.instance));
</script>
</body>
</html>
`

const webMain = `//go:build js && wasm

// {{.Display}} in a browser: the page, and the worker it supervises in the
// same program, since a page cannot start one. Written by vero add: the
// worker's state as rows, and a control for each request. Make it your own.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"syscall/js"

	"github.com/imclaren/vero"
{{- if .ServeImport}}

	worker "{{.ServeImport}}"
{{- end}}
)

var document = js.Global().Get("document")

// The requests the worker handles, and what each takes.
var requests = []struct {
	name   string
	fields [][2]string
}{
{{- range .Requests}}
	{"{{.Name}}", [][2]string{ {{range .Fields}}{"{{.JSON}}", "{{.Kind}}"}, {{end}} }},
{{- end}}
}

func main() {
	entries := map[string][]js.Value{}
	var sup *vero.Supervisor
	sup = vero.Supervise(vero.SupervisorOptions{
		Serve:   serve,
		OnEvent: apply,
		OnLog:   func(line string) { js.Global().Get("console").Call("log", "worker: "+line) },
	})
	defer sup.Stop()
	if status := sup.Latest(); status != nil {
		apply(status)
	}
	box := document.Call("getElementById", "requests")
	for _, r := range requests {
		r := r
		row := document.Call("createElement", "div")
		row.Set("className", "req")
		button := document.Call("createElement", "button")
		button.Set("textContent", r.name)
		row.Call("appendChild", button)
		for _, f := range r.fields {
			input := document.Call("createElement", "input")
			input.Set("placeholder", f[0]+" ("+f[1]+")")
			row.Call("appendChild", input)
			entries[r.name] = append(entries[r.name], input)
		}
		button.Call("addEventListener", "click", js.FuncOf(func(js.Value, []js.Value) any {
			data := map[string]any{}
			for i, f := range r.fields {
				t := entries[r.name][i].Get("value").String()
				switch f[1] {
				case "int":
					data[f[0]], _ = strconv.Atoi(t)
				case "float":
					data[f[0]], _ = strconv.ParseFloat(t, 64)
				case "bool":
					data[f[0]] = t == "1" || t == "true" || t == "yes"
				default:
					data[f[0]] = t
				}
			}
			go func() {
				reply, err := sup.Call(context.Background(), r.name, data)
				if err != nil {
					document.Call("getElementById", "problem").Set("textContent", err.Error())
					return
				}
				document.Call("getElementById", "problem").Set("textContent", "")
				apply(reply)
			}()
			return nil
		}))
		box.Call("appendChild", row)
	}
	<-make(chan struct{})
}

// apply redraws the state as rows: scalars as "key: value", lists and
// nested objects indented under their key.
func apply(state json.RawMessage) {
	var v any
	if json.Unmarshal(state, &v) != nil {
		return
	}
	tree := document.Call("getElementById", "tree")
	tree.Set("innerHTML", "")
	rows(tree, v, 0)
}

func rows(tree js.Value, v any, depth int) {
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			switch x[k].(type) {
			case map[string]any, []any:
				row(tree, k+":", depth, true)
				rows(tree, x[k], depth+1)
			default:
				row(tree, k+": "+text(x[k]), depth, false)
			}
		}
	case []any:
		for _, item := range x {
			if o, ok := item.(map[string]any); ok {
				var parts []string
				keys := make([]string, 0, len(o))
				for k := range o {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					switch o[k].(type) {
					case map[string]any, []any:
					default:
						parts = append(parts, k+": "+text(o[k]))
					}
				}
				row(tree, strings.Join(parts, ", "), depth, false)
			} else {
				row(tree, text(item), depth, false)
			}
		}
	}
}

func row(tree js.Value, s string, depth int, bold bool) {
	div := document.Call("createElement", "div")
	div.Set("textContent", s)
	div.Get("style").Set("marginLeft", fmt.Sprintf("%dpx", depth*16))
	if bold {
		div.Set("className", "b")
	}
	tree.Call("appendChild", div)
}

func text(v any) string {
	switch x := v.(type) {
	case nil:
		return "-"
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return fmt.Sprint(v)
}

{{if .ServeImport -}}
// serve is the worker, run in this program: the same code the other
// systems start as a program, from {{.ServeImport}}.
func serve(in io.Reader, out io.Writer) error {
	return worker.Serve(in, out)
}
{{- else -}}
// serve is the worker, run in this program. Replace this placeholder with
// your worker's Serve.
func serve(in io.Reader, out io.Writer) error {
	w := vero.NewWorker(vero.WorkerOptions{In: in, Out: out, Version: "0.0.0"})
	vero.NewState(w, struct{}{})
	return w.Serve()
}
{{- end}}
`

// --- WASI ------------------------------------------------------------------

const wasiReadme = `# {{.Display}} on WASI

A terminal front end, written by ` + "`vero add`" + `, that runs the worker in
` + "`../{{.Worker}}`" + ` as ` + "`worker.wasm`" + ` under a WASI runtime: one file that runs on
any system the runtime does, boxed in so that it sees only what it is
given. A WASI worker answers one request at a time and does nothing in
between, so work that runs in a goroutine on other systems runs inside a
request handler here.

    ./build.sh                    builds worker.wasm and this program
    ./{{.Name}}-wasi              runs it, with wasmtime on the PATH
`

const wasiBuild = `#!/bin/sh
set -e
cd "$(dirname "$0")"
GOOS=wasip1 GOARCH=wasm go build -o worker.wasm ../{{.Worker}}
go build -o {{.Name}}-wasi .
echo "built: ./{{.Name}}-wasi  (needs wasmtime)"
`

const wasiMain = `// {{.Display}} on WASI: a terminal front end for worker.wasm. Written by
// vero add: it prints the worker's state as it changes, and sends a request
// typed as its name and, for what the request takes, key=value pairs.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/imclaren/vero"
)

func main() {
	worker := flag.String("worker", "worker.wasm", "the worker, as a wasm file")
	runtime := flag.String("runtime", "wasmtime", "the WASI runtime to run it with")
	flag.Parse()

	sup := vero.Supervise(vero.SupervisorOptions{
		Path:    *runtime,
		Args:    []string{"run", "--env", "VERO_SERVE=1", *worker},
		Lock:    *worker,
		OnEvent: func(e json.RawMessage) { show(e) },
	})
	defer sup.Stop()
	if err := sup.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "vero: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("{{.Display}}: requests are{{range .Requests}} {{.Name}}{{range .Fields}} {{.JSON}}=…{{end}};{{end}} q leaves.")
	deadline := time.Now().Add(10 * time.Second)
	for sup.Latest() == nil && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if s := sup.Latest(); s != nil {
		show(s)
	}
	in := bufio.NewScanner(os.Stdin)
	for in.Scan() {
		words := strings.Fields(in.Text())
		if len(words) == 0 || words[0] == "q" {
			break
		}
		data := map[string]any{}
		for _, w := range words[1:] {
			k, v, _ := strings.Cut(w, "=")
			if n, err := strconv.ParseFloat(v, 64); err == nil {
				data[k] = n
			} else if v == "true" || v == "false" {
				data[k] = v == "true"
			} else {
				data[k] = v
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		reply, err := sup.Call(ctx, words[0], data)
		cancel()
		if err != nil {
			fmt.Println("  ", err)
			continue
		}
		show(reply)
	}
}

// show prints the state as indented rows.
func show(state json.RawMessage) {
	var v any
	if json.Unmarshal(state, &v) != nil {
		return
	}
	fmt.Println()
	rows(v, 1)
}

func rows(v any, depth int) {
	pad := strings.Repeat("  ", depth)
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			switch x[k].(type) {
			case map[string]any, []any:
				fmt.Printf("%s%s:\n", pad, k)
				rows(x[k], depth+1)
			default:
				fmt.Printf("%s%s: %v\n", pad, k, x[k])
			}
		}
	case []any:
		for _, item := range x {
			if o, ok := item.(map[string]any); ok {
				var parts []string
				for k, val := range o {
					switch val.(type) {
					case map[string]any, []any:
					default:
						parts = append(parts, fmt.Sprintf("%s: %v", k, val))
					}
				}
				sort.Strings(parts)
				fmt.Printf("%s%s\n", pad, strings.Join(parts, ", "))
			} else {
				fmt.Printf("%s%v\n", pad, item)
			}
		}
	}
}
`

// --- Plan 9 ----------------------------------------------------------------

const plan9Readme = `# {{.Display}} on Plan 9

A rio window drawn with libdraw, in Go, written by ` + "`vero add`" + ` for the worker
in ` + "`../{{.Worker}}`" + `. It shows the worker's state as lines of text, and sends a
request when its name is typed (with key=value pairs for what it takes) and
Enter pressed. Plan 9 has no toolkit, so there are no widgets to grow it
from; it is a starting point for a text-first window. A module of its own,
since the drawing library is wanted by nothing else in your app.

    GOOS=plan9 GOARCH=amd64 go build -o {{.WorkerName}} ../{{.Worker}}
    GOOS=plan9 GOARCH=amd64 go build -o {{.Name}}-plan9 .

Copy both to a Plan 9 and run ` + "`{{.Name}}-plan9 {{.WorkerName}}`" + ` in rio. vero's
` + "`scripts/run-plan9.sh`" + ` boots one in qemu.
`

const plan9Mod = `module {{.Module}}/plan9

go 1.24

require (
	9fans.net/go v0.0.8-0.20260825183529-7dfa0e8c5041
	github.com/imclaren/vero v0.15.0
)

// The upstream libdraw port does not build for Plan 9: this fork does.
replace 9fans.net/go => github.com/imclaren/9fans-go v0.0.8-0.20261003011416-5961d14f794b
`

const plan9Main = `// {{.Display}} on Plan 9: the worker's state as lines in a rio window,
// and a line to type a request on. Written by vero add; make it your own.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"9fans.net/go/draw"
	"github.com/imclaren/vero"
)

func main() {
	worker := "{{.WorkerName}}"
	if len(os.Args) > 1 {
		worker = os.Args[1]
	}
	if dir, err := os.Getwd(); err == nil && !filepath.IsAbs(worker) {
		worker = filepath.Join(dir, worker)
	}
	d, err := draw.Init(nil, "", "{{.Display}}", "520x360")
	if err != nil {
		log.Fatalf("draw: %v", err)
	}
	ui := &ui{d: d}
	ui.fg, _ = d.AllocImage(image.Rect(0, 0, 1, 1), d.ScreenImage.Pix, true, draw.Black)
	ui.bg, _ = d.AllocImage(image.Rect(0, 0, 1, 1), d.ScreenImage.Pix, true, draw.White)

	events := make(chan []string, 16)
	sup := vero.Supervise(vero.SupervisorOptions{
		Path: worker,
		OnEvent: func(e json.RawMessage) {
			select {
			case events <- lines(e):
			default:
			}
		},
	})
	defer sup.Stop()
	if err := sup.Err(); err != nil {
		log.Fatalf("vero: %v", err)
	}
	mouse := d.InitMouse()
	kbd := d.InitKeyboard()
	ui.draw()
	for {
		select {
		case l := <-events:
			ui.lines = l
			ui.draw()
		case <-mouse.C:
		case <-mouse.Resize:
			if err := d.Attach(draw.RefNone); err == nil {
				ui.draw()
			}
		case r := <-kbd.C:
			switch r {
			case 0x7f: // Del
				return
			case '\n':
				words := strings.Fields(ui.typed)
				ui.typed = ""
				if len(words) > 0 {
					go func() {
						data := map[string]any{}
						for _, w := range words[1:] {
							k, v, _ := strings.Cut(w, "=")
							if n, err := strconv.ParseFloat(v, 64); err == nil {
								data[k] = n
							} else {
								data[k] = v
							}
						}
						ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
						defer cancel()
						if reply, err := sup.Call(ctx, words[0], data); err == nil {
							events <- lines(reply)
						} else {
							events <- []string{err.Error()}
						}
					}()
				}
			case '\b':
				if len(ui.typed) > 0 {
					ui.typed = ui.typed[:len(ui.typed)-1]
				}
			default:
				ui.typed += string(r)
			}
			ui.draw()
		}
	}
}

type ui struct {
	d      *draw.Display
	fg, bg *draw.Image
	lines  []string
	typed  string
}

func (u *ui) draw() {
	screen := u.d.ScreenImage
	screen.Draw(screen.R, u.bg, nil, draw.ZP)
	y := screen.R.Min.Y + 12
	for _, l := range u.lines {
		screen.String(image.Pt(screen.R.Min.X+12, y), u.fg, draw.ZP, u.d.Font, l)
		y += u.d.Font.Height + 2
	}
	prompt := "> " + u.typed + "_    ({{range .Requests}}{{.Name}}{{range .Fields}} {{.JSON}}=…{{end}}; {{end}}Del quits)"
	screen.String(image.Pt(screen.R.Min.X+12, screen.R.Max.Y-u.d.Font.Height-12), u.fg, draw.ZP, u.d.Font, prompt)
	u.d.Flush()
}

// lines is the state as indented lines of text.
func lines(state json.RawMessage) []string {
	var v any
	if json.Unmarshal(state, &v) != nil {
		return nil
	}
	var out []string
	var walk func(v any, depth int)
	walk = func(v any, depth int) {
		pad := strings.Repeat("  ", depth)
		switch x := v.(type) {
		case map[string]any:
			keys := make([]string, 0, len(x))
			for k := range x {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				switch x[k].(type) {
				case map[string]any, []any:
					out = append(out, pad+k+":")
					walk(x[k], depth+1)
				default:
					out = append(out, fmt.Sprintf("%s%s: %v", pad, k, x[k]))
				}
			}
		case []any:
			for _, item := range x {
				if o, ok := item.(map[string]any); ok {
					var parts []string
					for k, val := range o {
						switch val.(type) {
						case map[string]any, []any:
						default:
							parts = append(parts, fmt.Sprintf("%s: %v", k, val))
						}
					}
					sort.Strings(parts)
					out = append(out, pad+strings.Join(parts, ", "))
				} else {
					out = append(out, fmt.Sprintf("%s%v", pad, item))
				}
			}
		}
	}
	walk(v, 0)
	return out
}
`
