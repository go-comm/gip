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
//	    ├── bin   -> ../versions/1.22.0/bin
//	    └── gobin -> ../gobin/1.22.0
//
// Note that GOROOT must never point at "current": the directory only holds
// the bin and gobin links and is missing the src, pkg and lib directories
// that a GOROOT requires. The "current" links exist purely as a convenience
// for users who put ~/.gip/current/bin on their PATH.
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

// CurrentBinLink is the link to the bin directory of the current release.
func (s *Store) CurrentBinLink() string {
	return filepath.Join(s.CurrentDir(), "bin")
}

// CurrentGoBinLink is the link to the gobin directory of the current release.
func (s *Store) CurrentGoBinLink() string {
	return filepath.Join(s.CurrentDir(), "gobin")
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
// current/bin link.
func (s *Store) currentFromLink() (string, bool) {
	target, err := os.Readlink(s.CurrentBinLink())
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

// Use makes the given release the current one.
//
// Both the bin and the gobin link are switched, so that the compiler and the
// tools installed next to it always come from the same release. The links
// are staged under temporary names and then renamed into place; if the
// second rename fails the first one is rolled back, so the previous
// selection stays intact.
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

	tmpBin := s.CurrentBinLink() + ".tmp"
	tmpGoBin := s.CurrentGoBinLink() + ".tmp"
	defer os.Remove(tmpBin)
	defer os.Remove(tmpGoBin)

	relBin := filepath.Join("..", "versions", v, "bin")
	relGoBin := filepath.Join("..", "gobin", v)

	if err := createLink(tmpBin, relBin); err != nil {
		return err
	}
	if err := createLink(tmpGoBin, relGoBin); err != nil {
		return err
	}

	if err := replaceLink(s.CurrentBinLink(), tmpBin); err != nil {
		return err
	}
	if err := replaceLink(s.CurrentGoBinLink(), tmpGoBin); err != nil {
		// Roll the bin link back to the previous release.
		if prev, ok := s.currentFromLink(); ok && prev != v {
			rollback := tmpBin + ".rollback"
			if createLink(rollback, filepath.Join("..", "versions", prev, "bin")) == nil {
				replaceLink(s.CurrentBinLink(), rollback)
				os.Remove(rollback)
			}
		}
		return err
	}

	return s.writeMeta(v)
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
		os.Remove(s.metaPath())
	}
	return nil
}
