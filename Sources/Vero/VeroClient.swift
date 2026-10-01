import Foundation

#if canImport(Combine)
import Combine
#endif

/// A worker, and the frontend state that goes with driving one.
///
/// ``Vero`` moves messages. This adds what every frontend built on it needs
/// anyway: whether a request is in flight, whether the worker is up, and
/// somewhere for events to arrive already on the main actor.
///
///     @StateObject private var worker = try! VeroClient(workerPath: path)
///
///     Button("Add group") {
///         worker.send(Request(type: "addGroup", id: name)) {
///             worker.send(Request(type: "getGroups"))   // after the first is answered
///         }
///     }
///     .disabled(worker.isBusy)
///
/// Everything it publishes is written on the main actor, so views can read it
/// directly.
///
/// The raw ``Vero`` underneath stays available through ``link`` for anything
/// that wants a different shape - progress per row, say, rather than one flag
/// for the window.
@MainActor
public final class VeroClient: ObservableObject {

    /// What the worker is doing. Worth showing: "restarting" and stale
    /// progress look identical otherwise.
    @Published public private(set) var state: WorkerState = .starting

    /// How many times the worker has been relaunched without one of them
    /// lasting. A worker that runs for a while and then exits - to pick up a
    /// change, or to run a version it has just downloaded - puts this back to
    /// zero, so a climbing count means the binary itself is the problem and
    /// not that the worker has a reason to restart.
    @Published public private(set) var restarts: Int = 0

    /// How many requests the frontend is waiting on.
    ///
    /// A count rather than a flag, because a flag invites dropping a request
    /// that arrives while another is running - which loses the second half of
    /// every "do this, then refresh".
    ///
    /// Requests sent with `showsBusy: false` are excluded. A long poll is
    /// outstanding almost all the time by design, so counting it would leave
    /// every button disabled, or flickering as each poll returns and the next
    /// begins.
    @Published public private(set) var inFlight: Int = 0

    /// In-flight requests by tag, for frontends that want to disable one
    /// control rather than all of them.
    @Published public private(set) var inFlightByTag: [String: Int] = [:]

    /// The last error that was not simply "the worker is restarting", or nil.
    @Published public private(set) var lastError: String?

    /// True while any request is in flight.
    public var isBusy: Bool { inFlight > 0 }

    /// True while a request with this tag is in flight.
    public func isBusy(_ tag: String) -> Bool { (inFlightByTag[tag] ?? 0) > 0 }

    /// The underlying channel, for anything this class does not cover.
    public let link: Vero

    private var watchers: [Task<Void, Never>] = []

    /// Launches the worker and starts supervising it.
    public init(workerPath: String, arguments: [String] = []) throws {
        self.link = try Vero(workerPath: workerPath, arguments: arguments)
        watchStatus()
    }

    /// A worker compiled into this application - see
    /// ``Vero/init(compiledInWorker:)``, which iOS needs.
    public init(compiledInWorker arguments: [String] = []) throws {
        self.link = try Vero(compiledInWorker: arguments)
        watchStatus()
    }

#if !os(iOS) && !os(tvOS) && !os(watchOS) && !os(visionOS)
    /// Prepares the worker shipped in the application bundle, then launches it.
    ///
    /// A bundle is read-only and signed, so a worker that updates itself
    /// cannot run from there. ``VeroWorkerBundle`` copies it somewhere
    /// writable and decides, on every launch, whether the copy already there
    /// is newer than the one just shipped.
    ///
    ///     let worker = try VeroClient(
    ///         bundledWorker: "myapp-worker",
    ///         directoryName: "MyApp/bin")
    public convenience init(
        bundledWorker name: String,
        directoryName: String,
        supersededNames: [String] = [],
        arguments: [String] = []
    ) throws {
        let bundle = VeroWorkerBundle(
            bundledName: name,
            directoryName: directoryName,
            supersededNames: supersededNames)
        let url = try bundle.prepare()
        try self.init(workerPath: url.path, arguments: arguments)
        self.bundle = bundle
    }

    /// The bundle this worker came from, if it was launched from one.
    ///
    /// Use it to put the shipped worker back when one keeps crashing on
    /// startup: the supervisor restarts a worker that died, but cannot tell
    /// that the file itself is the problem.
    public private(set) var bundle: VeroWorkerBundle?
#endif

