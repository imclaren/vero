import Foundation

#if canImport(CVero)
import CVero
#endif

/// What went wrong, and whether the worker is there at all.
public enum VeroError: Error, LocalizedError {
    /// The worker received the request and refused it. It is still running,
    /// so this is a problem with the request, not the connection.
    case refused(String)
    /// The worker is not running: it is starting, restarting after a crash,
    /// or has been stopped. Wait rather than treating it as a failure.
    case notRunning
    /// Another copy of this application is already running a worker. Offer
    /// to switch to it: retrying will not help, and nothing is broken.
    case alreadyRunning(String)
    /// The reply could not be decoded into the type you asked for.
    case badReply(String)
    /// Something else went wrong: the request could not be encoded, or the
    /// worker could not be launched.
    case failed(String)

    public var errorDescription: String? {
        switch self {
        case .refused(let message): return message
        case .notRunning: return "the worker is not running"
        case .alreadyRunning(let message): return message
        case .badReply(let message): return "could not read the reply: \(message)"
        case .failed(let message): return message
        }
    }
}

/// What the worker is doing.
public enum WorkerState: String {
    case starting, running, restarting, stopped, unknown

    init(_ raw: String) { self = WorkerState(rawValue: raw) ?? .unknown }
}

/// Drives a Go worker from a native frontend.
///
/// Create one when the application launches - from the app delegate, not from
/// a view appearing, or nothing starts until someone opens a window - then
/// read ``events`` to keep the frontend current and call ``send(_:)`` to ask
/// the worker to do something.
public final class Vero {
    /// Requests run here, never on a cooperative thread, so a long request
    /// cannot stall Swift concurrency.
    ///
    /// Concurrent, and it has to be. A request blocks until its reply arrives,
    /// and a worker may legitimately hold one open for minutes - that is what
    /// long polling is. On a serial queue the first such request stops every
    /// other one from even starting, so the frontend appears to ignore every
    /// button until the poll happens to return. The worker answers requests
    /// concurrently and matches replies by id, so there is nothing to
    /// serialise here.
    private static let queue = DispatchQueue(
        label: "vero.requests", qos: .userInitiated, attributes: .concurrent)
    private static let eventQueue = DispatchQueue(label: "vero.events", qos: .utility)

    /// Starts the worker and begins supervising it.
    ///
    /// Returns as soon as the launch is under way. Until the worker is up,
    /// requests throw ``VeroError/notRunning``.

    /// A worker compiled into this application, rather than one started
    /// beside it.
    ///
    /// iOS is why this exists: an application there may not start a program,
    /// so there is nothing to spawn. The archive carries the worker instead -
    /// Go's `vero.ServeInProcess` registers it - and vero runs it on a
    /// goroutine joined to this process by a pipe in memory. Everything above
    /// that pipe is unchanged: the same requests, replies and events.
    ///
    /// Restarting goes away with the process boundary. A worker on a
    /// goroutine dies when the application does, and there is nothing left to
    /// restart it into.
    public convenience init(compiledInWorker arguments: [String] = []) throws {
        try self.init(workerPath: "", arguments: arguments)
    }

    public init(workerPath: String, arguments: [String] = []) throws {
        let argsJSON = arguments.isEmpty
            ? ""
            : String(data: try JSONEncoder().encode(arguments), encoding: .utf8) ?? ""
        let path = strdup(workerPath)
        let args = strdup(argsJSON)
        defer { free(path); free(args) }
        _ = try Vero.unwrap(VeroStart(path, args))
    }

    /// Stops the worker.
    ///
    /// Not required: the worker's standard input closes when this process
    /// exits, and it stops with it, even on a crash or a force quit.
    public func stop() { VeroStop() }

    /// What the worker is doing now.
    public var state: WorkerState {
        guard let data = try? Vero.unwrap(VeroState()),
              let raw = try? JSONDecoder().decode(String.self, from: data)
        else { return .unknown }
        return WorkerState(raw)
    }

