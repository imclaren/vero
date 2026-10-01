import Foundation
import CryptoKit

// A worker shipped beside the application, which the platforms left out
// below cannot have: they may not start a program at all, so there is no
// Process to start one with.  They compile the worker in instead - see
// Vero.init(compiledInWorker:).
#if !os(iOS) && !os(tvOS) && !os(watchOS) && !os(visionOS)

// Everything here starts a worker that was shipped beside the application,
// which the platforms below cannot do: there is no Process to start one with,
// because an application may not start a program at all.  They compile the
// worker in instead - see Vero.init(compiledInWorker:).
#if !os(iOS) && !os(tvOS) && !os(watchOS) && !os(visionOS)

/// Where the worker actually runs from.
///
/// An application ships its worker inside its own bundle, which is read-only
/// and signed - so a worker that updates itself cannot live there. It has to
/// be copied somewhere writable first, and then two things are replacing the
/// same file: the application, when it ships a newer one, and the worker
/// itself, when it downloads one.
///
/// Getting that wrong is expensive and quiet, so it is here rather than in
/// every application:
///
/// * **Compare versions, not contents.** A self-updated worker always differs
///   from the bundled one. Copying whenever they differ means the application
///   copies its older worker over the newer one, the worker downloads the
///   newer one again, and that repeats on every launch - tens of megabytes at
///   a time, with nothing reporting it.
/// * **Compare them numerically.** As strings, `0.10.0` sorts below `0.9.9`,
///   which strands clients exactly when releases start accumulating.
/// * **A universal binary is not a thin one.** `lipo` output begins
///   `ca fe ba be`, which reads as `0xbebafeca` on a little-endian Mac. Check
///   only the big-endian spelling and the application rejects the worker it
///   was built with.
///
/// The worker must answer `-version`, which `vero.WorkerOptions` provides.
public struct VeroWorkerBundle {

    /// The worker's filename inside the application bundle.
    public let bundledName: String

    /// Directory under Application Support to run it from, e.g. "calmdocs/bin".
    public let directoryName: String

    /// Names left behind by an earlier scheme, removed on preparation.
    /// Superseded per-architecture copies, typically.
    public var supersededNames: [String]

    /// In a debug build, always take the worker from the bundle.
    ///
    /// Rebuilding a worker without bumping its version leaves the on-disk copy
    /// in place - correctly, since it is the same version - so the change
    /// under test never runs and nothing says why. Automatic rather than a
    /// flag somebody has to remember.
    public var useBundleInDebugBuilds: Bool

    public init(
        bundledName: String,
        directoryName: String,
        supersededNames: [String] = [],
        useBundleInDebugBuilds: Bool = true
    ) {
        self.bundledName = bundledName
        self.directoryName = directoryName
        self.supersededNames = supersededNames
        self.useBundleInDebugBuilds = useBundleInDebugBuilds
    }

    // MARK: - Preparing

    /// Returns a worker ready to launch, copying it out of the bundle if the
    /// copy on disk is missing, unusable, or older.
    @discardableResult
    public func prepare() throws -> URL {
        let bundled = try bundledURL()
        let writable = try writableURL()

        try removeSuperseded(besides: writable)

        let replace = shouldUseBundle
            || shouldReplace(bundled: bundled, writable: writable)

        if replace {
            try copy(from: bundled, to: writable)
        } else {
            log("using the worker already on disk: \(writable.path)")
        }
        return writable
    }

    /// Replaces the on-disk worker with the bundled one unconditionally.
    ///
    /// For when a worker crashes repeatedly on startup: the supervisor can
    /// restart a worker that died, but not notice that the file itself is the
    /// problem - a bad self-update, or a truncated download.
    @discardableResult
    public func restoreFromBundle() throws -> URL {
        let bundled = try bundledURL()
        let writable = try writableURL()
        try copy(from: bundled, to: writable)
        return writable
    }

    /// The version the bundled worker reports, or nil if it cannot say.
    public func bundledVersion() -> String? {
        guard let url = try? bundledURL() else { return nil }
        return VeroWorkerBundle.version(of: url)
    }

    // MARK: - Locations

