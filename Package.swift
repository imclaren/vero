// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "Vero",
    platforms: [.macOS(.v12)],
    products: [
        .library(name: "Vero", targets: ["Vero"]),
    ],
    targets: [
        // The C archive built from ./cshim, which is the same for every
        // application: the worker's path arrives at runtime and every message
        // is JSON.
        //
        // On main this is only the declarations, and the application links the
        // archive itself.  scripts/release.sh replaces this with a binaryTarget
        // pointing at the release's CVero.xcframework.zip, so a tagged version
        // carries the archive with it and nobody has to build one.
        .binaryTarget(
            name: "CVero",
            url: "https://github.com/imclaren/vero/releases/download/v0.12.0/CVero.xcframework.zip",
            checksum: "fad1b2e90ca87def6dbe5e3527ce3a36e7c485c1ce429ca5805df2294ada391f"
        ),
        .target(name: "Vero", dependencies: ["CVero"]),
    ]
)
