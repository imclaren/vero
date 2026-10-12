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

/// Turns HTML into PDF files without showing it, one after another through
/// one web view that is never in a window, so that thousands can be written
/// in turn:
///
///     let pdf = VeroPDFWriter()
///     try await pdf.write(html: message.body, to: folder.appendingPathComponent("message.pdf"))
///
/// Each file is one page as long as the message, as WKWebView's createPDF
/// makes it. Scripts are off and nothing is loaded from the network unless
/// remote is set.
@MainActor
public final class VeroPDFWriter: NSObject, WKNavigationDelegate {
    public var remote: Bool
    public var policy: String
    private var view: WKWebView?
    private var loaded: CheckedContinuation<Void, Error>?
    private var last: Task<Error?, Never>?

    public init(remote: Bool = false, policy: String = VeroHTML.policy) {
        self.remote = remote
        self.policy = policy
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
        let view = self.view ?? {
            let v = VeroHTML.webView()
            v.navigationDelegate = self
            self.view = v
            return v
        }()
        try await withCheckedThrowingContinuation { (c: CheckedContinuation<Void, Error>) in
            loaded = c
            view.loadHTMLString(VeroHTML.withPolicy(html, policy: policy, remote: remote), baseURL: nil)
        }
        let data: Data = try await withCheckedThrowingContinuation { c in
            view.createPDF { result in c.resume(with: result) }
        }
        try data.write(to: url, options: .atomic)
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
