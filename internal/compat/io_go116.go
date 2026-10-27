//go:build go1.16
// +build go1.16

package compat

// Go 1.16 moved the calls below from io/ioutil to os and io. gip builds with
// Go 1.13 and later, so this file is the one that runs on a current
// toolchain; io_legacy.go spells the same calls the old way.

import (
	"io"
	"os"
)

// ReadFile reads the named file.
func ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

// WriteFile writes data to the named file, creating it with perm when it does
// not exist.
func WriteFile(name string, data []byte, perm os.FileMode) error {
	return os.WriteFile(name, data, perm)
}

// ReadDir reads the named directory.
func ReadDir(name string) ([]DirEntry, error) {
	entries, err := os.ReadDir(name)
	if err != nil {
		return nil, err
	}
	out := make([]DirEntry, len(entries))
	for i, e := range entries {
		out[i] = e
	}
	return out, nil
}

// TempFile creates a new temporary file in dir and returns it open.
func TempFile(dir, pattern string) (*os.File, error) {
	return os.CreateTemp(dir, pattern)
}

// TempDir creates a new temporary directory in dir and returns its path.
func TempDir(dir, pattern string) (string, error) {
	return os.MkdirTemp(dir, pattern)
}

// ReadAll reads r until EOF.
func ReadAll(r io.Reader) ([]byte, error) {
	return io.ReadAll(r)
}

// Discard is an io.Writer on which all writes succeed.
var Discard io.Writer = io.Discard
