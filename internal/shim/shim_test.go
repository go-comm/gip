// Package shim keeps the versioned names of the go command and of the tools of
// a release in the bin directory of the store.
package shim

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/go-comm/gip/internal/store"
	"github.com/go-comm/gip/internal/testutil"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testutil.Cleanup()
	os.Exit(code)
}

func TestNameParseRoundTrip(t *testing.T) {
	cases := []struct {
		tool    string
		release string
	}{
		{"go", "1.20.14"},
		{"gofmt", "1.20.14"},
		{"gopls", "1.20.14"},
		{"dlv", "1.20.14"},
		{"go", "1.22.0"},
		{"go", "1.22rc1"},
		{"gofmt", "1.22.0"},
		{"custom-tool", "1.20.14"},
	}

	for _, tc := range cases {
		name := Name(tc.tool, tc.release)
		tool, release, ok := Parse(name)
		if !ok {
			t.Errorf("Parse(%q) = %q, %q, %v; want ok=true", name, tool, release, ok)
			continue
		}
		if tool != tc.tool {
			t.Errorf("Parse(%q) tool = %q; want %q", name, tool, tc.tool)
		}
		if release != tc.release {
			t.Errorf("Parse(%q) release = %q; want %q", name, release, tc.release)
		}
	}
}

func TestParseNonShimNames(t *testing.T) {
	nonShims := []string{
		"go",
		"gofmt",
		"gopls",
		"dlv",
		"go1",
		"go1.20",
		"go1.20.",
		"go1.20.14.foo",
		"gofmt1.20",
		"gofmt1.20.",
		"gopls-go",
		"gopls-go1.20",
		"gopls-go1.20.",
		"gofoo",
		"gofoo-bar",
		"not-a-tool",
		"",
	}
	for _, n := range nonShims {
		_, _, ok := Parse(n)
		if ok {
			t.Errorf("Parse(%q) = ok; want ok=false", n)
		}
	}
}

func TestParseIgnoresCase(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("the file system separates the case of a name")
	}
	for _, n := range []string{"Go1.20.14.exe", "GOFMT1.20.14.EXE", "GoPls-Go1.20.14.exe"} {
		tool, release, ok := Parse(n)
		if !ok || release != "1.20.14" {
			t.Errorf("Parse(%q) = %q, %q, %v; want a name of 1.20.14", n, tool, release, ok)
		}
	}
	if tool, _, _ := Parse("GoPls-Go1.20.14.exe"); tool != "gopls" {
		t.Errorf("Parse(\"GoPls-Go1.20.14.exe\") tool = %q; want \"gopls\"", tool)
	}
}

func TestEnsureRemove(t *testing.T) {
	// Use a temp store rooted at a temp directory.
	root := testutil.TempDir(t)
	st := store.New(root)

	// Create a fake release layout.
	release := "1.20.14"
	if err := os.MkdirAll(st.VersionDir(release)+"/bin", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(st.GoBinDir(release), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(st.BinDir(), 0755); err != nil {
		t.Fatal(err)
	}

	// Place fake binaries.
	for _, tool := range []string{"go", "gofmt", "gopls", "dlv"} {
		path := filepath.Join(st.GoBinDir(release), tool+exeSuffix())
		if err := writeEmpty(path); err != nil {
			t.Fatal(err)
		}
	}
	// go and gofmt also in versions/<rel>/bin
	for _, tool := range []string{"go", "gofmt"} {
		path := filepath.Join(st.VersionDir(release), "bin", tool+exeSuffix())
		if err := writeEmpty(path); err != nil {
			t.Fatal(err)
		}
	}

	// The current gip binary (the test binary).
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	// Ensure creates the names.
	if err := Ensure(st, exe, release); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	// Check bin directory has the expected names.
	expected := []string{
		"go" + release + exeSuffix(),
		"gofmt" + release + exeSuffix(),
		"gopls-go" + release + exeSuffix(),
		"dlv-go" + release + exeSuffix(),
	}
	for _, exp := range expected {
		path := filepath.Join(st.BinDir(), exp)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("missing %s: %v", exp, err)
		} else if runtime.GOOS != "windows" {
			// On Unix, verify it's a hard link (or at least same file) to exe.
			if !sameFile(exe, path) {
				t.Errorf("%s is not a link to the running binary", exp)
			}
		}
	}

	// Remove deletes the names of the release.
	if err := Remove(st, release); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	for _, exp := range expected {
		path := filepath.Join(st.BinDir(), exp)
		if _, err := os.Stat(path); err == nil {
			t.Errorf("%s still exists after Remove", exp)
		}
	}
}

func TestEnsureRemovesStale(t *testing.T) {
	root := testutil.TempDir(t)
	st := store.New(root)

	release := "1.20.14"
	if err := os.MkdirAll(st.VersionDir(release)+"/bin", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(st.GoBinDir(release), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(st.BinDir(), 0755); err != nil {
		t.Fatal(err)
	}

	// Old tool that should be removed.
	old := filepath.Join(st.BinDir(), "oldtool-go"+release+exeSuffix())
	if err := writeEmpty(old); err != nil {
		t.Fatal(err)
	}

	// Current tools.
	for _, tool := range []string{"go", "gofmt", "gopls"} {
		path := filepath.Join(st.GoBinDir(release), tool+exeSuffix())
		if err := writeEmpty(path); err != nil {
			t.Fatal(err)
		}
	}
	for _, tool := range []string{"go", "gofmt"} {
		path := filepath.Join(st.VersionDir(release), "bin", tool+exeSuffix())
		if err := writeEmpty(path); err != nil {
			t.Fatal(err)
		}
	}

	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	if err := Ensure(st, exe, release); err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	// Old tool should be gone.
	if _, err := os.Stat(old); err == nil {
		t.Error("stale tool oldtool was not removed")
	}

	// New tools should exist.
	for _, exp := range []string{"go" + release + exeSuffix(), "gofmt" + release + exeSuffix(), "gopls-go" + release + exeSuffix()} {
		path := filepath.Join(st.BinDir(), exp)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("missing %s: %v", exp, err)
		}
	}
}

func writeEmpty(path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	return f.Close()
}
