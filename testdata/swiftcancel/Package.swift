// swift-tools-version:5.9
import PackageDescription
let package = Package(
    name: "SwiftCancel",
    platforms: [.macOS(.v12)],
    dependencies: [.package(path: "../..")],
    targets: [.executableTarget(name: "SwiftCancel", dependencies: [.product(name: "Vero", package: "vero")])]
)
