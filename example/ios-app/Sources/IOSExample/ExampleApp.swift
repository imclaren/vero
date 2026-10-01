import SwiftUI
import Vero

// What the worker pushes: the same shape as Status and Job in
// ../../archive/worker.go.
struct Job: Decodable, Identifiable {
    let id: Int
    let name: String
    let phase: String
    let progress: Int
}

struct Status: Decodable {
    let jobs: [Job]
    let working: Bool
    let since: String
}

// One of these per handler on the worker. The name routes the request -
// "restartJob" is the vero.UpdateWith in worker.go - and Reply says what
// comes back, so nothing has to guess at it.
struct RestartJob: NamedRequest {
    static let name = "restartJob"
    typealias Reply = Status
    let id: Int
}

@main
struct ExampleApp: App {
    // The one line that differs from the macOS example, which names a worker
    // to copy out of the bundle and start. An iOS application may not start a
    // program, so this one has no worker to name: the archive it links
    // carries the worker, and vero runs it on a goroutine.
    @StateObject private var vero = VeroModel<Status>(compiledInWorker: [])

    var body: some Scene {
        WindowGroup {
            NavigationStack {
                List {
                    if let problem = vero.problem {
                        Text(problem).foregroundStyle(.red)
                    }
                    ForEach(vero.state?.jobs ?? []) { job in
                        VStack(alignment: .leading, spacing: 6) {
                            HStack {
                                Text(job.name)
                                Spacer()
                                Text(job.phase).foregroundStyle(.secondary)
                                // The press. This goes to the "restartJob"
                                // handler in worker.go. There is no reply to
                                // deal with here, because the worker pushes
                                // the new state and that is what redraws.
                                Button("Restart") { vero.call(RestartJob(id: job.id)) }
                                    .buttonStyle(.bordered)
                            }
                            ProgressView(value: Double(job.progress) / 100)
                        }
                        .padding(.vertical, 4)
                    }
                }
                .navigationTitle("vero")
                .toolbar {
                    // Pushed as well, so this turns while the worker is busy
                    // without anything here asking it whether it is.
                    Image(systemName: vero.state?.working == true
                          ? "arrow.triangle.2.circlepath"
                          : "checkmark.circle")
                }
            }
        }
    }
}
