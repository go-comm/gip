package store

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// cwdMu serializes the temporary chdir performed by createLink.
var cwdMu sync.Mutex

// createLink creates a symbolic link at linkPath whose target is target.
//
// On Windows creating a symbolic link requires a privilege that is not
// granted by default, so a directory junction is used as a fallback. A
// junction always stores an absolute path, which is why the relative target
// is resolved before the junction is created.
//
// The link is created from inside its own directory: os.Symlink inspects the
// target to find out whether it points at a directory, and it resolves
// relative targets against the working directory.
func createLink(linkPath, target string) error {
	os.Remove(linkPath)

	if err := symlink(linkPath, target); err == nil {
		return nil
	} else if runtime.GOOS != "windows" {
		return err
	}

	absTarget, err := linkTargetPath(linkPath, target)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(linkPath), 0755); err != nil {
		return err
	}
	cmd := exec.Command("cmd", "/c", "mklink", "/J", linkPath, absTarget)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("cannot link %s to %s: %s", linkPath, target, msg)
	}
	return nil
}

// linkTargetPath resolves a link target against the directory holding the
// link. A relative target of a link in dir is relative to dir, not to the
// working directory.
func linkTargetPath(linkPath, target string) (string, error) {
	if filepath.IsAbs(target) {
		return filepath.Clean(target), nil
	}
	return filepath.Clean(filepath.Join(filepath.Dir(linkPath), target)), nil
}

// symlink creates a symbolic link at linkPath pointing at target.
//
// A relative target is resolved against the directory holding the link
// rather than against the working directory. os.Symlink inspects its target
// to find out whether it points at a directory, and that inspection has to
// yield the right answer for the link to be usable on Windows.
func symlink(linkPath, target string) error {
	if !filepath.IsAbs(target) {
		cwdMu.Lock()
		defer cwdMu.Unlock()

		old, err := os.Getwd()
		if err != nil {
			return os.Symlink(target, linkPath)
		}
		if err := os.Chdir(filepath.Dir(linkPath)); err != nil {
			return os.Symlink(target, linkPath)
		}
		err = os.Symlink(target, linkPath)
		os.Chdir(old)
		return err
	}
	return os.Symlink(target, linkPath)
}

// replaceLink moves the link at tmp over the link at dst.
//
// On Unix the rename replaces the existing link atomically. On Windows a
// rename over a directory junction is not always possible, so the previous
// link is moved aside first and restored if the replacement fails.
func replaceLink(dst, tmp string) error {
	if err := os.Rename(tmp, dst); err == nil {
		return nil
	}

	backup := dst + ".bak"
	os.Remove(backup)
	if err := os.Rename(dst, backup); err != nil {
		return fmt.Errorf("cannot replace %s: %v", dst, err)
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Rename(backup, dst)
		return fmt.Errorf("cannot replace %s: %v", dst, err)
	}
	os.Remove(backup)
	return nil
}

// isWindows reports whether gip is running on Windows.
func isWindows() bool { return runtime.GOOS == "windows" }
