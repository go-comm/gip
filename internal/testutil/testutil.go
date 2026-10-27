// Package testutil holds what the tests of gip need from Go versions newer
// than the one gip builds with.
//
// gip supports Go 1.13 and later, and the testing package grew the helpers
// below over several releases, so the tests carry their own.
package testutil

import (
	"os"
	"sync"
	"testing"

	"github.com/go-comm/gip/internal/compat"
)

var (
	mu    sync.Mutex
	undos []func()
)

// TempDir creates a temporary directory and returns its path. The directory is
// removed when the test binary exits. It is t.TempDir, which arrived in Go
// 1.15.
func TempDir(t *testing.T) string {
	t.Helper()
	dir, err := compat.TempDir("", "gip-test-")
	if err != nil {
		t.Fatal(err)
	}
	OnCleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// Setenv sets the environment variable key to value. The previous value is
// restored when the test binary exits. It is t.Setenv, which arrived in Go
// 1.17.
func Setenv(t *testing.T, key, value string) {
	t.Helper()
	old, had := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatal(err)
	}
	OnCleanup(func() {
		if had {
			_ = os.Setenv(key, old)
			return
		}
		_ = os.Unsetenv(key)
	})
}

// OnCleanup registers a function that runs when the test binary exits. It is
// t.Cleanup, which arrived in Go 1.14, with the lifetime of the whole binary
// rather than of a single test.
func OnCleanup(f func()) {
	mu.Lock()
	undos = append(undos, f)
	mu.Unlock()
}

// Cleanup runs the registered functions in reverse order. A TestMain function
// has to call it after m.Run.
func Cleanup() {
	mu.Lock()
	u := undos
	undos = nil
	mu.Unlock()
	for i := len(u) - 1; i >= 0; i-- {
		u[i]()
	}
}
