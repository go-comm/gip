// Package store manages the on-disk layout of gip: the installed toolchains,
// their per-version build directories and the "current" selection.
//
// The layout under the gip home directory is:
//
//	~/.gip/
//	├── versions/
//	│   ├── 1.19.10/        <- GOROOT of that release
//	│   └── 1.22.0/
//	├── gobin/
//	│   ├── 1.19.10/        <- GOBIN of that release
//	│   └── 1.22.0/
//	├── gocache/
//	│   ├── 1.19.10/        <- GOCACHE of that release
//	│   └── 1.22.0/
//	└── current/
//	    ├── gobin  -> ../gobin/1.22.0
//	    └── goroot -> ../versions/1.22.0
//
// Note that GOROOT must never point at "current" itself: the directory only
// holds the gobin and goroot links and is missing the src, pkg and lib
// directories that a GOROOT requires. The goroot link points at a whole
// GOROOT and is the path to hand to tools that expect one, such as the
// go.goroot setting of the VS Code Go extension. Its bin directory is the
// one to put in front of the PATH.
package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-comm/gip/pkg/version"
)

// DefaultHomeEnv is the environment variable that overrides the location of
// the gip home directory.
const DefaultHomeEnv = "GIP_HOME"

// Store gives access to the gip home directory.
type Store struct {
	// Root is the gip home directory, for example /home/user/.gip.
	Root string
}

// New returns a Store rooted at the given directory.
func New(root string) *Store { return &Store{Root: root} }

// Default returns the Store for the current user.
//
// The home directory is taken from $GIP_HOME if set, otherwise from the
// user home directory.
func Default() (*Store, error) {
	if root := os.Getenv(DefaultHomeEnv); root != "" {
		abs, err := filepath.Abs(root)
		if err != nil {
			return nil, err
		}
		return New(abs), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %v", err)
	}
	return New(filepath.Join(home, ".gip")), nil
}

// VersionsDir is the directory holding the installed toolchains.
func (s *Store) VersionsDir() string { return filepath.Join(s.Root, "versions") }

// VersionDir is the GOROOT of the given release.
func (s *Store) VersionDir(v string) string {
	return filepath.Join(s.VersionsDir(), v)
}

// GoBinDir is the GOBIN of the given release.
func (s *Store) GoBinDir(v string) string {
	return filepath.Join(s.Root, "gobin", v)
}

// GoCacheDir is the GOCACHE of the given release.
func (s *Store) GoCacheDir(v string) string {
	return filepath.Join(s.Root, "gocache", v)
}

// CurrentDir is the directory holding the "current" links.
func (s *Store) CurrentDir() string { return filepath.Join(s.Root, "current") }

// CurrentGoBinLink is the link to the gobin directory of the current release.
func (s *Store) CurrentGoBinLink() string {
	return filepath.Join(s.CurrentDir(), "gobin")
}

// GorootLink is the link to the GOROOT of the current release.
//
// It points at a whole GOROOT, with its src, pkg and lib directories, so it
// can be handed to tools that expect a GOROOT, such as the go.goroot setting
// of the VS Code Go extension. Its path stays the same when the release
// changes, and its bin directory holds the go command.
func (s *Store) GorootLink() string {
	return filepath.Join(s.CurrentDir(), "goroot")
}

// GorootBinDir is the directory with the go command of the current release.
//
// It is the directory to put in front of the PATH.
func (s *Store) GorootBinDir() string {
	return filepath.Join(s.GorootLink(), "bin")
}

// GorootVersion returns the release the GOROOT link points at.
func (s *Store) GorootVersion() (string, bool) {
	target, err := os.Readlink(s.GorootLink())
	if err != nil {
		return "", false
	}
	return versionFromPath(target)
}

// metaPath is the file recording which release is current.
func (s *Store) metaPath() string { return filepath.Join(s.Root, "current.version") }

// GoBinary returns the path of the go command of the given release.
func (s *Store) GoBinary(v string) string {
	return GoBinaryIn(s.VersionDir(v))
}

// GoBinaryIn returns the path of the go command inside a GOROOT directory.
func GoBinaryIn(dir string) string {
	name := "go"
	if isWindows() {
		name = "go.exe"
	}
	return filepath.Join(dir, "bin", name)
}

