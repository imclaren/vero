import AppKit
import PDFKit
import Vero

// Writes PDFs through VeroPDFWriter, offscreen: two at once, a long message
// that must take several A4 pages, and the same on one long page.
@MainActor
func check() async -> Bool {
    let folder = FileManager.default.temporaryDirectory.appendingPathComponent("vero-swift-pdf")
    try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
    let long = "<h1>A long message</h1>" + String(repeating: "<p>A paragraph of a receipt, long enough to fill a line or two of the page.</p>", count: 150)
    let pdf = VeroPDFWriter(paper: .a4, margin: 10)
    async let one: Void = pdf.write(html: "<h1>Message 1</h1><p>Hello.</p>", to: folder.appendingPathComponent("1.pdf"))
    async let two: Void = pdf.write(html: long, to: folder.appendingPathComponent("2.pdf"))
    do {
        _ = try await (one, two)
        try await VeroPDFWriter(onePage: true).write(html: long, to: folder.appendingPathComponent("3.pdf"))
    } catch {
        print("FAIL: \(error)")
        return false
    }
    func pages(_ name: String) -> (Int, NSSize) {
        guard let doc = PDFDocument(url: folder.appendingPathComponent(name)), let page = doc.page(at: 0) else { return (0, .zero) }
        return (doc.pageCount, page.bounds(for: .mediaBox).size)
    }
    let (n1, size) = pages("1.pdf"), (n2, _) = pages("2.pdf"), (n3, _) = pages("3.pdf")
    guard n1 == 1, abs(size.width - 595.28) < 1, abs(size.height - 841.89) < 1 else {
        print("FAIL: the short message: \(n1) pages of \(size)"); return false
    }
    guard n2 > 1 else { print("FAIL: the long message is \(n2) page"); return false }
    guard n3 == 1 else { print("FAIL: onePage made \(n3) pages"); return false }
    print("PASS: PDFs written offscreen, the long one on \(n2) A4 pages, and on one page when asked")
    return true
}

let app = NSApplication.shared
Task { @MainActor in exit(await check() ? 0 : 1) }
app.run()
