//go:build !go1.18
// +build !go1.18

package compat

// Go 1.13 to 1.17 predate bytes.Cut, so this file writes the same call out.

import "bytes"

// Cut slices s around the first instance of sep, returning the text before and
// after sep. The found result reports whether sep appears in s. If sep does
// not appear, Cut returns s, nil, false.
func Cut(s, sep []byte) (before, after []byte, found bool) {
	if i := bytes.Index(s, sep); i >= 0 {
		return s[:i], s[i+len(sep):], true
	}
	return s, nil, false
}
