import AppKit
import Combine
import Foundation
import Sparkle

/// Sparkle, set up as a vero app wants it, for the views to observe.
///
/// vero-repo puts the appcast's address (SUFeedURL) and the public key
/// (SUPublicEDKey) in the app's Info.plist when it releases the app, so
/// there is nothing to configure here. What this adds to Sparkle is for an
/// app that people leave open, a menu bar app most of all:
///
/// - Nothing ever quits such an app, and Sparkle installs an update that it
///   has downloaded only when the app quits. So the update is installed as
///   soon as it is ready, silently, unless the app is busy (set `isBusy`)
///   or somebody is using it. Sparkle then asks, as it would have.
/// - An alert from an app that is not active opens behind every other
///   window. A scheduled update's question brings the app forward.
/// - A check that people ask for while Sparkle is already checking does
///   nothing, so `canCheckForUpdates` says when the button works.
///
/// Make one, keep it for the life of the app, and hand it to the views:
///
///     let updates = VeroUpdater()
///     Button("Check for Updates…") { updates.checkForUpdates() }
///         .disabled(!updates.canCheckForUpdates)
///
/// An app on the App Store leaves this product out, and Sparkle with it.
public final class VeroUpdater: ObservableObject {
    /// Sparkle itself, for anything this does not cover.
    public let updater: SPUUpdater

    private let controller: SPUStandardUpdaterController
    // Sparkle holds its delegates weakly, so this keeps ours alive.
    private let delegate: Delegate

    /// False while Sparkle already has a check, a download or an install in
    /// progress, when `checkForUpdates()` would do nothing.
    @Published public private(set) var canCheckForUpdates = false

    /// Whether the app checks for, downloads and installs its updates by
    /// itself. Sparkle keeps the choice between launches, and takes its
    /// first value from SUEnableAutomaticChecks and SUAutomaticallyUpdate
    /// in Info.plist. It is written only when it changes, so that a launch
    /// never overwrites what somebody chose.
    @Published public var automaticallyUpdates: Bool {
        didSet {
            guard automaticallyUpdates != oldValue else { return }
            updater.automaticallyChecksForUpdates = automaticallyUpdates
            updater.automaticallyDownloadsUpdates = automaticallyUpdates
            if automaticallyUpdates { checkInBackground() }
        }
    }

    /// While this is true, an update waits rather than restarting the app:
    /// set it while the app's worker is in the middle of something that a
    /// restart would interrupt, such as a sync or an import. It can be set
    /// from any thread.
    public var isBusy: Bool {
        get { delegate.busy.value }
        set { delegate.busy.value = newValue }
    }

    /// Starts Sparkle. busy is `isBusy` to begin with: true for an app that
    /// should not restart until its worker has said what it is doing.
    /// checkEvery is how often a scheduled check runs, and the app also
    /// checks once at launch when it updates automatically.
    public init(busy: Bool = false, checkEvery: TimeInterval = 4 * 60 * 60) {
        let delegate = Delegate()
        delegate.busy.value = busy
        self.delegate = delegate
        controller = SPUStandardUpdaterController(
            startingUpdater: true, updaterDelegate: delegate, userDriverDelegate: delegate)
        updater = controller.updater
        updater.updateCheckInterval = checkEvery
        // Assigned in init, so didSet does not write it back to Sparkle.
        automaticallyUpdates = updater.automaticallyChecksForUpdates
        updater.publisher(for: \.canCheckForUpdates).assign(to: &$canCheckForUpdates)
        if automaticallyUpdates { checkInBackground() }
    }

    /// Checks now, showing what it finds, even that the app is up to date.
    public func checkForUpdates() {
        // An app with no window of its own has nothing to come forward,
        // and Sparkle's panels open behind everything unless it is active.
        Self.activate()
        updater.checkForUpdates()
    }

    /// Sends the token with every check and download, for a site that
    /// answers only somebody on its list. An empty token sends none.
    public func signIn(token: String) {
        updater.httpHeaders = token.isEmpty ? nil : ["Authorization": "Bearer \(token)"]
    }

    private func checkInBackground() {
        if !updater.sessionInProgress { updater.checkForUpdatesInBackground() }
    }

    static func activate() {
        if #available(macOS 14.0, *) {
            NSApp.activate()
        } else {
            NSApp.activate(ignoringOtherApps: true)
        }
    }

    /// A value read from Sparkle's callbacks and written by the app, which
    /// may be on different threads.
    final class Locked<T>: @unchecked Sendable {
        private let lock = NSLock()
        private var v: T
        init(_ v: T) { self.v = v }
        var value: T {
            get { lock.lock(); defer { lock.unlock() }; return v }
            set { lock.lock(); v = newValue; lock.unlock() }
        }
    }

    final class Delegate: NSObject, SPUUpdaterDelegate, SPUStandardUserDriverDelegate {
        let busy = Locked(false)

        /// Installs a downloaded update now, rather than at a quit that
        /// does not come, unless the app is busy or somebody is using it.
        func updater(_ updater: SPUUpdater, willInstallUpdateOnQuit item: SUAppcastItem,
                     immediateInstallationBlock immediateInstallHandler: @escaping () -> Void) -> Bool {
            guard !busy.value, !Self.inUse() else { return false }
            // Asynchronously, so that Sparkle is not entered again from
            // inside its own callback. This installs and relaunches with no
            // window at all.
            DispatchQueue.main.async { immediateInstallHandler() }
            return true
        }

        static func inUse() -> Bool {
            Thread.isMainThread ? NSApp.isActive : DispatchQueue.main.sync { NSApp.isActive }
        }

        var supportsGentleScheduledUpdateReminders: Bool { true }

        func standardUserDriverShouldHandleShowingScheduledUpdate(
            _ update: SUAppcastItem, andInImmediateFocus immediateFocus: Bool) -> Bool {
            true
        }

        /// A check that somebody asked for brings the app forward already.
        /// A scheduled one does not.
        func standardUserDriverWillHandleShowingUpdate(
            _ handleShowingUpdate: Bool, forUpdate update: SUAppcastItem, state: SPUUserUpdateState) {
            guard handleShowingUpdate, !state.userInitiated else { return }
            VeroUpdater.activate()
        }
    }
}