    deinit {
        for w in watchers { w.cancel() }
    }

    /// Stops the worker.
    ///
    /// Not required: its standard input closes when this process exits and it
    /// stops with it, crash and force quit included.
    public func stop() {
        for w in watchers { w.cancel() }
        watchers.removeAll()
        link.stop()
    }

    // MARK: - Requests

    /// Sends a request and waits for the reply, tracking it as in flight.
    ///
    /// A refusal is thrown as ``VeroError/refused(_:)``. A request made while
    /// the worker is restarting waits for it rather than failing: that is the
    /// one error worth retrying, and the only one this retries.
    public func send<T: Encodable, R: Decodable>(
        _ request: T,
        returning: R.Type,
        tag: String? = nil,
        showsBusy: Bool = true,
        waitingForWorker timeout: TimeInterval = 10
    ) async throws -> R {
        begin(tag, showsBusy)
        defer { end(tag, showsBusy) }
        do {
            let reply: R = try await withWorker(timeout: timeout) {
                try await self.link.send(request, returning: R.self)
            }
            lastError = nil
            return reply
        } catch {
            record(error)
            throw error
        }
    }

    /// Sends a request, ignoring the reply's contents.
    @discardableResult
    public func send<T: Encodable>(
        _ request: T,
        tag: String? = nil,
        showsBusy: Bool = true,
        waitingForWorker timeout: TimeInterval = 10
    ) async throws -> Data {
        begin(tag, showsBusy)
        defer { end(tag, showsBusy) }
        do {
            let data = try await withWorker(timeout: timeout) {
                try await self.link.send(request)
            }
            lastError = nil
            return data
        } catch {
            record(error)
            throw error
        }
    }

    /// Sends a request from somewhere that cannot await - a SwiftUI button.
    ///
    /// `then` runs once the reply has arrived, which is what "do this, then
    /// refresh" needs: asking for the list at the same moment as changing it
    /// can return the list from before the change.
    public func send<T: Encodable>(
        _ request: T,
        tag: String? = nil,
        showsBusy: Bool = true,
        then next: (@MainActor () -> Void)? = nil
    ) {
        Task { [weak self] in
            guard let self else { return }
            _ = try? await self.send(request, tag: tag, showsBusy: showsBusy)
            next?()
        }
    }

    /// Puts the bundled worker back if it cannot stay up.
    ///
    /// The supervisor restarts a worker that died, but cannot tell that the
    /// binary on disk is the problem - a bad self-update, a truncated
    /// download, a file that will not execute. This can: a worker that never
    /// manages a full run is one whose file is suspect, so the copy that
    /// shipped with the application goes back.
    ///
    /// It waits for ``restarts`` to reach `limit`, rather than acting on each
    /// one. Workers exit deliberately - a changed configuration, a version
    /// just downloaded - and restoring the shipped copy on one of those would
    /// undo a self-update every time, which the worker would then redo,
    /// which exits again. ``restarts`` counts only restarts with no healthy
    /// run between them, so reaching `limit` means the worker really is
    /// failing to start.
    ///
    /// Only useful when the worker came from ``init(bundledWorker:directoryName:supersededNames:arguments:)``;
    /// there is nothing to restore otherwise.
    ///
    /// - Parameters:
    ///   - limit: consecutive failed starts before the shipped worker is
    ///     restored, and again before giving up on it.
    ///   - giveUp: called if the shipped copy cannot stay up either, so the
    ///     frontend can say something rather than looping in silence.
    public func restoreBundledWorkerAfterRepeatedRestarts(
        _ limit: Int = 3,
        giveUp: (@MainActor () -> Void)? = nil
    ) {
#if os(iOS) || os(tvOS) || os(watchOS) || os(visionOS)
        // A compiled-in worker has no shipped copy to restore, and cannot
        // restart in the first place: it dies with the application.
        return
#else
        guard let bundle else { return }
        watchers.append(Task { [weak self] in
            // The count when the shipped worker was put back, or zero while it
            // has not been.
            var restoredAt = 0

            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: 2_000_000_000)
                guard let self else { return }

                let restarts = await MainActor.run { self.restarts }

                // The supervisor clears the count once a worker lasts, so a
                // drop means the one running now is working.
                if restarts < restoredAt { restoredAt = 0 }

                guard restarts >= limit else { continue }

                if restoredAt == 0 {
                    if (try? bundle.restoreFromBundle()) != nil {
                        print("vero: restored the bundled worker after \(restarts) restarts")
                    }
                    restoredAt = restarts
                } else if restarts >= restoredAt + limit {
                    await MainActor.run { giveUp?() }
                    return
                }
            }
        })
