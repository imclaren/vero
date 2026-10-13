// winid PID: the number of the process's main window on screen, for
// screencapture -l, or nothing while it has none. A window of the usual
// layer comes first; a menu bar app's window, which floats above the
// others, comes next. Anything 100 points high or less, such as the menu
// bar item itself, is left out. record-mac.sh builds it.
import CoreGraphics

let pid = Int32(CommandLine.arguments[1]) ?? 0
let list = CGWindowListCopyWindowInfo([.optionOnScreenOnly], kCGNullWindowID) as? [[String: Any]] ?? []
let tall = list.filter { w in
    guard (w[kCGWindowOwnerPID as String] as? Int32) == pid,
          let b = w[kCGWindowBounds as String] as? [String: Any], let h = b["Height"] as? Double else { return false }
    return h > 100
}
if let w = tall.first(where: { ($0[kCGWindowLayer as String] as? Int) == 0 }) ?? tall.first {
    print(w[kCGWindowNumber as String]!)
}
