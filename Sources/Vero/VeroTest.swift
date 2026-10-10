import Foundation

/// The steps vero's tests play against an installed app while its window is
/// open, as vero.py's `play` does: what vero-app.toml's `[[test.step]]`s
/// say, from the JSON file VERO_TEST names. Each step calls a request,
/// sends one without a name, waits until the state shows something,
/// pauses, or copies a file; {tmp} is a new empty folder and {files} the
/// folder of the test's files, VERO_TEST_FILES. How it went is written to
/// VERO_TEST_RESULT, and a line for each step to standard error.
///
/// The state is read with ``Vero/latest(_:)``, not the event stream, which
/// would take events from the app's own window.
enum VeroTest {
    static func startIfAsked(_ vero: Vero) {
        let env = ProcessInfo.processInfo.environment
        guard let file = env["VERO_TEST"], !file.isEmpty else { return }
        Thread.detachNewThread { _ = play(vero, stepsFile: file, environment: env) }
    }

    struct Failure: Error, CustomStringConvertible { let description: String }

    @discardableResult
    static func play(_ vero: Vero, stepsFile: String, environment env: [String: String]) -> Bool {
        let started = Date()
        let tmp = FileManager.default.temporaryDirectory
            .appendingPathComponent("vero-test-" + UUID().uuidString).path
        try? FileManager.default.createDirectory(atPath: tmp, withIntermediateDirectories: true)
        let folders = ["tmp": tmp, "files": env["VERO_TEST_FILES"] ?? ""]
        var results: [[String: Any]] = []
        var ok = true
        do {
            let data = try Data(contentsOf: URL(fileURLWithPath: stepsFile))
            guard case .object(let top) = try JSONDecoder().decode(JSONValue.self, from: data),
                  case .array(let steps) = top["steps"] ?? .array([]) else {
                throw Failure(description: "\(stepsFile) has no steps")
            }
            // The worker up, with a state to show.
            let deadline = Date().addingTimeInterval(60)
            while vero.state != .running || vero.latest(JSONValue.self) == nil {
                if Date() > deadline { throw Failure(description: "the worker wasn't running after 60s (\(vero.state))") }
                Thread.sleep(forTimeInterval: 0.05)
            }
            for (i, raw) in steps.enumerated() {
                guard case .object(let step) = fill(raw, folders) else { continue }
                let began = Date()
                let what = describe(step)
                do {
                    try run(vero, step, env)
                    let secs = Date().timeIntervalSince(began)
                    results.append(["step": what, "seconds": round(secs * 100) / 100])
                    log("step \(i + 1)/\(steps.count), \(what): ok (\(String(format: "%.1f", secs))s)")
                } catch {
                    ok = false
                    let secs = Date().timeIntervalSince(began)
                    results.append(["step": what, "seconds": round(secs * 100) / 100, "error": "\(error)"])
                    log("step \(i + 1)/\(steps.count), \(what): FAILED: \(error)")
                    break
                }
            }
        } catch {
            ok = false
            results.append(["step": "starting", "error": "\(error)"])
            log("\(error)")
        }
        if let out = env["VERO_TEST_RESULT"], !out.isEmpty {
            let result: [String: Any] = ["ok": ok, "seconds": round(Date().timeIntervalSince(started) * 100) / 100, "steps": results]
            if let data = try? JSONSerialization.data(withJSONObject: result, options: [.prettyPrinted]) {
                try? data.write(to: URL(fileURLWithPath: out + ".part"))
                _ = try? FileManager.default.replaceItemAt(URL(fileURLWithPath: out), withItemAt: URL(fileURLWithPath: out + ".part"))
                if !FileManager.default.fileExists(atPath: out) {
                    try? FileManager.default.moveItem(atPath: out + ".part", toPath: out)
                }
            }
        }
        return ok
    }

    private static func log(_ line: String) {
        FileHandle.standardError.write(("vero test: " + line + "\n").data(using: .utf8)!)
    }

    /// value with {tmp} and {files} replaced, in every string in it.
    static func fill(_ value: JSONValue, _ folders: [String: String]) -> JSONValue {
        switch value {
        case .string(var s):
            for (name, folder) in folders { s = s.replacingOccurrences(of: "{\(name)}", with: folder) }
            return .string(s)
        case .array(let a): return .array(a.map { fill($0, folders) })
        case .object(let o): return .object(o.mapValues { fill($0, folders) })
        default: return value
        }
    }