    /// How many times the worker has been relaunched after dying.
    ///
    /// Distinguishes a single crash it recovered from, which is worth a log
    /// line, from a worker crashing repeatedly, which usually means the binary
    /// on disk is the problem rather than anything the supervisor can fix.
    public var restarts: Int {
        guard let data = try? Vero.unwrap(VeroRestarts()),
              let n = try? JSONDecoder().decode(Int.self, from: data)
        else { return 0 }
        return n
    }

    /// The most recent event, without waiting for the next one.
    ///
    /// Use it to draw a window that has just opened; ``events`` keeps it up to
    /// date afterwards.
    public func latest<T: Decodable>(_ type: T.Type) -> T? {
        guard let data = try? Vero.unwrap(VeroLatest()), !data.isEmpty else { return nil }
        return try? JSONDecoder().decode(T.self, from: data)
    }

    /// Every state change the worker reports, as it happens.
    ///
    /// The worker sends one whenever its state moves, so there is no polling,
    /// no interval to choose, and nothing sent while it is quiet.
    ///
    ///     for await status in vero.events(Status.self) {
    ///         self.status = status          // already on the main actor
    ///     }
    public func events<T: Decodable>(_ type: T.Type) -> AsyncStream<T> {
        AsyncStream { continuation in
            let cancelled = Cancelled()
            Vero.eventQueue.async {
                while !cancelled.value {
                    guard let data = try? Vero.unwrap(VeroWaitForEvent()), !data.isEmpty else { continue }
                    if let decoded = try? JSONDecoder().decode(T.self, from: data) {
                        continuation.yield(decoded)
                    }
                }
                continuation.finish()
            }
            continuation.onTermination = { _ in cancelled.value = true }
        }
    }

    /// A request that knows what it will be answered with.
    ///
    /// Declaring the reply type on the request removes the guessing: without
    /// it a caller receives bytes and tries one type, then another, until one
    /// succeeds - and an empty JSON array succeeds for every list type, so a
    /// list that happens to be empty is read as whatever was tried first.
    ///
    ///     struct GetGroups: NamedRequest {
    ///         static let name = "getGroups"
    ///         typealias Reply = [Group]
    ///     }
    ///
    ///     let groups = try await vero.call(GetGroups())
    ///
    /// Sends a typed request to the handler registered under its name.
    public func call<R: NamedRequest>(_ request: R) async throws -> R.Reply {
        let data = try await call(R.name, request)
        if data.isEmpty, let empty = EmptyReply() as? R.Reply { return empty }
        do {
            return try JSONDecoder().decode(R.Reply.self, from: data)
        } catch {
            throw VeroError.badReply("\(error)")
        }
    }

    /// Sends a request by name and returns the reply as raw JSON.
    public func call<T: Encodable>(_ name: String, _ request: T) async throws -> Data {
        let body = try JSONEncoder().encode(request)
        return try await withCheckedThrowingContinuation { continuation in
            Vero.queue.async {
                let n = strdup(name)
                let buf = strdup(String(data: body, encoding: .utf8) ?? "{}")
                defer { free(n); free(buf) }
                do {
                    continuation.resume(returning: try Vero.unwrap(VeroCall(n, buf)))
                } catch {
                    continuation.resume(throwing: error)
                }
            }
        }
    }

    /// Sends a request and returns the reply as raw JSON.
    ///
    /// Empty data means the worker chose not to answer, which is normal for a
    /// request that only causes an action.
    public func send<T: Encodable>(_ request: T) async throws -> Data {
        let body = try JSONEncoder().encode(request)
        return try await withCheckedThrowingContinuation { continuation in
            Vero.queue.async {
                let buf = strdup(String(data: body, encoding: .utf8) ?? "{}")
                defer { free(buf) }
                do {
                    continuation.resume(returning: try Vero.unwrap(VeroRequest(buf)))
                } catch {
                    continuation.resume(throwing: error)
                }
            }
        }
    }

