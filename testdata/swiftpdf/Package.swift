// swift-tools-version:5.9
import PackageDescription
let package = Package(
    name: "SwiftPDF",
    platforms: [.macOS(.v12)],
    dependencies: [.package(path: "../..")],
    targets: [.executableTarget(name: "SwiftPDF", dependencies: [.product(name: "Vero", package: "vero")])]
)
