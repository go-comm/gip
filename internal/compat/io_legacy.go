//go:build !go1.16
// +build !go1.16

package compat

// Go 1.13 to 1.15 predate os.ReadFile, os.WriteFile, os.ReadDir,
// os.CreateTemp, os.MkdirTemp, io.ReadAll and io.Discard: this file spells the
// same calls with io/ioutil for those releases.

import (
	"io"
	"io/ioutil"
	"os"
)

// ReadFile reads the named file.
func ReadFile(name string) ([]byte, error) {
	return ioutil.ReadFile(name)
}

// WriteFile writes data to the named file, creating it with perm when it does
// not exist.
func WriteFile(name string, data []byte, perm os.FileMode) error {
	return ioutil.WriteFile(name, data, perm)
}

// ReadDir reads the named directory.
func ReadDir(name string) ([]DirEntry, error) {
	entries, err := ioutil.ReadDir(name)
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
	return ioutil.TempFile(dir, pattern)
}

// TempDir creates a new temporary directory in dir and returns its path.
func TempDir(dir, pattern string) (string, error) {
	return ioutil.TempDir(dir, pattern)
}

// ReadAll reads r until EOF.
func ReadAll(r io.Reader) ([]byte, error) {
	return ioutil.ReadAll(r)
}

// Discard is an io.Writer on which all writes succeed.
var Discard io.Writer = ioutil.Discard
