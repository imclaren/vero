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
        // scripts/release.sh points this binaryTarget at each release's
        // CVero.xcframework.zip, so a tagged version carries the archive with
        // it and nobody has to build one.
        .binaryTarget(
            name: "CVero",
            url: "https://github.com/imclaren/vero/releases/download/v0.16.0/CVero.xcframework.zip",
            checksum: "fd346cbf1ed36ca1c3ae333a167e7e4307e803ab6b63e521367ed98e10a5beb8"
        ),
        .target(name: "Vero", dependencies: ["CVero"]),
    ]
)