    private func bundledURL() throws -> URL {
        if let url = Bundle.main.url(forResource: bundledName, withExtension: nil) {
            return url
        }
        // Not inside an application bundle: a SwiftPM executable, or a
        // command-line build during development. Look beside the executable,
        // which is where a build puts it.
        if let exe = Bundle.main.executableURL?.deletingLastPathComponent() {
            let beside = exe.appendingPathComponent(bundledName)
            if FileManager.default.fileExists(atPath: beside.path) { return beside }
        }
        throw VeroWorkerBundleError.notInBundle(bundledName)
    }

    private func writableURL() throws -> URL {
        guard let support = FileManager.default.urls(
            for: .applicationSupportDirectory, in: .userDomainMask).first
        else {
            throw VeroWorkerBundleError.noApplicationSupport
        }
        let dir = support.appendingPathComponent(directoryName)
        do {
            try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        } catch {
            throw VeroWorkerBundleError.cannotCreateDirectory(dir.path, error)
        }
        return dir.appendingPathComponent(bundledName)
    }

    private func removeSuperseded(besides keep: URL) throws {
        let dir = keep.deletingLastPathComponent()
        for name in supersededNames where name != bundledName {
            let url = dir.appendingPathComponent(name)
            if FileManager.default.fileExists(atPath: url.path) {
                try? FileManager.default.removeItem(at: url)
                log("removed superseded worker: \(name)")
            }
        }
    }

    // MARK: - The decision

    private var shouldUseBundle: Bool {
        #if DEBUG
        if useBundleInDebugBuilds {
            log("debug build: using the worker from the bundle regardless of version")
            return true
        }
        return false
        #else
        return false
        #endif
    }

    private func shouldReplace(bundled: URL, writable: URL) -> Bool {
        if !FileManager.default.fileExists(atPath: writable.path) {
            log("no worker on disk yet")
            return true
        }
        if !VeroWorkerBundle.isUsable(writable) {
            log("the worker on disk is unusable, replacing it")
            return true
        }

        let onDisk = VeroWorkerBundle.version(of: writable)
        let inBundle = VeroWorkerBundle.version(of: bundled)

        switch (onDisk, inBundle) {
        case (nil, nil):
            // Neither can say, so there is no version to compare. Fall back to
            // content equality, which is right while neither can describe
            // itself.
            if let a = sha256(bundled), let b = sha256(writable), a == b { return false }
            log("neither worker reports a version and they differ")
            return true

        case (nil, _):
            // On disk predates -version and the bundled one does not, so the
            // bundle is newer by definition.
            log("the worker on disk predates -version")
            return true

        case (_, nil):
            // The reverse: on disk can answer and the bundled one cannot, so
            // on disk is the newer. Keeping it stops a downgrade dragging a
            // client backwards.
            log("the bundled worker predates -version, keeping the one on disk")
            return false

        case let (disk?, bundle?):
            if VeroWorkerBundle.compare(bundle, disk) == .orderedDescending {
                log("bundled worker \(bundle) is newer than \(disk) on disk")
                return true
            }
            return false
        }
    }

    private func copy(from bundled: URL, to writable: URL) throws {
        if FileManager.default.fileExists(atPath: writable.path) {
            // Why it is being replaced was logged by whatever decided to.
            try? FileManager.default.removeItem(at: writable)
        }
        do {
            try FileManager.default.copyItem(at: bundled, to: writable)
            try FileManager.default.setAttributes(
                [.posixPermissions: 0o755], ofItemAtPath: writable.path)
        } catch {
            throw VeroWorkerBundleError.copyFailed(error)
        }
        guard VeroWorkerBundle.isUsable(writable) else {
            try? FileManager.default.removeItem(at: writable)
            throw VeroWorkerBundleError.copiedWorkerIsUnusable(writable.path)
        }
        log("copied the worker from the bundle to \(writable.path)")
    }

    private func sha256(_ url: URL) -> String? {
        guard let data = try? Data(contentsOf: url) else { return nil }
        return SHA256.hash(data: data).map { String(format: "%02x", $0) }.joined()
    }

    private func log(_ message: String) { print("vero: \(message)") }
}

