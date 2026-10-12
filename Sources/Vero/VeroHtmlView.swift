#if os(macOS)
import AppKit
import SwiftUI
import WebKit

/// What vero's web views allow a page, and the page with it.
public enum VeroHTML {
    /// What a page may load: its own styles, and pictures that come with it.
    public static let policy = "default-src 'none'; style-src 'unsafe-inline'; img-src data: cid:"

    /// The page with its Content Security Policy, which allows pictures from
    /// the web too when remote is set.
    public static func withPolicy(_ html: String, policy: String = policy, remote: Bool = false) -> String {
        var policy = policy
        if remote {
            policy = policy.contains("img-src")
                ? policy.replacingOccurrences(of: "img-src", with: "img-src https: http:")
                : policy + "; img-src https: http: data:"
        }
        let escaped = policy.replacingOccurrences(of: "&", with: "&amp;").replacingOccurrences(of: "\"", with: "&quot;")
        return "<meta http-equiv=\"Content-Security-Policy\" content=\"\(escaped)\">" + html
    }

    /// A web view with scripts off, which keeps nothing once it is gone.
    @MainActor
    static func webView() -> WKWebView {
        let configuration = WKWebViewConfiguration()
        configuration.websiteDataStore = .nonPersistent()
        configuration.defaultWebpagePreferences.allowsContentJavaScript = false
        return WKWebView(frame: NSRect(x: 0, y: 0, width: 800, height: 1000), configuration: configuration)
    }
}

/// Shows HTML, such as a message, with scripts off and nothing loaded from
/// the network unless remote is set. policy is the Content Security
/// Policy, `VeroHTML.policy` unless the app sets its own. Links to the web
/// and mailto: open in the browser. A link in one of the app's own schemes,
/// such as myapp-attachment:, goes to onLink, which returns true when it
/// has dealt with it.
///
///     VeroHtmlView(html: message.body, remote: allowsPictures) { url in
///         url.scheme == "myapp-attachment" ? open(url) : false
///     }
public struct VeroHtmlView: NSViewRepresentable {
    public var html: String
    public var remote: Bool
    public var policy: String
    public var onLink: ((URL) -> Bool)?

    public init(html: String, remote: Bool = false, policy: String = VeroHTML.policy, onLink: ((URL) -> Bool)? = nil) {
        self.html = html
        self.remote = remote
        self.policy = policy
        self.onLink = onLink
    }

    public func makeCoordinator() -> Coordinator { Coordinator() }

    public func makeNSView(context: Context) -> WKWebView {
        let view = VeroHTML.webView()
        view.navigationDelegate = context.coordinator
        return view
    }

    public func updateNSView(_ view: WKWebView, context: Context) {
        context.coordinator.onLink = onLink
        let page = VeroHTML.withPolicy(html, policy: policy, remote: remote)
        if context.coordinator.page != page {
            context.coordinator.page = page
            view.loadHTMLString(page, baseURL: nil)
        }
    }

    public final class Coordinator: NSObject, WKNavigationDelegate {
        var page = ""
        var onLink: ((URL) -> Bool)?

        public func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction,
                            decisionHandler: @escaping @MainActor @Sendable (WKNavigationActionPolicy) -> Void) {
            guard let url = action.request.url else { return decisionHandler(.cancel) }
            // The page itself, as loadHTMLString gives it.
            if action.navigationType == .other, url.scheme == "about" || url.scheme == "data" {
                return decisionHandler(.allow)
            }
            decisionHandler(.cancel)
            if onLink?(url) == true { return }
            if ["http", "https", "mailto"].contains(url.scheme?.lowercased() ?? "") {
                NSWorkspace.shared.open(url)
            }
        }
    }
}

/// A PDF's paper: the system's own, which follows the region and printer
/// (A4, or US Letter in the United States and Canada), or one named.
public enum VeroPaper: Sendable {
    case system, a4, letter

    /// Width and height in points.
    @MainActor
    var size: NSSize {
        switch self {
        case .system: return NSPrintInfo.shared.paperSize
        case .a4: return NSSize(width: 595.28, height: 841.89)
        case .letter: return NSSize(width: 612, height: 792)
        }
    }
}

