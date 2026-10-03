#if os(macOS) && canImport(AppKit)
import AppKit
import Combine

/// One copy of an application at a time.
///
/// The copy in /Applications and the one still in ~/Downloads after an
/// update are two processes with the same bundle identifier, and macOS runs
/// both. The worker is safe from that - it holds a lock, and the second copy
/// is refused one (``VeroError/alreadyRunning(_:)``) - but the second copy
/// stays open all the same, with a second menu bar item and a window that can
/// do nothing. What somebody who opened the application twice wanted was the
/// application.
///
/// So the second copy brings the first one forward, asks it to show itself,
/// and quits:
///
///     @main
///     struct ExampleApp: App {
///         init() { VeroSingleCopy.yieldToRunningCopy() }
///
///         @StateObject private var vero = VeroModel<Status>(...)
///     }
///
/// and the first, if it has a window that may be closed, opens it when asked:
///
///     .onReceive(VeroSingleCopy.reopenRequests) { openWindow(id: "main") }
///
/// Two copies started in the same moment - one by a login item, one by hand -
/// do not see each other that early, and both go on. One of them then loses
/// the worker's lock, which ``VeroModel/isAlreadyRunning`` reports; calling
/// ``yieldToRunningCopy(bundleIdentifier:)`` again at that point quits the one
/// that lost and leaves the one that has the worker.
public enum VeroSingleCopy {

    /// Quits this process if another copy of the application is already
    /// running, having brought that copy forward and asked it to show itself.
    /// Returns, having done nothing, if this is the only copy.
    ///
    /// Call it first: in `App.init`, before anything starts a worker. It is
    /// safe that early, which the obvious way of writing it is not -
    /// `NSRunningApplication.current` has no process identifier until the
    /// application has registered with the system, so comparing against it
    /// there never matches, and the check passes with two copies running.
    ///
    /// Copies are matched by bundle identifier. A program with none - one run
    /// straight from a build folder - is matched by the path of its
    /// executable instead.
    public static func yieldToRunningCopy(bundleIdentifier: String? = Bundle.main.bundleIdentifier) {
        guard let other = runningCopy(bundleIdentifier: bundleIdentifier) else { return }

        DistributedNotificationCenter.default().postNotificationName(
            reopenNotification(bundleIdentifier: bundleIdentifier),
            object: nil, userInfo: nil, deliverImmediately: true)
        other.activate(options: [.activateAllWindows])
        print("vero: this application is already running (pid \(other.processIdentifier)); showing that copy and quitting this one.")

        // Before the application has started there is nothing to terminate,
        // and nothing running that needs to be told.
        if let app = NSApp, app.isRunning {
            app.terminate(nil)
        } else {
            exit(0)
        }
    }

    /// The other copy of this application that is running, if there is one.
    public static func runningCopy(bundleIdentifier: String? = Bundle.main.bundleIdentifier) -> NSRunningApplication? {
        // Not NSRunningApplication.current: see yieldToRunningCopy.
        let me = ProcessInfo.processInfo.processIdentifier
        let copies: [NSRunningApplication]
        if let bundleIdentifier {
            copies = NSRunningApplication.runningApplications(withBundleIdentifier: bundleIdentifier)
        } else {
            let executable = Bundle.main.executableURL?.resolvingSymlinksInPath()
            copies = NSWorkspace.shared.runningApplications.filter {
                executable != nil && $0.executableURL?.resolvingSymlinksInPath() == executable
            }
        }
        return copies.first { $0.processIdentifier != me && !$0.isTerminated }
    }

    /// Fires in the copy that is running each time a second copy was opened
    /// and quit in its favour. An application whose window can be closed
    /// while it goes on running - a menu bar application - opens it here;
    /// being brought forward does not open a window that is not there.
    public static var reopenRequests: AnyPublisher<Void, Never> {
        DistributedNotificationCenter.default()
            .publisher(for: reopenNotification(bundleIdentifier: Bundle.main.bundleIdentifier))
            .map { _ in () }
            .receive(on: DispatchQueue.main)
            .eraseToAnyPublisher()
    }

    /// What a second copy sends the first. Named for the application, so that
    /// two applications built on vero do not hear each other's.
    static func reopenNotification(bundleIdentifier: String?) -> Notification.Name {
        let application = bundleIdentifier
            ?? Bundle.main.executableURL?.resolvingSymlinksInPath().path
            ?? ProcessInfo.processInfo.processName
        return Notification.Name("vero.reopen." + application)
    }
}
#endif
