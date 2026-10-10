// winid PID: the number of the process's main window on screen, for
// screencapture -l, or nothing while it has none. record-mac.sh builds it.
import CoreGraphics

let pid = Int32(CommandLine.arguments[1]) ?? 0
let list = CGWindowListCopyWindowInfo([.optionOnScreenOnly], kCGNullWindowID) as? [[String: Any]] ?? []
for w in list where (w[kCGWindowOwnerPID as String] as? Int32) == pid && (w[kCGWindowLayer as String] as? Int) == 0 {
    if let b = w[kCGWindowBounds as String] as? [String: Any], let h = b["Height"] as? Double, h > 100 {
        print(w[kCGWindowNumber as String]!)
        break
    }
}