    static func describe(_ step: [String: JSONValue]) -> String {
        if case .string(let name)? = step["call"] { return "call " + name }
        if let body = step["send"] { return "send " + text(body) }
        if case .object(let w)? = step["wait"] {
            let op = ["is", "not", "at_least", "contains"].first { w[$0] != nil } ?? "is"
            return "wait until \(text(w["path"] ?? .null, quoted: false)) \(op.replacingOccurrences(of: "_", with: " ")) \(text(w[op] ?? .null))"
        }
        if let p = step["pause"] { return "pause \(text(p))s" }
        if case .object(let c)? = step["copy"] {
            return "copy \(text(c["from"] ?? .null, quoted: false)) to \(text(c["to"] ?? .null, quoted: false))"
        }
        return "unknown step"
    }

    private static func text(_ v: JSONValue, quoted: Bool = true) -> String {
        if !quoted, case .string(let s) = v { return s }
        if case .number(let n) = v, n == n.rounded() { return String(Int(n)) }
        guard let data = try? JSONEncoder().encode(v) else { return "?" }
        return String(data: data, encoding: .utf8) ?? "?"
    }

    private static func run(_ vero: Vero, _ step: [String: JSONValue], _ env: [String: String]) throws {
        if case .string(let name)? = step["call"] {
            try wait { try await vero.call(name, step["with"] ?? .object([:])) }
        } else if let body = step["send"] {
            try wait { try await vero.send(body) }
        } else if case .number(let secs)? = step["pause"] {
            Thread.sleep(forTimeInterval: secs)
        } else if case .object(let c)? = step["copy"], case .string(var from)? = c["from"], case .string(var to)? = c["to"] {
            if !from.hasPrefix("/") { from = (env["VERO_TEST_FILES"] ?? "") + "/" + from }
            let fm = FileManager.default
            if to.hasSuffix("/") {
                try fm.createDirectory(atPath: to, withIntermediateDirectories: true)
                to += (from as NSString).lastPathComponent
            } else {
                try fm.createDirectory(atPath: (to as NSString).deletingLastPathComponent, withIntermediateDirectories: true)
            }
            if fm.fileExists(atPath: to) { try fm.removeItem(atPath: to) }
            try fm.copyItem(atPath: from, toPath: to)
        } else if case .object(let w)? = step["wait"] {
            var timeout = 60.0
            if case .number(let t)? = step["timeout"] { timeout = t }
            let deadline = Date().addingTimeInterval(timeout)
            var state = vero.latest(JSONValue.self) ?? .null
            while !holds(state, w) {
                if Date() > deadline {
                    let path: String
                    if case .string(let p)? = w["path"] { path = p } else { path = "" }
                    throw Failure(description: "after \(text(.number(timeout)))s, \(path) is \(text(at(state, path)))")
                }
                Thread.sleep(forTimeInterval: 0.25)
                state = vero.latest(JSONValue.self) ?? state
            }
        } else {
            throw Failure(description: "a step is call, send, wait, pause or copy")
        }
    }

    /// An async request, waited for on this thread, which isn't the app's.
    private static func wait(_ request: @escaping () async throws -> Data) throws {
        let done = DispatchSemaphore(value: 0)
        var failure: Error?
        Task.detached {
            do { _ = try await request() } catch { failure = error }
            done.signal()
        }
        done.wait()
        if let failure { throw failure }
    }

    /// What path names in state: keys and list indexes, "#" for a length.
    static func at(_ state: JSONValue, _ path: String) -> JSONValue {
        var v = state
        for part in path.split(separator: ".").map(String.init) {
            switch v {
            case .array(let a) where part == "#": v = .number(Double(a.count))
            case .object(let o) where part == "#": v = .number(Double(o.count))
            case .string(let s) where part == "#": v = .number(Double(s.count))
            case .array(let a):
                guard let i = Int(part) else { return .null }
                let j = i < 0 ? a.count + i : i
                guard a.indices.contains(j) else { return .null }
                v = a[j]
            case .object(let o):
                guard let next = o[part] else { return .null }
                v = next
            default: return .null
            }
        }
        return v
    }

    static func holds(_ state: JSONValue, _ wait: [String: JSONValue]) -> Bool {
        guard case .string(let path)? = wait["path"] else { return false }
        let got = at(state, path)
        if let want = wait["is"] { return got == want }
        if let want = wait["not"] { return got != want }
        if case .number(let least)? = wait["at_least"] {
            if case .number(let n) = got { return n >= least }
            return false
        }
        if let want = wait["contains"] {
            switch (got, want) {
            case (.string(let s), .string(let w)): return s.contains(w)
            case (.array(let a), _): return a.contains(want)
            case (.object(let o), .string(let k)): return o[k] != nil
            default: return false
            }
        }
        return false
    }
}
