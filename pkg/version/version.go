// Package version implements parsing, normalization and comparison of Go
// toolchain versions such as "1.21", "1.21.0" and "1.21rc1".
//
// The rules implemented here mirror the Go standard library's internal/gover
// package (used by cmd/go to pick a toolchain). Keeping the same semantics is
// what makes gip select exactly the same toolchain that the official go
// command would select for a given go.mod or go.work file.
//
// The most important consequence of that ordering is:
//
//	1.21 < 1.21rc1 < 1.21.0
//
// The "language version" 1.21 compares *less* than the release 1.21.0, and
// both 1.21rc1 and 1.21.0 are able to build a module whose go directive says
// "go 1.21". Before Go 1.21 the picture was simpler: "1.20" and "1.20.0"
// denote the very same release, because there was no patch component at all.
//
// The implementation is derived from the Go standard library
// (https://go.googlesource.com/go), licensed under the BSD 3-Clause license.
package version

import "strings"

// A Version is a parsed Go toolchain version: major[.minor[.patch]][kind[pre]].
//
// The numeric fields are kept as decimal strings rather than integers so that
// arbitrarily large version numbers parse correctly on 32-bit platforms.
//
// The zero Version is the "invalid" version. It compares less than every
// well-formed version, exactly like the Go standard library does.
type Version struct {
	Major string // decimal, e.g. "1"
	Minor string // decimal or ""
	Patch string // decimal or ""
	Kind  string // "", "alpha", "beta" or "rc"
	Pre   string // decimal or ""

	// patchSet reports whether the patch component was spelled out in the
	// original string. Go treats "1.20" and "1.20.0" as the same version but
	// "1.21" and "1.21.0" as different ones, so the distinction has to
	// survive parsing even though it does not affect ordering.
	patchSet bool
}

// IsValid reports whether v is a well-formed version.
func (v Version) IsValid() bool { return v != Version{} }

// String renders the version back to its textual form, preserving the
// distinction between "1.21" and "1.21.0".
func (v Version) String() string {
	if v.Major == "" {
		return ""
	}
	s := v.Major
	if v.Minor != "" {
		s += "." + v.Minor
	}
	if v.Patch != "" && v.patchSet {
		s += "." + v.Patch
	}
	if v.Kind != "" {
		s += v.Kind + v.Pre
	}
	return s
}

// TrimGo removes a leading "go" prefix from a version string.
//
// Only "go" immediately followed by a digit is stripped, so that unrelated
// strings such as "golang.org/x" are left untouched.
func TrimGo(s string) string {
	if len(s) > 2 && s[0] == 'g' && s[1] == 'o' && s[2] >= '0' && s[2] <= '9' {
		return s[2:]
	}
	return s
}

// Parse parses a Go version string.
//
// An optional "go" prefix is accepted, so "go1.21.3" and "1.21.3" parse
// identically. Parse returns the zero Version if x is malformed.
func Parse(x string) Version {
	var v Version
	var ok bool

	x = TrimGo(strings.TrimSpace(x))

	// Major version.
	v.Major, x, ok = cutInt(x)
	if !ok {
		return Version{}
	}
	if x == "" {
		// Interpret "1" as "1.0.0".
		v.Minor = "0"
		v.Patch = "0"
		v.patchSet = true
		return v
	}
	if x[0] != '.' {
		return Version{}
	}

	// Minor version.
	v.Minor, x, ok = cutInt(x[1:])
	if !ok {
		return Version{}
	}
	if x == "" {
		// A missing patch is the same as "0" for versions before Go 1.21.
		// Starting with Go 1.21 a missing patch is different from an
		// explicit ".0".
		if cmpInt(v.Minor, "21") < 0 {
			v.Patch = "0"
		}
		return v
	}

	// Patch version.
	if x[0] == '.' {
		v.Patch, x, ok = cutInt(x[1:])
		if !ok || x != "" {
			// Prereleases of a patch release are deliberately not allowed:
			// "1.21.3rc1 < 1.21.3" would contradict "1.21 < 1.21rc1".
			return Version{}
		}
		v.patchSet = true
		return v
	}

	// Prerelease suffix: alpha, beta or rc followed by a number.
	i := 0
	for i < len(x) && (x[i] < '0' || x[i] > '9') {
		if x[i] < 'a' || x[i] > 'z' {
			return Version{}
		}
		i++
	}
	if i == 0 {
		return Version{}
	}
	v.Kind, x = x[:i], x[i:]
	if x == "" {
		return v
	}
	v.Pre, x, ok = cutInt(x)
	if !ok || x != "" {
		return Version{}
	}
	return v
}

