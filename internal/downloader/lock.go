package downloader

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Lock serializes concurrent installations of the same release.
//
// Two terminals asking for the same missing toolchain would otherwise both
// download and unpack it into the same directory. The lock is a file that is
// created exclusively; a lock left behind by a crashed process is considered
// stale and taken over after a while.
type Lock struct {
	path string
}

// LockPath returns the path of the lock guarding a release.
func LockPath(versionsDir, v string) string {
	return filepath.Join(versionsDir, "."+v+".lock")
}

// Acquire takes the lock guarding a release, waiting for up to timeout for
// another process to release it.
//
// The returned function releases the lock.
func Acquire(path string, timeout time.Duration) (func(), error) {
	deadline := time.Now().Add(timeout)
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			fmt.Fprintf(f, "%d\n", os.Getpid())
			f.Close()
			return func() { os.Remove(path) }, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}

		// Take over a lock that is obviously stale.
		if fi, serr := os.Stat(path); serr == nil && time.Since(fi.ModTime()) > 30*time.Minute {
			os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for %s", path)
		}
		time.Sleep(200 * time.Millisecond)
	}
}
