#if os(macOS) && canImport(AppKit)
import AppKit
import SwiftUI

/// The Dock icon of an app that lives in the menu bar, while it has a main
/// window open.
///
/// An app in the menu bar has no Dock icon, which vero ship arranges with
/// `menubar = true`. A main window that it opens then comes up behind other
/// apps and is missing from Cmd-Tab. So while one of its own windows is open
/// (the main window, or Settings), the app shows its Dock icon and menus,
/// and when the last one closes it goes back to the menu bar alone. The menu
/// bar's own small window does not count.
///
///     @main
///     struct ExampleApp: App {
///         init() {
///             VeroSingleCopy.yieldToRunningCopy()
///             VeroDock.follow()
///         }
///     }
///
/// and, from the menu bar's window, open the main window with
///
///     @Environment(\.openWindow) private var openWindow
///     Button("Open Example") { VeroDock.open("main", with: openWindow) }
@MainActor
public enum VeroDock {
    /// Windows whose identifier starts with one of these bring the Dock icon
    /// back. SwiftUI gives a `Window(id:)` scene's window that id, with more
    /// after it; Settings' window has "settings" in its identifier.
    public static var windows: [String] = ["main", "settings"]

    private static var observers: [NSObjectProtocol] = []
    private static var started: Date?
    /// Whether the main window has been asked for, by ``open(_:with:)``.
    static var asked = false

    /// Starts following the app's windows. Call it once, in `App.init`.
    public static func follow() {
        guard observers.isEmpty else { return }
        started = Date()
        let center = NotificationCenter.default
        observers = [
            center.addObserver(forName: NSWindow.didBecomeKeyNotification, object: nil, queue: .main) { _ in
                MainActor.assumeIsolated { update(closing: nil) }
            },
            center.addObserver(forName: NSWindow.willCloseNotification, object: nil, queue: .main) { note in
                let window = note.object as? NSWindow
                MainActor.assumeIsolated { update(closing: window) }
            },
        ]
    }

    /// Whether a window is one of the app's own, which bring the Dock icon.
    public static func isAppWindow(_ window: NSWindow) -> Bool {
        guard let id = window.identifier?.rawValue.lowercased() else { return false }
        return windows.contains { id.hasPrefix($0) || ($0 == "settings" && id.contains("settings")) }
    }

    /// Shows the Dock icon while an app window is open, and hides it when
    /// none is. closing is a window about to close, which no longer counts.
    public static func update(closing: NSWindow?) {
        let open = NSApp.windows.contains { $0 !== closing && $0.isVisible && isAppWindow($0) }
        let policy: NSApplication.ActivationPolicy = open ? .regular : .accessory
        guard NSApp.activationPolicy() != policy else { return }
        NSApp.setActivationPolicy(policy)
        if open { NSApp.activate(ignoringOtherApps: true) }
    }

    /// Brings the window with this id forward, opening it if it is not open,
    /// with the Dock icon, in front of other apps.
    @available(macOS 13, *)
    public static func open(_ id: String, with openWindow: OpenWindowAction) {
        asked = true
        if let window = NSApp.windows.first(where: { $0.identifier?.rawValue.hasPrefix(id) == true && $0.canBecomeMain }) {
            window.makeKeyAndOrderFront(nil)
        } else {
            openWindow(id: id)
        }
        NSApp.setActivationPolicy(.regular)
        NSApp.activate(ignoringOtherApps: true)
        // A window opened while the app was not active may never become key,
        // which is what update follows; check again once it is up.
        DispatchQueue.main.async { update(closing: nil) }
    }

    /// Whether a window that has just appeared opened by itself at launch,
    /// rather than being asked for.
    static func openedByItself() -> Bool {
        guard let started else { return false }
        return !asked && Date().timeIntervalSince(started) < 5
    }
}

extension View {
    /// For a menu bar app's main window: closes it if it opens by itself at
    /// launch, as SwiftUI does on macOS 13 and 14, which cannot be told not
    /// to, so that the app starts in the menu bar alone. It needs
    /// ``VeroDock/follow()``, which says when the app started. Once the window has
    /// been asked for, with ``VeroDock/open(_:with:)``, it stays.
    public func veroOpensWhenAsked() -> some View {
        background(VeroWindowReader { window in
            if VeroDock.openedByItself() {
                window.close()
            }
        })
    }
}

/// Finds the window a view is in, once it is in one.
struct VeroWindowReader: NSViewRepresentable {
    let found: @MainActor (NSWindow) -> Void

    func makeNSView(context: Context) -> NSView { Reader(found: found) }
    func updateNSView(_ nsView: NSView, context: Context) {}

    final class Reader: NSView {
        let found: @MainActor (NSWindow) -> Void
        init(found: @escaping @MainActor (NSWindow) -> Void) {
            self.found = found
            super.init(frame: .zero)
        }
        required init?(coder: NSCoder) { nil }
        override func viewDidMoveToWindow() {
            super.viewDidMoveToWindow()
            if let window { MainActor.assumeIsolated { found(window) } }
        }
    }
}
#endif
