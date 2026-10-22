// Package downloader fetches Go toolchain archives from the official release
// manifest.
//
// The manifest is the JSON document served by https://go.dev/dl/?mode=json.
// It lists every published release together with the SHA-256 checksum and
// size of each archive, which is what makes it possible to download a
// toolchain without relying on the go command being installed.
package downloader

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/go-comm/gip/pkg/version"
)

// DefaultMirror is the official download site.
const DefaultMirror = "https://go.dev/dl"

// MirrorEnv is the environment variable that overrides the download site.
const MirrorEnv = "GIP_MIRROR"

// File describes one downloadable artifact of a release.
type File struct {
	Filename string `json:"filename"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Version  string `json:"version"`
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
	Kind     string `json:"kind"`
}

// Release describes one published Go release.
type Release struct {
	Version string `json:"version"`
	Stable  bool   `json:"stable"`
	Files   []File `json:"files"`
}

// Manifest is the list of published releases.
//
// The manifest served by the download site is a JSON array of releases.
type Manifest struct {
	Releases []Release
}

// UnmarshalJSON accepts both the array form used by the download site and
// an object with a "releases" member.
func (m *Manifest) UnmarshalJSON(data []byte) error {
	var list []Release
	if err := json.Unmarshal(data, &list); err == nil {
		m.Releases = list
		return nil
	}
	var obj struct {
		Releases []Release `json:"releases"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	m.Releases = obj.Releases
	return nil
}

// Mirror returns the download site to use, honouring $GIP_MIRROR.
//
// A mirror must serve the same manifest format, for example
// "https://golang.google.cn/dl".
func Mirror() string {
	if m := strings.TrimSpace(os.Getenv(MirrorEnv)); m != "" {
		return m
	}
	return DefaultMirror
}

// ManifestURL returns the URL of the release manifest for a mirror.
func ManifestURL(mirror string) string {
	return strings.TrimRight(mirror, "/") + "/?mode=json&include=all"
}

// FileURL returns the URL of an archive on a mirror.
func FileURL(mirror, filename string) string {
	return strings.TrimRight(mirror, "/") + "/" + filename
}

// FetchManifest downloads and parses the release manifest.
//
// The manifest is fetched with a few attempts, because a network hiccup
// should not stop a command that would otherwise succeed.
func FetchManifest(ctx context.Context, mirror string) (*Manifest, error) {
	url := ManifestURL(mirror)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			}
		}
		m, err := fetchManifest(ctx, url)
		if err == nil {
			return m, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// fetchManifest downloads and parses the manifest once.
func fetchManifest(ctx context.Context, url string) (*Manifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot fetch release manifest from %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cannot fetch release manifest from %s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("cannot parse release manifest from %s: %v", url, err)
	}
	if len(m.Releases) == 0 {
		return nil, fmt.Errorf("release manifest from %s is empty", url)
	}
	return &m, nil
}

// ReleaseVersion returns the manifest name of a release, for example
// "go1.22.0" for the version "1.22.0".
func ReleaseVersion(v string) string {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "go") {
		return v
	}
	return "go" + v
}

// Find returns the release and the archive matching a version and a
// platform.
//
// The version is the plain release name such as "1.22.0"; the "go" prefix is
// optional.
func (m *Manifest) Find(v, goos, goarch string) (*Release, *File, error) {
	want := ReleaseVersion(v)
	for i := range m.Releases {
		rel := &m.Releases[i]
		if rel.Version != want {
			continue
		}
		for j := range rel.Files {
			f := &rel.Files[j]
			if f.OS == goos && f.Arch == goarch && f.Kind == "archive" {
				return rel, f, nil
			}
		}
		return rel, nil, fmt.Errorf("no %s/%s archive for %s", goos, goarch, want)
	}
	return nil, nil, fmt.Errorf("unknown Go release %s", want)
}

// Latest returns the newest stable release in the manifest.
func (m *Manifest) Latest() (*Release, error) {
	var best *Release
	for i := range m.Releases {
		rel := &m.Releases[i]
		if !rel.Stable {
			continue
		}
		v := TrimReleasePrefix(rel.Version)
		if version.Parse(v).Kind != "" {
			continue // skip prereleases
		}
		if best == nil || version.Compare(v, TrimReleasePrefix(best.Version)) > 0 {
			best = rel
		}
	}
	if best == nil {
		return nil, fmt.Errorf("no stable release in the manifest")
	}
	return best, nil
}

// Sorted returns the releases of the manifest, newest first.
func (m *Manifest) Sorted() []*Release {
	out := make([]*Release, 0, len(m.Releases))
	for i := range m.Releases {
		out = append(out, &m.Releases[i])
	}
	sort.SliceStable(out, func(i, j int) bool {
		return version.Compare(TrimReleasePrefix(out[i].Version), TrimReleasePrefix(out[j].Version)) > 0
	})
	return out
}

// TrimReleasePrefix removes the "go" prefix of a release name.
func TrimReleasePrefix(rel string) string {
	return version.TrimGo(rel)
}

// Platform returns the OS and architecture gip is running on.
func Platform() (goos, goarch string) {
	return runtime.GOOS, runtime.GOARCH
}
