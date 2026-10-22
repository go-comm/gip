package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvironment(t *testing.T) {
	r := &Runner{
		VersionDir: filepath.Join("/gip", "versions", "1.22.0"),
		GoBinDir:   filepath.Join("/gip", "gobin", "1.22.0"),
		GoCacheDir: filepath.Join("/gip", "gocache", "1.22.0"),
	}
	base := []string{
		"PATH=" + filepath.Join("/usr", "bin"),
		"HOME=/home/user",
		"GOROOT=/somewhere/else",
		"GOBIN=/somewhere/else",
		"GOCACHE=/somewhere/else",
	}
	env := r.Environment(base)

	get := func(key string) string {
		for _, kv := range env {
			if strings.HasPrefix(kv, key+"=") {
				return kv[len(key)+1:]
			}
		}
		return ""
	}

	if got := get("GOROOT"); got != r.VersionDir {
		t.Errorf("GOROOT = %q, want %q", got, r.VersionDir)
	}
	if got := get("GOBIN"); got != r.GoBinDir {
		t.Errorf("GOBIN = %q, want %q", got, r.GoBinDir)
	}
	if got := get("GOCACHE"); got != r.GoCacheDir {
		t.Errorf("GOCACHE = %q, want %q", got, r.GoCacheDir)
	}
	if got := get("HOME"); got != "/home/user" {
		t.Errorf("HOME = %q, want /home/user", got)
	}
	if got := get("GOTOOLCHAIN"); got != "local" {
		t.Errorf("GOTOOLCHAIN = %q, want local", got)
	}

	// The bin directories of the release come first in PATH.
	path := get("PATH")
	sep := string(os.PathListSeparator)
	want := r.GoBinDir + sep + filepath.Join(r.VersionDir, "bin") + sep + filepath.Join("/usr", "bin")
	if path != want {
		t.Errorf("PATH = %q, want %q", path, want)
	}

	// A variable must not be set twice.
	counts := map[string]int{}
	for _, kv := range env {
		key := kv[:strings.IndexByte(kv, '=')]
		counts[key]++
	}
	for key, n := range counts {
		if n > 1 {
			t.Errorf("%s is set %d times", key, n)
		}
	}
}

func TestEnvironmentKeepsExplicitToolchain(t *testing.T) {
	t.Setenv("GOTOOLCHAIN", "go1.22.0+auto")
	r := &Runner{VersionDir: "/gip/versions/1.22.0"}
	env := r.Environment(os.Environ())
	for _, kv := range env {
		if kv == "GOTOOLCHAIN=go1.22.0+auto" {
			return
		}
	}
	t.Error("an explicit GOTOOLCHAIN was overwritten")
}