// Ensure creates the gip home directory skeleton.
func (s *Store) Ensure() error {
	for _, dir := range []string{s.Root, s.VersionsDir(), filepath.Join(s.Root, "gobin"), filepath.Join(s.Root, "gocache")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

// EnsureVersionDirs creates the per-version gobin and gocache directories.
func (s *Store) EnsureVersionDirs(v string) error {
	for _, dir := range []string{s.GoBinDir(v), s.GoCacheDir(v)} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}

// IsInstalled reports whether the given release is installed and usable.
func (s *Store) IsInstalled(v string) bool {
	fi, err := os.Stat(s.GoBinary(v))
	return err == nil && !fi.IsDir()
}

// Installed returns the installed releases, oldest first.
func (s *Store) Installed() ([]string, error) {
	entries, err := os.ReadDir(s.VersionsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() || !version.IsValid(e.Name()) {
			continue
		}
		if !s.IsInstalled(e.Name()) {
			continue
		}
		out = append(out, e.Name())
	}
	sort.Slice(out, func(i, j int) bool {
		return version.Compare(out[i], out[j]) < 0
	})
	return out, nil
}

// Current returns the version that "current" points at, or an empty string
// if no release has been selected yet.
func (s *Store) Current() (string, error) {
	if v, ok := s.currentFromLink(); ok {
		return v, nil
	}
	// Fall back to the recorded metadata, which also survives a manual
	// removal of the links.
	data, err := os.ReadFile(s.metaPath())
	if err != nil {
		return "", nil
	}
	v := strings.TrimSpace(string(data))
	if !version.IsValid(v) {
		return "", nil
	}
	return v, nil
}

// currentFromLink derives the current version from the target of the
// goroot link.
func (s *Store) currentFromLink() (string, bool) {
	target, err := os.Readlink(s.GorootLink())
	if err != nil {
		return "", false
	}
	return versionFromPath(target)
}

// versionFromPath extracts a release name from a path that points into the
// versions directory.
//
// The link target may be relative ("../versions/1.22.0/bin") or absolute,
// depending on the platform and on whether the link was created as a
// symbolic link or as a Windows junction.
func versionFromPath(target string) (string, bool) {
	parts := strings.FieldsFunc(target, func(r rune) bool {
		return r == '/' || r == '\\'
	})
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == "versions" && version.IsValid(parts[i+1]) {
			return parts[i+1], true
		}
	}
	return "", false
}

// Resolve maps a requested version to an installed release.
//
// The lookup tries, in order, the exact name and then the newest installed
// release of the same series that is not older than the request. The second
// step is what makes "1.22" find an installed 1.22.5, while an explicit
// "1.22.0" always selects exactly that release.
func (s *Store) Resolve(req string) (string, bool) {
	installed, err := s.Installed()
	if err != nil {
		return "", false
	}
	for _, v := range installed {
		if v == req {
			return v, true
		}
	}
	lang := version.Lang(req)
	best := ""
	for _, v := range installed {
		if version.Lang(v) != lang {
			continue
		}
		if version.Compare(v, req) < 0 {
			continue
		}
		if best == "" || version.Compare(v, best) > 0 {
			best = v
		}
	}
	if best == "" {
		return "", false
	}
	return best, true
}

// ResolveSeries maps a version requirement to an installed release.
//
// The newest installed release of the same series that is not older than the
// requirement is returned, so a requirement of "1.21" is satisfied by an
// installed 1.21.13. A requirement of "1.21.0" is not satisfied by 1.20.x.
//
// This is what the go directive of a module asks for: it states a minimum,
// not a pin.
func (s *Store) ResolveSeries(req string) (string, bool) {
	installed, err := s.Installed()
	if err != nil {
		return "", false
	}
	lang := version.Lang(req)
	best := ""
	for _, v := range installed {
		if version.Lang(v) != lang {
			continue
		}
		if version.Compare(v, req) < 0 {
			continue
		}
		if best == "" || version.Compare(v, best) > 0 {
			best = v
		}
	}
	if best == "" {
		return "", false
	}
	return best, true
}

// link is a symbolic link of the current directory.
type link struct {
	path   string
	target string
}

// currentLinks returns the links of the current directory for a release.
//
// The targets are relative to the current directory, so that the whole gip
// home directory can be moved.
func (s *Store) currentLinks(v string) []link {
	return []link{
		{s.CurrentGoBinLink(), filepath.Join("..", "gobin", v)},
		{s.GorootLink(), filepath.Join("..", "versions", v)},
	}
}

// Use makes the given release the current one.
//
// The bin, gobin and goroot links are switched together, so that the
// compiler, the tools installed next to it and the GOROOT always come from
// the same release. The links are staged under temporary names and then
// renamed into place; if a rename fails the previous selection is restored,
// so that it stays intact.
func (s *Store) Use(v string) error {
	if !version.IsValid(v) {
		return fmt.Errorf("invalid version %q", v)
	}
	if !s.IsInstalled(v) {
		return fmt.Errorf("go %s is not installed; run \"gip install %s\" first", v, v)
	}
	if err := s.EnsureVersionDirs(v); err != nil {
		return err
	}
	if err := os.MkdirAll(s.CurrentDir(), 0755); err != nil {
		return err
	}

	links := s.currentLinks(v)

	// The previous selection, so that it can be restored if one of the
	// renames fails.
	prev, _ := s.Current()

	// Stage the links under temporary names.
	for _, l := range links {
		tmp := l.path + ".tmp"
		defer os.Remove(tmp)
		if err := createLink(tmp, l.target); err != nil {
			return err
		}
	}

	// Move them into place.
	for _, l := range links {
		if err := replaceLink(l.path, l.path+".tmp"); err != nil {
			s.restoreLinks(prev)
			return err
		}
	}

	return s.writeMeta(v)
}

// restoreLinks points the links of the current directory back at a release.
func (s *Store) restoreLinks(v string) {
	if v == "" {
		return
	}
	for _, l := range s.currentLinks(v) {
		tmp := l.path + ".rollback"
		if err := createLink(tmp, l.target); err != nil {
			continue
		}
		replaceLink(l.path, tmp)
		os.Remove(tmp)
	}
}

// writeMeta records the current release.
func (s *Store) writeMeta(v string) error {
	if err := os.MkdirAll(s.Root, 0755); err != nil {
		return err
	}
	return os.WriteFile(s.metaPath(), []byte(v+"\n"), 0644)
}

// Remove deletes an installed release along with its gobin and gocache
// directories. The current selection is left alone if it points at the
// release being removed.
func (s *Store) Remove(v string) error {
	if !s.IsInstalled(v) {
		return fmt.Errorf("go %s is not installed", v)
	}
	for _, dir := range []string{s.VersionDir(v), s.GoBinDir(v), s.GoCacheDir(v)} {
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
	}
	if cur, err := s.Current(); err == nil && cur == v {
		// The current release is gone, so the links that point at it would
		// dangle. The goroot link is read to find the current release, so
		// it has to be the last one to go.
		os.Remove(s.metaPath())
		os.Remove(s.CurrentGoBinLink())
		os.Remove(s.GorootLink())
	}
	return nil
}
