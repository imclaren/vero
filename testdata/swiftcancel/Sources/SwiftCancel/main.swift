import Foundation
import Vero

// Cancels a slow request after 0.2 seconds, and checks that the call ends at
// once with CancellationError and that the worker's handler stopped.
let vero = try Vero(workerPath: CommandLine.arguments[1])
try await Task.sleep(nanoseconds: 1_000_000_000)
struct Empty: Encodable {}
let start = Date()
let task = Task { try await vero.call("slow", Empty()) }
try await Task.sleep(nanoseconds: 200_000_000)
task.cancel()
do {
    _ = try await task.value
    print("FAIL: the slow call finished")
    exit(1)
} catch is CancellationError {
    print("cancelled after \(String(format: "%.2f", Date().timeIntervalSince(start)))s")
}
for _ in 0..<50 {
    let n = String(data: try await vero.call("stopped", Empty()), encoding: .utf8) ?? ""
    if n == "1" { print("PASS: the handler stopped"); exit(0) }
    try await Task.sleep(nanoseconds: 50_000_000)
}
print("FAIL: the handler never stopped")
exit(1)
