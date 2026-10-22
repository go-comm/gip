package downloader

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// writeTarGz builds a tar.gz archive holding a single "go" directory.
func writeTarGz(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	files := map[string]string{
		"go/bin/go":      "binary",
		"go/src/main.go": "package main",
		"go/VERSION":     "go1.22.0",
	}
	for name, body := range files {
		hdr := &tar.Header{
			Name:     name,
			Mode:     0755,
			Size:     int64(len(body)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
}

// writeZip builds a zip archive holding a single "go" directory.
func writeZip(t *testing.T, path string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	defer zw.Close()

	for name, body := range map[string]string{
		"go/bin/go.exe":  "binary",
		"go/src/main.go": "package main",
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestExtractTarGz(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "go1.22.0.linux-amd64.tar.gz")
	writeTarGz(t, archive)

	dest := filepath.Join(dir, "out")
	if err := Extract(archive, dest, filepath.Base(archive)); err != nil {
		t.Fatal(err)
	}

	// The top level "go" directory has to be stripped.
	for _, rel := range []string{"bin/go", "src/main.go", "VERSION"} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(rel))); err != nil {
			t.Errorf("missing %s: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dest, "go")); err == nil {
		t.Error("the top level directory was not stripped")
	}
}

func TestExtractZip(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "go1.22.0.windows-amd64.zip")
	writeZip(t, archive)

	dest := filepath.Join(dir, "out")
	if err := Extract(archive, dest, filepath.Base(archive)); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"bin/go.exe", "src/main.go"} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(rel))); err != nil {
			t.Errorf("missing %s: %v", rel, err)
		}
	}
}

func TestExtractRejectsUnknownFormat(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "go1.22.0.rar")
	if err := os.WriteFile(archive, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := Extract(archive, dir, filepath.Base(archive)); err == nil {
		t.Error("Extract accepted an unknown archive format")
	}
}

// TestExtractRefusesToEscape makes sure a malicious archive cannot write
// outside of the destination directory.
func TestExtractRefusesToEscape(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "evil.zip")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("../escaped.txt")
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte("nope"))
	zw.Close()
	f.Close()

	dest := filepath.Join(dir, "out")
	if err := Extract(archive, dest, filepath.Base(archive)); err == nil {
		t.Error("Extract accepted an entry escaping the destination directory")
	}
}
