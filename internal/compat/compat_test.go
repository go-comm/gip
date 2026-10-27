package compat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileCalls(t *testing.T) {
	dir, err := TempDir("", "gip-compat-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "a.txt")
	if err := WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	data, err := ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Errorf("ReadFile = %q, want hello", string(data))
	}

	if err := os.Mkdir(filepath.Join(dir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	entries, err := ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]bool, len(entries))
	for _, e := range entries {
		got[e.Name()] = e.IsDir()
	}
	if len(got) != 2 || got["a.txt"] || !got["sub"] {
		t.Errorf("ReadDir = %v, want a.txt as a file and sub as a directory", got)
	}
}

func TestTempFile(t *testing.T) {
	f, err := TempFile("", "gip-compat-*")
	if err != nil {
		t.Fatal(err)
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(name)

	if _, err := os.Stat(name); err != nil {
		t.Errorf("the temporary file is not there: %v", err)
	}
}

func TestReadAll(t *testing.T) {
	data, err := ReadAll(strings.NewReader("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Errorf("ReadAll = %q, want hello", string(data))
	}
}

func TestDiscard(t *testing.T) {
	n, err := Discard.Write([]byte("hello"))
	if n != 5 || err != nil {
		t.Errorf("Discard.Write = %d, %v; want 5, nil", n, err)
	}
}

// TestBuildVersion checks the contract of the two implementations: either the
// build information carries a version, or the caller falls back to the version
// of the source tree. Which of the two happens depends on the toolchain the
// test binary was built with and on how it was built.
func TestBuildVersion(t *testing.T) {
	v, ok := BuildVersion("v0.0.0")
	if ok && v == "" {
		t.Error("BuildVersion reported a version but returned an empty string")
	}
	if !ok && v != "" {
		t.Errorf("BuildVersion reported no version but returned %q", v)
	}
}

func TestCut(t *testing.T) {
	tests := []struct {
		s, sep                string
		wantBefore, wantAfter string
		wantFound             bool
	}{
		{"a=b", "=", "a", "b", true},
		{"a=b=c", "=", "a", "b=c", true},
		{"=b", "=", "", "b", true},
		{"abc", "=", "abc", "", false},
		{"", "=", "", "", false},
	}
	for _, tt := range tests {
		before, after, found := Cut([]byte(tt.s), []byte(tt.sep))
		if string(before) != tt.wantBefore || string(after) != tt.wantAfter || found != tt.wantFound {
			t.Errorf("Cut(%q, %q) = %q, %q, %v; want %q, %q, %v",
				tt.s, tt.sep, before, after, found, tt.wantBefore, tt.wantAfter, tt.wantFound)
		}
	}
}