#endif
    }

    /// Sends a typed request, tracked and retried like any other.
    ///
    /// The reply type comes from the request, so nothing has to guess what
    /// came back.
    public func call<R: NamedRequest>(
        _ request: R,
        showsBusy: Bool = true,
        waitingForWorker timeout: TimeInterval = 10
    ) async throws -> R.Reply {
        begin(R.name, showsBusy)
        defer { end(R.name, showsBusy) }
        do {
            let reply = try await withWorker(timeout: timeout) {
                try await self.link.call(request)
            }
            lastError = nil
            return reply
        } catch {
            record(error)
            throw error
        }
    }

    /// Sends a typed request from somewhere that cannot await - a button.
    public func call<R: NamedRequest>(
        _ request: R,
        showsBusy: Bool = true,
        then next: (@MainActor () -> Void)? = nil
    ) {
        Task { [weak self] in
            guard let self else { return }
            _ = try? await self.call(request, showsBusy: showsBusy)
            next?()
        }
    }

    // MARK: - Events

    /// Calls `handler` for every event the worker pushes, on the main actor.
    ///
    /// Events arrive when the worker's state moves, so nothing polls and
    /// nothing is sent while it is quiet.
    public func onEvent<T: Decodable>(
        _ type: T.Type,
        _ handler: @escaping @MainActor (T) -> Void
    ) {
        let stream = link.events(T.self)
        watchers.append(Task { [weak self] in
            for await value in stream {
                if Task.isCancelled { return }
                await MainActor.run { handler(value) }
            }
            _ = self
        })
    }

    /// The most recent event, for drawing a window that has just opened
    /// without waiting for the next change.
    public func latest<T: Decodable>(_ type: T.Type) -> T? { link.latest(type) }

    // MARK: - Internals

    /// Retries only while the worker is not running.
    ///
    /// A refusal means the worker received the request and said no, so sending
    /// it again just asks again. Retrying every error - which is the obvious
    /// thing to write, and what applications do write - turns one refusal into
    /// several.
    private func withWorker<R>(
        timeout: TimeInterval,
        _ body: () async throws -> R
    ) async throws -> R {
        let deadline = Date().addingTimeInterval(timeout)
        var wait: UInt64 = 50_000_000   // 50ms, doubling
        while true {
            do {
                return try await body()
            } catch VeroError.notRunning {
                if Date() >= deadline { throw VeroError.notRunning }
                try? await Task.sleep(nanoseconds: wait)
                wait = min(wait * 2, 1_000_000_000)
            }
        }
    }

    private func begin(_ tag: String?, _ showsBusy: Bool) {
        guard showsBusy else { return }
        inFlight += 1
        if let tag { inFlightByTag[tag, default: 0] += 1 }
    }

    private func end(_ tag: String?, _ showsBusy: Bool) {
        guard showsBusy else { return }
        inFlight = max(0, inFlight - 1)
        if let tag {
            let n = (inFlightByTag[tag] ?? 1) - 1
            if n <= 0 { inFlightByTag.removeValue(forKey: tag) } else { inFlightByTag[tag] = n }
        }
    }

    private func record(_ error: Error) {
        // "The worker is restarting" is not something to put in front of
        // somebody: it resolves itself, and state already says so.
        if case VeroError.notRunning = error { return }
        lastError = error.localizedDescription
    }

    /// Keeps ``state`` and ``restarts`` current.
    ///
    /// A poll, because the supervisor's state lives on the other side of a C
    /// boundary with no way to push across it. It is a mutex and a small JSON
    /// encode once a second, and it is here rather than in every application
    /// that would otherwise write the same loop.
    private func watchStatus() {
        watchers.append(Task { [weak self] in
            while !Task.isCancelled {
                guard let self else { return }
                let s = self.link.state
                let r = self.link.restarts
                await MainActor.run {
                    if self.state != s { self.state = s }
                    if self.restarts != r { self.restarts = r }
                }
                try? await Task.sleep(nanoseconds: 1_000_000_000)
            }
        })
    }
}
