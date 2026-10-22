package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newStore returns a store with a fake release installed.
func newStore(t *testing.T, versions ...string) *Store {
	t.Helper()
	s := New(t.TempDir())
	if err := s.Ensure(); err != nil {
		t.Fatal(err)
	}
	for _, v := range versions {
		dir := s.VersionDir(v)
		if err := os.MkdirAll(filepath.Join(dir, "bin"), 0755); err != nil {
			t.Fatal(err)
		}
		name := "go"
		if isWindows() {
			name = "go.exe"
		}
		if err := os.WriteFile(filepath.Join(dir, "bin", name), []byte("binary"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := s.EnsureVersionDirs(v); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func TestInstalled(t *testing.T) {
	s := newStore(t, "1.22.0", "1.19.10", "1.21.0")
	got, err := s.Installed()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"1.19.10", "1.21.0", "1.22.0"}
	if len(got) != len(want) {
		t.Fatalf("Installed() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Installed()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestUseAndCurrent(t *testing.T) {
	s := newStore(t, "1.22.0", "1.19.10")

	if cur, err := s.Current(); err != nil || cur != "" {
		t.Fatalf("Current() = %q, %v; want \"\", nil", cur, err)
	}

	if err := s.Use("1.22.0"); err != nil {
		t.Fatal(err)
	}
	cur, err := s.Current()
	if err != nil {
		t.Fatal(err)
	}
	if cur != "1.22.0" {
		t.Errorf("Current() = %q, want 1.22.0", cur)
	}

	// The bin link has to point at the release.
	target, err := os.Readlink(s.CurrentBinLink())
	if err != nil {
		t.Fatal(err)
	}
	if v, ok := versionFromPath(target); !ok || v != "1.22.0" {
		t.Errorf("current/bin points at %q, want the 1.22.0 release", target)
	}

	// Switching again replaces both links.
	if err := s.Use("1.19.10"); err != nil {
		t.Fatal(err)
	}
	if cur, _ := s.Current(); cur != "1.19.10" {
		t.Errorf("Current() = %q, want 1.19.10", cur)
	}
	target, err = os.Readlink(s.CurrentGoBinLink())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filepath.ToSlash(target), "gobin/1.19.10") {
		t.Errorf("current/gobin points at %q, want the gobin directory of 1.19.10", target)
	}

	// No temporary files may be left behind.
	entries, err := os.ReadDir(s.CurrentDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) != "" {
			t.Errorf("leftover file in the current directory: %s", e.Name())
		}
	}
}

func TestUseNotInstalled(t *testing.T) {
	s := newStore(t, "1.22.0")
	if err := s.Use("1.19.10"); err == nil {
		t.Error("Use of an uninstalled release succeeded, want an error")
	}
}

func TestResolve(t *testing.T) {
	s := newStore(t, "1.21.0", "1.22.0", "1.22.5")
	tests := []struct {
		req  string
		want string
	}{
		{"1.21.0", "1.21.0"},
		{"1.22.0", "1.22.0"},
		{"1.22", "1.22.5"},
		{"1.21", "1.21.0"},
		{"1.19", ""},
	}
	for _, tt := range tests {
		got, ok := s.Resolve(tt.req)
		if tt.want == "" {
			if ok {
				t.Errorf("Resolve(%q) = %q, want no match", tt.req, got)
			}
			continue
		}
		if !ok || got != tt.want {
			t.Errorf("Resolve(%q) = %q, %v; want %q", tt.req, got, ok, tt.want)
		}
	}
}

func TestVersionFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"../versions/1.22.0/bin", "1.22.0"},
		{`C:\gip\versions\1.19.10\bin`, "1.19.10"},
		{"/home/u/.gip/versions/1.21.0/bin", "1.21.0"},
		{"../gobin/1.22.0", ""},
	}
	for _, tt := range tests {
		got, ok := versionFromPath(tt.path)
		if tt.want == "" {
			if ok {
				t.Errorf("versionFromPath(%q) = %q, want no match", tt.path, got)
			}
			continue
		}
		if !ok || got != tt.want {
			t.Errorf("versionFromPath(%q) = %q, %v; want %q", tt.path, got, ok, tt.want)
		}
	}
}
