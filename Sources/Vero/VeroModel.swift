import Combine
import Foundation

/// A worker, its latest state, and everything a SwiftUI view needs to draw
/// both - without an application writing any of it.
///
/// ``VeroClient`` cannot do this itself, for two reasons that are both about
/// types rather than design. It is not generic, so it has nowhere to keep a
/// state your worker defines; and its initialiser throws, which `@StateObject`
/// cannot express, so every application ended up wrapping it in a class of its
/// own to catch that. This is that class, written once.
///
///     struct ContentView: View {
///         @StateObject private var vero = VeroModel<Status>(
///             bundledWorker: "worker", directoryName: "Example/bin")
///
///         var body: some View {
///             List(vero.state?.jobs ?? []) { job in
///                 Button("Restart") { vero.call(RestartJob(id: job.id)) }
///                     .disabled(vero.isBusy)
///             }
///         }
///     }
///
/// Drop to ``worker`` for anything this does not cover - a second event type,
/// a request whose reply you need to await - the way ``VeroClient`` exposes
/// ``VeroClient/link`` for the same reason. Nothing here is a wall.
@MainActor
public final class VeroModel<State: Decodable>: ObservableObject {

    /// The most recent state the worker pushed, or nil before the first one.
    ///
    /// Optional deliberately: a view that opens before the worker has said
    /// anything has nothing to draw, and pretending otherwise with an empty
    /// value makes "starting up" and "genuinely empty" look identical.
    @Published public private(set) var state: State?

    /// What went wrong, in a sentence fit to show someone.
    ///
    /// Set when the worker could not be started at all, and when a request
    /// came back refused. Cleared by the next state the worker pushes, since
    /// a worker that is talking again is a worker that is working.
    @Published public private(set) var problem: String?

    /// The worker underneath, or nil if it could not be started.
    public private(set) var worker: VeroClient?

    private var forwarding: AnyCancellable?

#if !os(iOS) && !os(tvOS) && !os(watchOS) && !os(visionOS)
    /// Prepares the worker shipped in the application bundle, launches it, and
    /// subscribes to what it pushes.
    ///
    /// Does not throw. A worker that cannot start leaves ``worker`` nil and
    /// puts the reason in ``problem``, which is what a view wants anyway -
    /// there is nothing useful for `@StateObject` to do with an error.
    public init(
        bundledWorker name: String,
        directoryName: String,
        supersededNames: [String] = [],
        arguments: [String] = []
    ) {
        do {
            let worker = try VeroClient(
                bundledWorker: name,
                directoryName: directoryName,
                supersededNames: supersededNames,
                arguments: arguments)
            adopt(worker)
        } catch {
            problem = error.localizedDescription
        }
    }
#endif

    /// As above, for a worker that is already somewhere on disk.
    public init(workerPath: String, arguments: [String] = []) {
        do {
            adopt(try VeroClient(workerPath: workerPath, arguments: arguments))
        } catch {
            problem = error.localizedDescription
        }
    }

    /// A worker compiled into this application - see
    /// ``Vero/init(compiledInWorker:)``, which iOS needs.
    public init(compiledInWorker arguments: [String] = []) {
        do {
            adopt(try VeroClient(compiledInWorker: arguments))
        } catch {
            problem = error.localizedDescription
        }
    }

    private func adopt(_ worker: VeroClient) {
        self.worker = worker

        // A nested ObservableObject does not republish its parent, so a view
        // reading isBusy or state through this object would never redraw when
        // they moved. Every application that wrote this by hand had to notice
        // that and forward it; most did not.
        forwarding = worker.objectWillChange.sink { [weak self] _ in
            self?.objectWillChange.send()
        }

        worker.onEvent(State.self) { [weak self] value in
            self?.apply(value)
        }
        if let first = worker.latest(State.self) { apply(first) }

        // A worker whose file is the problem restarts for ever, and the
        // supervisor cannot tell that from a worker that is merely unlucky.
        worker.restoreBundledWorkerAfterRepeatedRestarts(3) { [weak self] in
            self?.problem = "The worker keeps stopping. Try reinstalling."
        }
    }

    private func apply(_ value: State) {
        state = value
        problem = nil
    }

    // MARK: - Asking the worker to do something

    /// Sends a request from somewhere that cannot await - a button.
    ///
    /// A reply that is an error becomes ``problem``; one that arrives normally
    /// needs nothing done with it, because the worker pushes the new state.
    public func call<R: NamedRequest>(
        _ request: R,
        showsBusy: Bool = true,
        then next: (@MainActor () -> Void)? = nil
    ) {
        guard let worker else {
            problem = VeroError.notRunning.localizedDescription
            return
        }
        Task { [weak self] in
            do {
                _ = try await worker.call(request, showsBusy: showsBusy)
                next?()
            } catch VeroError.refused(let message) {
                // The worker received it and said no, which is worth showing.
                // "Not running" is not: it is already visible through state.
                self?.problem = message
            } catch {
                next?()
            }
        }
    }

    /// Sends a request and waits for its reply, for a caller that needs it.
    @discardableResult
    public func call<R: NamedRequest>(
        _ request: R,
        showsBusy: Bool = true
    ) async throws -> R.Reply {
        guard let worker else { throw VeroError.notRunning }
        return try await worker.call(request, showsBusy: showsBusy)
    }

    // MARK: - What a view draws

    /// What the worker is doing: starting, running, restarting, stopped.
    public var workerState: WorkerState { worker?.state ?? .stopped }

    /// True while any request is in flight.
    public var isBusy: Bool { worker?.isBusy ?? false }

    /// True while a request with this tag is in flight.
    public func isBusy(_ tag: String) -> Bool { worker?.isBusy(tag) ?? false }

    /// Stops the worker.
    ///
    /// Not required - its standard input closes when the application exits and
    /// it stops with it, even on a crash - but it ends the work a moment
    /// sooner.
    public func stop() {
        worker?.stop()
    }
}
