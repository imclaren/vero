// swift-tools-version:5.9
import PackageDescription

// A path dependency on the checkout, so this example exercises the working tree
// rather than the last release - see the README.  That means it has to supply
// the archive itself: build.sh builds libvero.a, and the unsafeFlags below link
// it.  An application takes a tagged version instead, and the package brings the
// archive with it.
let package = Package(
    name: "MenuBarExample",
    platforms: [.macOS(.v13)],
    dependencies: [.package(name: "vero", path: "../..")],
    targets: [
        .executableTarget(
            name: "MenuBarExample",
            dependencies: [.product(name: "Vero", package: "vero")],
            linkerSettings: [.unsafeFlags(["-L.", "-lvero"])]
        )
    ]
)
