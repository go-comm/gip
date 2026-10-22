package downloader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Download fetches an archive from the mirror and verifies it against the
// SHA-256 checksum published in the manifest.
//
// The file is written to a temporary name next to dest and only moved into
// place once the checksum matches, so a failed or truncated download never
// leaves a file that looks complete.
func Download(ctx context.Context, mirror string, f *File, dest string) error {
	url := FileURL(mirror, f.Filename)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("cannot download %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cannot download %s: %s", url, resp.Status)
	}

	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	tmp := dest + ".part"
	os.Remove(tmp)
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	defer func() {
		out.Close()
		os.Remove(tmp)
	}()

	h := sha256.New()
	progress := newProgress(os.Stderr, f)
	w := io.MultiWriter(out, h)
	if _, err := io.Copy(w, io.TeeReader(resp.Body, progress)); err != nil {
		return fmt.Errorf("cannot download %s: %v", url, err)
	}
	progress.done()

	sum := hex.EncodeToString(h.Sum(nil))
	if f.SHA256 != "" && sum != f.SHA256 {
		return fmt.Errorf("checksum mismatch for %s: got %s, want %s", f.Filename, sum, f.SHA256)
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
}

// progress reports the progress of a download on a single line.
type progress struct {
	w        io.Writer
	name     string
	total    int64
	written  int64
	last     time.Time
	finished bool
}

func newProgress(w io.Writer, f *File) *progress {
	return &progress{w: w, name: f.Filename, total: f.Size}
}

func (p *progress) Write(b []byte) (int, error) {
	n := len(b)
	p.written += int64(n)
	now := time.Now()
	if now.Sub(p.last) >= 200*time.Millisecond || p.written >= p.total {
		p.last = now
		p.draw()
	}
	return n, nil
}

func (p *progress) draw() {
	if p.total <= 0 {
		return
	}
	pct := float64(p.written) / float64(p.total) * 100
	fmt.Fprintf(p.w, "\r  downloading %s  %5.1f%%  %s / %s",
		p.name, pct, humanBytes(p.written), humanBytes(p.total))
}

func (p *progress) done() {
	if p.finished {
		return
	}
	p.finished = true
	if p.total > 0 {
		fmt.Fprintf(p.w, "\r  downloaded %s  %s%s\n", p.name, humanBytes(p.written), strings.Repeat(" ", 20))
	}
}

// humanBytes renders a byte count in a compact form.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
