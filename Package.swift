// swift-tools-version:5.9
import Foundation
import PackageDescription

// Between releases, main's Swift may need C functions that the last
// release's archive lacks. scripts/build-cvero.sh builds the archive from
// this checkout into dist/, and the package then uses that one. A checkout
// without it, as every tagged version is to an app that depends on it,
// uses the release's.
let localCVero = FileManager.default.fileExists(atPath: Context.packageDirectory + "/dist/CVero.xcframework")

let package = Package(
    name: "Vero",
    platforms: [.macOS(.v12)],
    products: [
        .library(name: "Vero", targets: ["Vero"]),
        // Sparkle, set up for a vero app. A separate product, so that an
        // app on the App Store, which may not update itself, leaves
        // Sparkle out.
        .library(name: "VeroUpdates", targets: ["VeroUpdates"]),
    ],
    dependencies: [
        .package(url: "https://github.com/sparkle-project/Sparkle", from: "2.5.1"),
    ],
    targets: [
        // The C archive built from ./cshim, which is the same for every
        // application: the worker's path arrives at runtime and every message
        // is JSON.
        //
        // scripts/release.sh points this binaryTarget at each release's
        // CVero.xcframework.zip, so a tagged version carries the archive with
        // it and nobody has to build one.
        !localCVero ? .binaryTarget(
            name: "CVero",
            url: "https://github.com/imclaren/vero/releases/download/v0.18.1/CVero.xcframework.zip",
            checksum: "ae7469843d22e4023f3fdb4b693624e23e6b045fcf76f32f6b333b11b3641ca3"
        ) : .binaryTarget(name: "CVero", path: "dist/CVero.xcframework"),
        .target(name: "Vero", dependencies: ["CVero"]),
        .target(name: "VeroUpdates", dependencies: [.product(name: "Sparkle", package: "Sparkle")]),
    ]
)