// Compare returns -1, 0 or +1 depending on whether x is less than, equal to
// or greater than y, using Go's toolchain version ordering.
//
// Malformed versions compare less than well-formed ones and equal to each
// other. Note that "1.21" is *less* than "1.21.0" while "1.20" and "1.20.0"
// are equal.
func Compare(x, y string) int {
	vx := Parse(x)
	vy := Parse(y)

	if c := cmpInt(vx.Major, vy.Major); c != 0 {
		return c
	}
	if c := cmpInt(vx.Minor, vy.Minor); c != 0 {
		return c
	}
	if c := cmpInt(vx.Patch, vy.Patch); c != 0 {
		return c
	}
	// "" < alpha < beta < rc
	if c := strings.Compare(vx.Kind, vy.Kind); c != 0 {
		return c
	}
	if c := cmpInt(vx.Pre, vy.Pre); c != 0 {
		return c
	}
	return 0
}

// IsLang reports whether x denotes an overall Go language version rather
// than a specific release.
//
// Starting with Go 1.21, "1.x" denotes the overall language version and the
// first release of that version is "1.x.0". Before Go 1.21 the patch
// component did not exist, so "1.20" is a concrete release and not a
// language version.
func IsLang(x string) bool {
	v := Parse(x)
	return v.IsValid() && v.Patch == "" && v.Kind == "" && v.Pre == ""
}

// Lang returns the Go language version of x. For example Lang("1.2.3") is
// "1.2" and Lang("1.21rc1") is "1.21".
func Lang(x string) string {
	v := Parse(x)
	if !v.IsValid() {
		return ""
	}
	if v.Minor == "" || (v.Major == "1" && v.Minor == "0") {
		return v.Major
	}
	return v.Major + "." + v.Minor
}

// IsValid reports whether x is a well-formed Go version.
func IsValid(x string) bool { return Parse(x).IsValid() }

// FromToolchain returns the Go version for a named toolchain such as
// "go1.2.3". The name must start with "go" and may carry a custom suffix,
// which is stripped: FromToolchain("go1.2.3-bigcorp") is "1.2.3".
//
// The second return value is false if name is not a valid toolchain name.
func FromToolchain(name string) (string, bool) {
	if strings.ContainsAny(name, `\/`) {
		// The suffix must not include a path separator, since that would
		// cause a lookup in $PATH to resolve it from a relative directory
		// instead of from $PATH itself.
		return "", false
	}
	if !strings.HasPrefix(name, "go") {
		return "", false
	}
	v := name[2:]
	if i := strings.IndexAny(v, " \t-"); i >= 0 {
		v = v[:i]
	}
	if !IsValid(v) {
		return "", false
	}
	return v, true
}

// IsRelease reports whether the version names one exact release, as opposed
// to a language version that stands for a whole series of releases.
//
// "1.22.0" and "1.22rc1" name one release, while "1.22" stands for the
// series of 1.22 releases.
func IsRelease(x string) bool {
	v := Parse(x)
	if !v.IsValid() {
		return false
	}
	if v.Kind != "" {
		// A prerelease is one release.
		return true
	}
	return v.patchSet
}

// Canonical returns the name of the release that provides the given version.
//
// It is the version string that matches an entry of the official release
// manifest:
//
//	"1.21"   -> "1.21.0"  (Go 1.21 and later ship an explicit ".0")
//	"1.20"   -> "1.20"    (older releases have no patch component)
//	"1.21.3" -> "1.21.3"
//	"1.21rc1"-> "1.21rc1"
func Canonical(x string) (string, bool) {
	v := Parse(x)
	if !v.IsValid() {
		return "", false
	}
	if IsLang(x) && Compare(x, "1.21") >= 0 {
		// Do not use a language version (such as 1.22) to look up a
		// toolchain; use the first release of that language version
		// instead, such as 1.22.0.
		return v.String() + ".0", true
	}
	return v.String(), true
}

// cutInt scans the leading decimal number at the start of x and returns that
// number and the rest of the string.
func cutInt(x string) (n, rest string, ok bool) {
	i := 0
	for i < len(x) && x[i] >= '0' && x[i] <= '9' {
		i++
	}
	if i == 0 || (x[0] == '0' && i != 1) {
		// No digits, or an unnecessary leading zero.
		return "", "", false
	}
	return x[:i], x[i:], true
}

// cmpInt compares x and y as decimal numbers.
func cmpInt(x, y string) int {
	if x == y {
		return 0
	}
	if len(x) < len(y) {
		return -1
	}
	if len(x) > len(y) {
		return +1
	}
	if x < y {
		return -1
	}
	return +1
}
