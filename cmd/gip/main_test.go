package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-comm/gip/internal/store"
)

func TestSplitGlobalFlags(t *testing.T) {
	tests := []struct {
		args       []string
		wantVer    string
		wantNoDl   bool
		wantMirror string
		wantRest   []string
	}{
		{
			args:     []string{"run", "."},
			wantRest: []string{"run", "."},
		},
		{
			args:     []string{"--go", "1.22.0", "run", "."},
			wantVer:  "1.22.0",
			wantRest: []string{"run", "."},
		},
		{
			args:     []string{"--go=1.22.0", "run", "."},
			wantVer:  "1.22.0",
			wantRest: []string{"run", "."},
		},
		{
			// A flag may also be written with a single dash, like the flag
			// package of the go command allows.
			args:     []string{"-go=1.22.0", "run", "."},
			wantVer:  "1.22.0",
			wantRest: []string{"run", "."},
		},
		{
			args:     []string{"-go", "1.22.0", "run", "."},
			wantVer:  "1.22.0",
			wantRest: []string{"run", "."},
		},
		{
			args:     []string{"run", "-go=1.22.0", "."},
			wantVer:  "1.22.0",
			wantRest: []string{"run", "."},
		},
		{
			args:     []string{"--no-download", "run", "."},
			wantNoDl: true,
			wantRest: []string{"run", "."},
		},
		{
			args:     []string{"-no-download", "run", "."},
			wantNoDl: true,
			wantRest: []string{"run", "."},
		},
		{
			args:       []string{"--mirror", "https://example.com/dl", "ls"},
			wantMirror: "https://example.com/dl",
			wantRest:   []string{"ls"},
		},
		{
			args:       []string{"-mirror=https://example.com/dl", "ls"},
			wantMirror: "https://example.com/dl",
			wantRest:   []string{"ls"},
		},
		{
			// The flags of the go command must not be taken by gip.
			args:     []string{"ls", "--all"},
			wantRest: []string{"ls", "--all"},
		},
		{
			args:     []string{"doc", "--all", "fmt"},
			wantRest: []string{"doc", "--all", "fmt"},
		},
		{
			// Everything after the separator is passed through untouched.
			args:     []string{"run", ".", "--", "--go", "1.0.0"},
			wantRest: []string{"run", ".", "--", "--go", "1.0.0"},
		},
		{
			// Flags of the go command are not mistaken for flags of gip.
			args:     []string{"build", "-o", "app.exe", "./cmd/..."},
			wantRest: []string{"build", "-o", "app.exe", "./cmd/..."},
		},
	}

	for _, tt := range tests {
		opts, rest := splitGlobalFlags(tt.args)
		if opts.version != tt.wantVer {
			t.Errorf("splitGlobalFlags(%v) version = %q, want %q", tt.args, opts.version, tt.wantVer)
		}
		if opts.noDownload != tt.wantNoDl {
			t.Errorf("splitGlobalFlags(%v) noDownload = %v, want %v", tt.args, opts.noDownload, tt.wantNoDl)
		}
		if opts.mirror != tt.wantMirror {
			t.Errorf("splitGlobalFlags(%v) mirror = %q, want %q", tt.args, opts.mirror, tt.wantMirror)
		}
		if len(rest) != len(tt.wantRest) {
			t.Errorf("splitGlobalFlags(%v) rest = %v, want %v", tt.args, rest, tt.wantRest)
			continue
		}
		for i := range rest {
			if rest[i] != tt.wantRest[i] {
				t.Errorf("splitGlobalFlags(%v) rest = %v, want %v", tt.args, rest, tt.wantRest)
				break
			}
		}
	}
}

// newTestStore returns a store holding a single fake release.
func newTestStore(t *testing.T, v string) *store.Store {
	t.Helper()
	s := store.New(t.TempDir())
	if err := s.Ensure(); err != nil {
		t.Fatal(err)
	}
	dir := s.VersionDir(v)
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.GoBinaryIn(dir), []byte("x"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureVersionDirs(v); err != nil {
		t.Fatal(err)
	}
	return s
}

// chdir changes the working directory for the duration of a test.
func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })
}

func TestResolveRelease(t *testing.T) {
	s := newTestStore(t, "1.22.0")
	if err := s.Use("1.22.0"); err != nil {
		t.Fatal(err)
	}
	if err := s.EnsureVersionDirs("1.21.0"); err != nil {
		t.Fatal(err)
	}
	// A second release of an older series, so that the go directive can be
	// satisfied without a download.
	older := newTestStore(t, "1.21.0")
	for _, v := range []string{"1.21.0"} {
		if err := copyRelease(older, s, v); err != nil {
			t.Fatal(err)
		}
	}

	mod := t.TempDir()
	chdir(t, mod)

	// Without a go.mod the current release is used.
	got, err := resolveRelease(s, options{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "1.22.0" {
		t.Errorf("resolveRelease = %q, want 1.22.0", got)
	}

	// The go directive states a minimum: the release of the same series is
	// used, even when it is older than the current one.
	if err := os.WriteFile(filepath.Join(mod, "go.mod"), []byte("module m\n\ngo 1.21.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	got, err = resolveRelease(s, options{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "1.21.0" {
		t.Errorf("resolveRelease = %q, want 1.21.0", got)
	}

	// An explicit version wins over the go.mod.
	got, err = resolveRelease(s, options{version: "1.22.0"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "1.22.0" {
		t.Errorf("resolveRelease = %q, want 1.22.0", got)
	}

	// A missing release is reported instead of being downloaded when
	// downloading is disabled.
	if err := os.WriteFile(filepath.Join(mod, "go.mod"), []byte("module m\n\ngo 1.30.0\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveRelease(s, options{noDownload: true}, false); err == nil {
		t.Error("resolveRelease did not report the missing release")
	} else if !strings.Contains(err.Error(), "1.30.0") {
		t.Errorf("unexpected error: %v", err)
	}
}

// copyRelease copies a fake release from one store into another.
func copyRelease(from, to *store.Store, v string) error {
	data, err := os.ReadFile(store.GoBinaryIn(from.VersionDir(v)))
	if err != nil {
		return err
	}
	dir := to.VersionDir(v)
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(store.GoBinaryIn(dir), data, 0755); err != nil {
		return err
	}
	return to.EnsureVersionDirs(v)
}

func TestResolveReleaseInvalidVersion(t *testing.T) {
	s := newTestStore(t, "1.22.0")
	if _, err := resolveRelease(s, options{version: "nonsense"}, true); err == nil {
		t.Error("resolveRelease accepted an invalid version")
	}
}

func TestLooksLikeToolchainVersion(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"1.22.0", true},
		{"1.22", true},
		{"go1.22.0", true},
		{"1.21rc1", true},
		{"1", false},
		{"golang.org/x/tools/cmd/stringer@latest", false},
		{"stringer", false},
		{"v1.2.3", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := looksLikeToolchainVersion(tt.in); got != tt.want {
			t.Errorf("looksLikeToolchainVersion(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