/// Turns HTML into PDF files without showing it, one after another through
/// one web view in a window that is never seen, so that thousands can be
/// written in turn:
///
///     let pdf = VeroPDFWriter()
///     try await pdf.write(html: message.body, to: folder.appendingPathComponent("message.pdf"))
///
/// Each PDF has ordinary pages of paper, the system's own unless set, with
/// margin millimetres on every side, as vero's writers on Windows and
/// Linux make them. onePage makes instead one page as long as the message,
/// as WKWebView's createPDF does. Scripts are off and nothing is loaded
/// from the network unless remote is set.
@MainActor
public final class VeroPDFWriter: NSObject, WKNavigationDelegate {
    public var remote: Bool
    public var policy: String
    public var paper: VeroPaper
    public var margin: Double
    public var onePage: Bool
    private var view: WKWebView?
    private var window: NSWindow?
    private var loaded: CheckedContinuation<Void, Error>?
    private var printed: CheckedContinuation<Bool, Never>?
    private var last: Task<Error?, Never>?

    public init(remote: Bool = false, policy: String = VeroHTML.policy, paper: VeroPaper = .system,
                margin: Double = 12.7, onePage: Bool = false) {
        self.remote = remote
        self.policy = policy
        self.paper = paper
        self.margin = margin
        self.onePage = onePage
    }

    /// Writes html to url as a PDF. Writes asked for together run in turn.
    public func write(html: String, to url: URL) async throws {
        let previous = last
        let task = Task { @MainActor () -> Error? in
            _ = await previous?.value
            do { try await self.writeNow(html: html, to: url) } catch { return error }
            return nil
        }
        last = task
        if let error = await task.value { throw error }
    }

    private func writeNow(html: String, to url: URL) async throws {
        let view: WKWebView
        if let existing = self.view {
            view = existing
        } else {
            // Printing needs the web view in a window, so it has one far
            // off the screen, which is never shown.
            let window = NSWindow(contentRect: NSRect(x: -10000, y: -10000, width: 800, height: 1000),
                                  styleMask: [.titled], backing: .buffered, defer: false)
            window.isReleasedWhenClosed = false
            view = VeroHTML.webView()
            view.frame = window.contentView!.bounds
            window.contentView!.addSubview(view)
            view.navigationDelegate = self
            self.view = view
            self.window = window
        }
        try await withCheckedThrowingContinuation { (c: CheckedContinuation<Void, Error>) in
            loaded = c
            view.loadHTMLString(VeroHTML.withPolicy(html, policy: policy, remote: remote), baseURL: nil)
        }
        if onePage {
            let data: Data = try await withCheckedThrowingContinuation { c in
                view.createPDF { result in c.resume(with: result) }
            }
            try data.write(to: url, options: .atomic)
            return
        }
        try? FileManager.default.removeItem(at: url)
        let info = NSPrintInfo()
        info.paperSize = paper.size
        let points = margin * 72 / 25.4
        info.topMargin = points
        info.bottomMargin = points
        info.leftMargin = points
        info.rightMargin = points
        info.jobDisposition = .save
        info.dictionary()[NSPrintInfo.AttributeKey.jobSavingURL] = url
        info.horizontalPagination = .fit
        info.verticalPagination = .automatic
        let op = view.printOperation(with: info)
        op.showsPrintPanel = false
        op.showsProgressPanel = false
        op.view?.frame = view.bounds
        let ok = await withCheckedContinuation { (c: CheckedContinuation<Bool, Never>) in
            printed = c
            op.runModal(for: window!, delegate: self, didRun: #selector(didPrint(_:success:context:)), contextInfo: nil)
        }
        guard ok, FileManager.default.fileExists(atPath: url.path) else {
            throw VeroError.failed("the PDF was not written")
        }
    }

    @objc private func didPrint(_ op: NSPrintOperation, success: Bool, context: UnsafeMutableRawPointer?) {
        printed?.resume(returning: success)
        printed = nil
    }

    public func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        loaded?.resume()
        loaded = nil
    }

    public func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error) {
        loaded?.resume(throwing: error)
        loaded = nil
    }

    public func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) {
        loaded?.resume(throwing: error)
        loaded = nil
    }

    public func webView(_ webView: WKWebView, decidePolicyFor action: WKNavigationAction,
                        decisionHandler: @escaping @MainActor @Sendable (WKNavigationActionPolicy) -> Void) {
        // Nothing but the page: a link in it goes nowhere.
        let scheme = action.request.url?.scheme
        decisionHandler(scheme == "about" || scheme == "data" ? .allow : .cancel)
    }
}
#endif
