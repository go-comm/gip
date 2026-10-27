//go:build go1.18
// +build go1.18

package compat

// Go 1.18 added bytes.Cut; bytes_legacy.go has the equivalent for the older
// releases.

import "bytes"

// Cut slices s around the first instance of sep, returning the text before and
// after sep. The found result reports whether sep appears in s. If sep does
// not appear, Cut returns s, nil, false.
func Cut(s, sep []byte) (before, after []byte, found bool) {
	return bytes.Cut(s, sep)
}
