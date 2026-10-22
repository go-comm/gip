// Package installer downloads Go toolchains and installs them into a store.
package installer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/go-comm/gip/internal/downloader"
	"github.com/go-comm/gip/internal/store"
	"github.com/go-comm/gip/pkg/version"
)

// Installer installs Go releases into a store.
type Installer struct {
	// Store is the destination of the installations.
	Store *store.Store
	// Mirror is the download site to use. When empty the value of
	// $GIP_MIRROR is used, falling back to the official site.
	Mirror string
	// Out receives progress messages.
	Out io.Writer
}

// Install installs the requested version and returns the name of the release
// that was installed.
//
// When exact is false a language version such as "1.22" selects the newest
// release of that series, which is what a user asking for "1.22" usually
// means. When exact is true the request is resolved to a single release,
// the way the go directive of a go.mod file is.
//
// Installing a version that is already present is a no-op.
func (in *Installer) Install(ctx context.Context, req string, exact bool) (string, error) {
	if in.Store == nil {
		return "", errors.New("no store configured")
	}
	if in.Mirror == "" {
		in.Mirror = downloader.Mirror()
	}
	if in.Out == nil {
		in.Out = io.Discard
	}

	norm := version.TrimGo(strings.TrimSpace(req))

	if !version.IsValid(norm) && !strings.EqualFold(norm, "latest") {
		return "", fmt.Errorf("invalid Go version %q", req)
	}
	if err := in.Store.Ensure(); err != nil {
		return "", err
	}

	// "latest" is the newest stable release.
	if strings.EqualFold(norm, "latest") {
		return in.installLatest(ctx)
	}

	// Work out which release the request refers to. A request for a whole
	// series such as "1.22" means the newest release of that series, which
	// can only be found in the manifest, so the shortcut below does not
	// apply to it.
	if exact {
		canon, ok := version.Canonical(norm)
		if !ok {
			return "", fmt.Errorf("invalid Go version %q", req)
		}
		if in.Store.IsInstalled(canon) {
			fmt.Fprintf(in.Out, "go%s is already installed\n", canon)
			return canon, nil
		}
	}

	m, err := downloader.FetchManifest(ctx, in.Mirror)
	if err != nil {
		return "", err
	}
	release, err := selectRelease(m, norm, exact)
	if err != nil {
		return "", err
	}
	if in.Store.IsInstalled(release) {
		fmt.Fprintf(in.Out, "go%s is already installed\n", release)
		return release, nil
	}

	if err := in.install(ctx, m, release); err != nil {
		return "", err
	}
	return release, nil
}

// install downloads and unpacks a release into the store.
func (in *Installer) install(ctx context.Context, m *downloader.Manifest, release string) error {
	goos, goarch := downloader.Platform()
	_, file, err := m.Find(release, goos, goarch)
	if err != nil {
		return err
	}

	// Serialize concurrent installations of the same release.
	unlock, err := downloader.Acquire(downloader.LockPath(in.Store.VersionsDir(), release), 15*time.Minute)
	if err != nil {
		return err
	}
	defer unlock()

	// Another process may have finished the installation while we waited.
	if in.Store.IsInstalled(release) {
		return nil
	}

	// The archive is downloaded next to its destination so that the final
	// move is a rename on the same file system.
	archive, err := os.CreateTemp(in.Store.VersionsDir(), ".tmp-download-*")
	if err != nil {
		return err
	}
	archivePath := archive.Name()
	archive.Close()
	defer os.Remove(archivePath)

	fmt.Fprintf(in.Out, "downloading go%s for %s/%s\n", release, goos, goarch)
	if err := downloader.Download(ctx, in.Mirror, file, archivePath); err != nil {
		return err
	}

	extractDir, err := os.MkdirTemp(in.Store.VersionsDir(), ".tmp-extract-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(extractDir)

	if err := downloader.Extract(archivePath, extractDir, file.Filename); err != nil {
		return err
	}

	// The extracted directory is the GOROOT of the release; make sure it
	// really holds a go command before it is published.
	if _, err := os.Stat(store.GoBinaryIn(extractDir)); err != nil {
		return fmt.Errorf("the archive of go%s does not contain a go command", release)
	}

	// Discard a partial installation left behind by an earlier failure.
	if err := os.RemoveAll(in.Store.VersionDir(release)); err != nil {
		return err
	}
	if err := os.Rename(extractDir, in.Store.VersionDir(release)); err != nil {
		return fmt.Errorf("cannot install go%s: %v", release, err)
	}
	if err := in.Store.EnsureVersionDirs(release); err != nil {
		return err
	}
	fmt.Fprintf(in.Out, "installed go%s in %s\n", release, in.Store.VersionDir(release))
	return nil
}

// installLatest installs the newest stable release.
func (in *Installer) installLatest(ctx context.Context) (string, error) {
	m, err := downloader.FetchManifest(ctx, in.Mirror)
	if err != nil {
		return "", err
	}
	rel, err := m.Latest()
	if err != nil {
		return "", err
	}
	release := downloader.TrimReleasePrefix(rel.Version)
	if in.Store.IsInstalled(release) {
		fmt.Fprintf(in.Out, "go%s is already installed\n", release)
		return release, nil
	}
	if err := in.install(ctx, m, release); err != nil {
		return "", err
	}
	return release, nil
}

// selectRelease maps a requested version to the name of a release.
//
// In exact mode the request names one release. Otherwise the newest release
// of the same series is used, which has to be not older than the request.
func selectRelease(m *downloader.Manifest, norm string, exact bool) (string, error) {
	if exact {
		canon, ok := version.Canonical(norm)
		if !ok {
			return "", fmt.Errorf("invalid Go version %q", norm)
		}
		return canon, nil
	}

	lang := version.Lang(norm)
	best := ""
	for _, rel := range m.Releases {
		r := downloader.TrimReleasePrefix(rel.Version)
		if version.Lang(r) != lang {
			continue
		}
		if version.Parse(r).Kind != "" {
			// Skip prereleases such as 1.22rc1.
			continue
		}
		if version.Compare(r, norm) < 0 {
			// Older than the request.
			continue
		}
		if best == "" || version.Compare(r, best) > 0 {
			best = r
		}
	}
	if best == "" {
		return "", fmt.Errorf("no release of Go %s is available", norm)
	}
	return best, nil
}