    /// Sends a request and decodes the reply into your own type.
    public func send<T: Encodable, R: Decodable>(_ request: T, returning: R.Type) async throws -> R {
        let data = try await send(request)
        guard !data.isEmpty else { throw VeroError.badReply("the worker answered nothing") }
        do {
            return try JSONDecoder().decode(R.self, from: data)
        } catch {
            throw VeroError.badReply("\(error)")
        }
    }

    // Every C function returns {"p":...} or {"e":"..."} and hands ownership of
    // the string to us.
    private static func unwrap(_ raw: UnsafeMutablePointer<CChar>?) throws -> Data {
        guard let raw else { throw VeroError.notRunning }
        defer { VeroFree(raw) }
        let data = Data(String(cString: raw).utf8)

        struct Envelope: Decodable { let e: String?; let code: String? }
        if let envelope = try? JSONDecoder().decode(Envelope.self, from: data), let message = envelope.e {
            switch envelope.code {
            case "already_running": throw VeroError.alreadyRunning(message)
            case "not_running": throw VeroError.notRunning
            case "refused": throw VeroError.refused(message)
            default: throw VeroError.failed(message)
            }
        }
        struct Payload: Decodable { let p: RawJSON? }
        guard let payload = try? JSONDecoder().decode(Payload.self, from: data), let p = payload.p else {
            return Data()
        }
        return p.data
    }
}

/// A request that declares the name it is routed by and the type it is
/// answered with.
///
/// Named to avoid the C function of the same shape: `VeroRequest` is the
/// symbol in the archive, and a Swift protocol by that name shadows it.
public protocol NamedRequest: Encodable {
    /// What the reply decodes into. Use ``EmptyReply`` for a request that
    /// only causes an action.
    associatedtype Reply: Decodable

    /// The handler this is routed to, matching vero.Handle on the worker.
    static var name: String { get }
}

/// The reply type for a request that answers nothing.
public struct EmptyReply: Decodable {
    public init() {}
    public init(from decoder: Decoder) throws {}
}

/// A flag an AsyncStream's termination handler can set from another thread.
private final class Cancelled: @unchecked Sendable {
    private let lock = NSLock()
    private var flag = false
    var value: Bool {
        get { lock.lock(); defer { lock.unlock() }; return flag }
        set { lock.lock(); flag = newValue; lock.unlock() }
    }
}

/// Keeps a payload as bytes so it can be decoded into the caller's own type
/// rather than being re-encoded first.
private struct RawJSON: Decodable {
    let data: Data
    init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        data = try JSONEncoder().encode(try container.decode(JSONValue.self))
    }
}

private enum JSONValue: Codable {
    case null, bool(Bool), number(Double), string(String)
    case array([JSONValue]), object([String: JSONValue])

    init(from decoder: Decoder) throws {
        let c = try decoder.singleValueContainer()
        if c.decodeNil() { self = .null }
        else if let v = try? c.decode(Bool.self) { self = .bool(v) }
        else if let v = try? c.decode(Double.self) { self = .number(v) }
        else if let v = try? c.decode(String.self) { self = .string(v) }
        else if let v = try? c.decode([JSONValue].self) { self = .array(v) }
        else if let v = try? c.decode([String: JSONValue].self) { self = .object(v) }
        else { throw DecodingError.dataCorruptedError(in: c, debugDescription: "unrecognised JSON") }
    }

    func encode(to encoder: Encoder) throws {
        var c = encoder.singleValueContainer()
        switch self {
        case .null: try c.encodeNil()
        case .bool(let v): try c.encode(v)
        case .number(let v): try c.encode(v)
        case .string(let v): try c.encode(v)
        case .array(let v): try c.encode(v)
        case .object(let v): try c.encode(v)
        }
    }
}
