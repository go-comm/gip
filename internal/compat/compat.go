// Package compat holds the standard library calls whose spelling differs
// between the oldest Go release gip builds with (1.13) and the newer ones.
//
// Every call has two implementations, picked by build constraints: the files
// behind "go1.16" and "go1.18" use the functions that the os, io and bytes
// packages grew, and the legacy files use io/ioutil and a hand-written
// equivalent. The rest of gip calls these functions and stays free of the
// version split.
package compat

// DirEntry is what ReadDir returns: the name of a directory entry and whether
// it is a directory.
//
// Both os.FileInfo (which io/ioutil.ReadDir returns) and fs.DirEntry (which
// os.ReadDir returns) satisfy it, which is what lets the two implementations
// of ReadDir share a signature.
type DirEntry interface {
	Name() string
	IsDir() bool
}
