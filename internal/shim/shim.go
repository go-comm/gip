// Package shim keeps the versioned names of the go command and of the tools of
// a release in the bin directory of the store.
//
// A name spells the release it belongs to, so one directory on the PATH
// reaches every release:
//
//	go1.20.14.exe        the go command of 1.20.14
//	gofmt1.20.14.exe     the gofmt of 1.20.14
//	gopls-go1.20.14.exe  the gopls of the GOBIN of 1.20.14
//
// What a name runs is gip itself, which looks at the name it was started under
// and runs that command of that release with the environment of the release.
// The name cannot be a link to the command itself: the go command derives
// GOROOT from the path of its own binary, so a copy of it outside the GOROOT of
// its release looks for the standard library in the wrong place.
package shim

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/go-comm/gip/internal/compat"
	"github.com/go-comm/gip/internal/store"
	"github.com/go-comm/gip/pkg/version"
)

// The tools of a release itself are named after themselves; a tool of the
// GOBIN of a release names the release with a "go" prefix, so that
// gopls-go1.20.14 is the gopls of that release.
var rootTools = map[string]bool{"go": true, "gofmt": true}

// Name returns the file name of a tool of a release.
func Name(tool, release string) string {
	if rootTools[tool] {
		return tool + release + exeSuffix()
	}
	return tool + "-go" + release + exeSuffix()
}

// Parse reports the tool and the release a file name stands for.
func Parse(base string) (tool, release string, ok bool) {
	base = trimExe(base)
	if runtime.GOOS == "windows" {
		// The file system of Windows does not tell the names apart by case,
		// so "Go1.20.14.exe" is the same name as "go1.20.14.exe".
		base = strings.ToLower(base)
	}
	if strings.HasPrefix(base, "gofmt") {
		if v := base[len("gofmt"):]; isReleaseName(v) {
			return "gofmt", v, true
		}
	}
	if strings.HasPrefix(base, "go") {
		if v := base[len("go"):]; isReleaseName(v) {
			return "go", v, true
		}
	}
	if i := strings.LastIndex(base, "-go"); i > 0 {
		if v := base[i+len("-go"):]; isReleaseName(v) {
			return base[:i], v, true
		}
	}
	return "", "", false
}

// isReleaseName reports whether the version part of a file name spells one
// release.
//
// A name has to spell the release the way a release is installed, with a minor
// version, so that "go1" is not mistaken for the release 1.0.0 that
// pkg/version also sees in it.
func isReleaseName(v string) bool {
	return strings.Contains(v, ".") && version.IsRelease(v)
}

// Binary returns the command a tool of a release runs, and reports whether the
// release provides it.
func Binary(st *store.Store, tool, release string) (string, bool) {
	dir := st.GoBinDir(release)
	if rootTools[tool] {
		dir = filepath.Join(st.VersionDir(release), "bin")
	}
	path := filepath.Join(dir, tool+exeSuffix())
	if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
		return path, true
	}
	return "", false
}

// Tools returns the tools of a release that have a command: the go command and
// gofmt of the release, and every tool of its GOBIN.
func Tools(st *store.Store, release string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(tool string) {
		if seen[tool] {
			return
		}
		if _, ok := Binary(st, tool, release); ok {
			seen[tool] = true
			out = append(out, tool)
		}
	}
	add("go")
	add("gofmt")
	entries, err := compat.ReadDir(st.GoBinDir(release))
	if err != nil {
		return out
	}
	for _, e := range entries {
		if e.IsDir() || !isToolFile(e.Name()) {
			continue
		}
		add(trimExe(e.Name()))
	}
	return out
}

// Ensure makes the bin directory hold a name for every command of the release
// and no name for a command it no longer has.
//
// exe is the binary the names run, which is the gip binary that is running.
func Ensure(st *store.Store, exe, release string) error {
	if exe == "" {
		return errors.New("no binary for the names to run")
	}
	binDir := st.BinDir()
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return err
	}

	want := map[string]bool{}
	for _, tool := range Tools(st, release) {
		name := Name(tool, release)
		want[name] = true
		if err := link(exe, filepath.Join(binDir, name)); err != nil {
			// best effort: warn but continue with the other names
			fmt.Fprintf(os.Stderr, "gip: warning: could not create shim %s: %v\n", name, err)
		}
	}

	entries, err := compat.ReadDir(binDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		name := e.Name()
		_, rel, ok := Parse(name)
		if !ok || rel != release || want[name] {
			continue
		}
		if err := os.Remove(filepath.Join(binDir, name)); err != nil && !os.IsNotExist(err) {
			// best effort: warn but continue
			fmt.Fprintf(os.Stderr, "gip: warning: could not remove stale shim %s: %v\n", name, err)
		}
	}
	return nil
}

// Remove deletes the names of a release from the bin directory.
func Remove(st *store.Store, release string) error {
	entries, err := compat.ReadDir(st.BinDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		_, rel, ok := Parse(e.Name())
		if !ok || rel != release {
			continue
		}
		if err := os.Remove(filepath.Join(st.BinDir(), e.Name())); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// isToolFile reports whether a file of a GOBIN directory is an installed
// command.
func isToolFile(name string) bool {
	if strings.HasPrefix(name, ".") {
		// The locks and the like that belong to the directory.
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Ext(name), ".exe")
	}
	return true
}

// link makes name run exe.
//
// A link is preferred over a copy so that the names of the bin directory
// change together with the gip binary instead of drifting away from it, and so
// that a release does not cost a copy of gip per command. A file system that
// cannot link across the two directories gets a copy.
func link(exe, name string) error {
	if fi, err := os.Stat(name); err == nil {
		if !fi.IsDir() && sameFile(exe, name) {
			return nil
		}
		if err := os.Remove(name); err != nil {
			return err
		}
	}
	if err := os.Link(exe, name); err == nil {
		return nil
	}
	return copyFile(exe, name)
}

// sameFile reports whether the two paths name the same file.
func sameFile(a, b string) bool {
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(ai, bi)
}

// copyFile copies the executable exe to name.
func copyFile(exe, name string) error {
	in, err := os.Open(exe)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// exeSuffix is the extension of an executable on Windows and empty elsewhere.
func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

// trimExe removes the extension of an executable.
func trimExe(name string) string {
	if runtime.GOOS == "windows" && strings.EqualFold(filepath.Ext(name), ".exe") {
		return name[:len(name)-len(".exe")]
	}
	return name
}
