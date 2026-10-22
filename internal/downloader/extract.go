package downloader

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Extract unpacks a Go toolchain archive into destDir.
//
// The archives published by the Go project contain a single top level
// directory named "go"; that directory is stripped so that destDir becomes
// the GOROOT itself, holding bin, src, pkg and lib.
//
// Both the Unix archives (.tar.gz) and the Windows ones (.zip) are
// supported. The format is taken from the name of the archive, which is the
// file name published in the manifest; it is passed separately because the
// archive may be stored under a temporary name.
func Extract(archivePath, destDir, name string) error {
	switch {
	case strings.HasSuffix(name, ".zip"):
		return extractZip(archivePath, destDir)
	case strings.HasSuffix(name, ".tar.gz"), strings.HasSuffix(name, ".tgz"):
		return extractTarGz(archivePath, destDir)
	default:
		return fmt.Errorf("unsupported archive format: %s", filepath.Base(name))
	}
}

// topLevel returns the name of the single top level directory of an archive,
// or "" if the archive does not have one.
func topLevel(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	i := strings.Index(name, "/")
	if i <= 0 {
		return ""
	}
	top := name[:i]
	// A relative or absolute first element is not a directory to strip.
	if top == "." || top == ".." || strings.Contains(top, ":") {
		return ""
	}
	return top
}

// stripTop removes the top level directory from an archive entry name.
func stripTop(name, top string) (string, bool) {
	name = strings.ReplaceAll(name, "\\", "/")
	if top == "" {
		return name, name != ""
	}
	if name == top {
		return "", false
	}
	if strings.HasPrefix(name, top+"/") {
		return name[len(top)+1:], true
	}
	return name, true
}

// targetPath joins a relative archive entry name with destDir, refusing
// entries that would escape the destination directory.
func targetPath(destDir, name string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(name))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("archive entry %q escapes the destination directory", name)
	}
	// A name that still holds a parent element after cleaning is refused as
	// well, for instance "a/../../b".
	for _, part := range strings.Split(filepath.ToSlash(clean), "/") {
		if part == ".." {
			return "", fmt.Errorf("archive entry %q escapes the destination directory", name)
		}
	}
	return filepath.Join(destDir, clean), nil
}

// extractTarGz unpacks a gzip compressed tar archive.
func extractTarGz(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("cannot read %s: %v", filepath.Base(archivePath), err)
	}
	defer gz.Close()

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	tr := tar.NewReader(gz)
	top := ""
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("cannot read %s: %v", filepath.Base(archivePath), err)
		}
		if top == "" {
			top = topLevel(hdr.Name)
		}
		name, ok := stripTop(hdr.Name, top)
		if !ok || name == "" {
			continue
		}
		path, err := targetPath(destDir, name)
		if err != nil {
			return err
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}
			mode := hdr.FileInfo().Mode().Perm()
			if mode == 0 {
				mode = 0644
			}
			out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		case tar.TypeSymlink:
			// Go distributions do not ship symbolic links, but handle them
			// anyway rather than silently dropping them.
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}
			os.Remove(path)
			if err := os.Symlink(hdr.Linkname, path); err != nil {
				return err
			}
		default:
			// Anything else (devices, fifos) is not part of a Go
			// distribution.
		}
	}
	return nil
}

// extractZip unpacks a zip archive.
func extractZip(archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("cannot read %s: %v", filepath.Base(archivePath), err)
	}
	defer r.Close()

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	top := ""
	for _, f := range r.File {
		if top == "" {
			top = topLevel(f.Name)
		}
	}
	for _, f := range r.File {
		name, ok := stripTop(f.Name, top)
		if !ok || name == "" {
			continue
		}
		path, err := targetPath(destDir, name)
		if err != nil {
			return err
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}
		mode := f.Mode().Perm()
		if mode == 0 {
			mode = 0644
		}
		out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			rc.Close()
			out.Close()
			return err
		}
		rc.Close()
		if err := out.Close(); err != nil {
			return err
		}
	}
	return nil
}