// MARK: - Checks, usable on their own

extension VeroWorkerBundle {

    /// Whether a file looks like something that can be executed.
    ///
    /// Cheap checks only: this catches a truncated download or an HTML error
    /// page saved over the worker, not a hostile binary. Code signing is what
    /// answers that question.
    public static func isUsable(_ url: URL) -> Bool {
        let path = url.path
        let fm = FileManager.default
        guard fm.fileExists(atPath: path), fm.isReadableFile(atPath: path) else { return false }

        if let size = (try? fm.attributesOfItem(atPath: path))?[.size] as? UInt64, size < 1024 {
            return false
        }
        guard let handle = try? FileHandle(forReadingFrom: url) else { return false }
        defer { try? handle.close() }
        guard let head = try? handle.read(upToCount: 4), head.count == 4 else { return false }

        // Compared against a UInt32 loaded in host byte order, so each constant
        // is how the header actually reads on a little-endian Mac - not the
        // name it has in mach-o/loader.h. A universal binary begins
        // ca fe ba be, which loads as 0xbebafeca: list only the big-endian
        // spelling and lipo output is rejected.
        let magic = head.withUnsafeBytes { $0.load(as: UInt32.self) }
        let valid: Set<UInt32> = [
            0xfeedface, 0xfeedfacf,   // thin, 32 and 64 bit
            0xcefaedfe, 0xcffaedfe,   // thin, byte-swapped
            0xcafebabe, 0xbebafeca,   // universal
            0xcafebabf, 0xbfbafeca,   // universal, 64 bit
        ]
        return valid.contains(magic)
    }

    /// Asks a worker what version it is, or nil if it cannot say.
    ///
    /// A worker older than the `-version` flag exits non-zero with "flag
    /// provided but not defined", which is itself the answer: it predates
    /// versioning, so it is older than anything that can reply.
    public static func version(of url: URL) -> String? {
        let process = Process()
        process.executableURL = url
        process.arguments = ["-version"]
        let out = Pipe()
        process.standardOutput = out
        process.standardError = Pipe()      // discard the usage text

        do { try process.run() } catch { return nil }

        // It exits in milliseconds. Do not wait forever on one that ignores
        // the flag and starts up instead.
        let deadline = Date().addingTimeInterval(5)
        while process.isRunning && Date() < deadline { usleep(20_000) }
        if process.isRunning {
            process.terminate()
            return nil
        }
        guard process.terminationStatus == 0 else { return nil }

        let text = String(data: out.fileHandleForReading.readDataToEndOfFile(), encoding: .utf8)?
            .trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return text.isEmpty ? nil : text
    }

    /// Compares dotted versions numerically, so 0.10.0 is newer than 0.9.9.
    public static func compare(_ a: String, _ b: String) -> ComparisonResult {
        let lhs = a.split(separator: ".").map { Int($0.filter(\.isNumber)) ?? 0 }
        let rhs = b.split(separator: ".").map { Int($0.filter(\.isNumber)) ?? 0 }
        for i in 0..<max(lhs.count, rhs.count) {
            let l = i < lhs.count ? lhs[i] : 0
            let r = i < rhs.count ? rhs[i] : 0
            if l != r { return l < r ? .orderedAscending : .orderedDescending }
        }
        return .orderedSame
    }
}

public enum VeroWorkerBundleError: Error, LocalizedError {
    case notInBundle(String)
    case noApplicationSupport
    case cannotCreateDirectory(String, Error)
    case copyFailed(Error)
    case copiedWorkerIsUnusable(String)

    public var errorDescription: String? {
        switch self {
        case .notInBundle(let name):
            return "no worker named \(name) in the application bundle"
        case .noApplicationSupport:
            return "cannot find Application Support"
        case .cannotCreateDirectory(let path, let error):
            return "cannot create \(path): \(error.localizedDescription)"
        case .copyFailed(let error):
            return "cannot copy the worker out of the bundle: \(error.localizedDescription)"
        case .copiedWorkerIsUnusable(let path):
            return "the worker copied to \(path) does not look executable"
        }
    }
}

#endif

#endif
