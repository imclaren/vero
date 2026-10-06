package main

import (
	"strings"
)

// compareVersions compares two Debian versions, as dpkg does: an epoch
// before a colon, then the upstream version, then a revision after the
// last hyphen. Letters sort before everything else but a tilde, which
// sorts before even the end, so 1.0~beta comes before 1.0.
func compareVersions(a, b string) int {
	ae, au, ar := splitVersion(a)
	be, bu, br := splitVersion(b)
	if c := compareNumbers(ae, be); c != 0 {
		return c
	}
	if c := compareParts(au, bu); c != 0 {
		return c
	}
	return compareParts(ar, br)
}

func splitVersion(v string) (epoch, upstream, revision string) {
	epoch = "0"
	if i := strings.IndexByte(v, ':'); i >= 0 {
		epoch, v = v[:i], v[i+1:]
	}
	if i := strings.LastIndexByte(v, '-'); i >= 0 {
		return epoch, v[:i], v[i+1:]
	}
	return epoch, v, ""
}

// order is where a character sorts in a version's non-digit parts.
func order(c byte) int {
	switch {
	case c == '~':
		return -1
	case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z':
		return int(c)
	default:
		return int(c) + 256
	}
}

func compareParts(a, b string) int {
	for a != "" || b != "" {
		// The part that is not digits, character by character.
		for (a != "" && !isDigit(a[0])) || (b != "" && !isDigit(b[0])) {
			ac, bc := 0, 0
			if a != "" && !isDigit(a[0]) {
				ac = order(a[0])
			}
			if b != "" && !isDigit(b[0]) {
				bc = order(b[0])
			}
			if ac != bc {
				if ac < bc {
					return -1
				}
				return 1
			}
			if a != "" && !isDigit(a[0]) {
				a = a[1:]
			}
			if b != "" && !isDigit(b[0]) {
				b = b[1:]
			}
		}
		// Then the digits, as a number.
		var an, bn string
		an, a = digits(a)
		bn, b = digits(b)
		if c := compareNumbers(an, bn); c != 0 {
			return c
		}
	}
	return 0
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func digits(s string) (string, string) {
	i := 0
	for i < len(s) && isDigit(s[i]) {
		i++
	}
	return s[:i], s[i:]
}

// compareNumbers compares two runs of digits of any length.
func compareNumbers(a, b string) int {
	a, b = strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
	if len(a) != len(b) {
		if len(a) < len(b) {
			return -1
		}
		return 1
	}
	return strings.Compare(a, b)
}
