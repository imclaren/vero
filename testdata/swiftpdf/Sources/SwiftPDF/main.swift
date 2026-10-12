import AppKit
import Vero

// Writes two PDFs at once through VeroPDFWriter, offscreen, and checks that
// each is a PDF.
@MainActor
func check() async -> Bool {
    let folder = FileManager.default.temporaryDirectory.appendingPathComponent("vero-swift-pdf")
    try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
    let pdf = VeroPDFWriter()
    async let one: Void = pdf.write(html: "<h1>Message 1</h1><p>Hello.</p>", to: folder.appendingPathComponent("1.pdf"))
    async let two: Void = pdf.write(html: "<h1>Message 2</h1><p>Hello.</p>", to: folder.appendingPathComponent("2.pdf"))
    do {
        _ = try await (one, two)
    } catch {
        print("FAIL: \(error)")
        return false
    }
    for n in 1...2 {
        let data = (try? Data(contentsOf: folder.appendingPathComponent("\(n).pdf"))) ?? Data()
        guard data.starts(with: Array("%PDF-".utf8)) else { print("FAIL: PDF \(n) is not a PDF"); return false }
    }
    print("PASS: two PDFs written offscreen")
    return true
}

let app = NSApplication.shared
Task { @MainActor in exit(await check() ? 0 : 1) }
app.run()
