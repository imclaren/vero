import Foundation

/// Go's time.Time as JSON carries it: RFC 3339, with up to nine fractional
/// digits (`2026-10-07T05:35:02.123456789Z`) or an offset (`+10:00`). vero
/// gen's types read and write their Dates with it. Go's zero time,
/// `0001-01-01T00:00:00Z`, is `Date.distantPast`, the same instant.
public enum VeroDate {
    /// The date in s, or nil when s is not RFC 3339.
    public static func parse(_ s: String) -> Date? {
        let normalised = threeDigits(s)
        if let date = try? Date(normalised, strategy: Date.ISO8601FormatStyle(includingFractionalSeconds: true)) {
            return date
        }
        return try? Date(normalised, strategy: Date.ISO8601FormatStyle())
    }

    /// The date as Go reads a time.Time, in UTC, to the millisecond.
    public static func format(_ date: Date) -> String {
        date.formatted(Date.ISO8601FormatStyle(includingFractionalSeconds: true))
    }

    /// Exactly three fractional digits, which the ISO 8601 style reads
    /// reliably, where Go writes up to nine.
    private static func threeDigits(_ s: String) -> String {
        guard let t = s.firstIndex(of: "T"), let dot = s[t...].firstIndex(of: ".") else { return s }
        var end = s.index(after: dot)
        while end < s.endIndex, s[end].isNumber { end = s.index(after: end) }
        let digits = s[s.index(after: dot)..<end]
        return String(s[..<dot]) + "." + String((digits + "000").prefix(3)) + String(s[end...])
    }
}
