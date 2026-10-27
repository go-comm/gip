//go:build !go1.18
// +build !go1.18

package compat

// Go 1.13 to 1.17 predate runtime/debug.ReadBuildInfo, so a binary built with
// those releases carries no build information at all.

// BuildVersion reports that the build information carries no version.
//
// The runtime only keeps the build information since Go 1.18, and gip builds
// with Go 1.13 and later: a binary built with an older toolchain reports the
// version of the source tree instead, which is what the caller falls back to.
func BuildVersion(fallback string) (string, bool) {
	return "", false
}
