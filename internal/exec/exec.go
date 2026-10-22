// Package exec runs the go command of an installed release.
//
// The go command is not reimplemented: gip builds a clean environment for the
// go binary of the selected release and hands the arguments over unchanged,
// so that "gip build -o bin/app ./cmd/..." behaves exactly like "go build
// -o bin/app ./cmd/...".
package exec

import (
	"errors"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Runner runs the go command of one release.
type Runner struct {
	// VersionDir is the GOROOT of the release.
	VersionDir string
	// GoBinDir is the GOBIN of the release.
	GoBinDir string
	// GoCacheDir is the GOCACHE of the release.
	GoCacheDir string
}

// GoBinary returns the path of the go command of the release.
func (r *Runner) GoBinary() string {
	name := "go"
	if runtime.GOOS == "windows" {
		name = "go.exe"
	}
	return filepath.Join(r.VersionDir, "bin", name)
}

// Environment returns the environment the go command runs with.
//
// GOROOT, GOBIN and GOCACHE are pinned to the release, and the bin
// directories of the release are put in front of PATH. The build cache is
// kept per release: sharing it across toolchain versions can introduce
// subtle differences in the produced artefacts.
func (r *Runner) Environment(base []string) []string {
	set := map[string]string{
		"GOROOT":  r.VersionDir,
		"GOBIN":   r.GoBinDir,
		"GOCACHE": r.GoCacheDir,
	}
	if os.Getenv("GOTOOLCHAIN") == "" {
		// gip has already selected the toolchain, so the go command should
		// not switch to another one behind the user's back.
		set["GOTOOLCHAIN"] = "local"
	}

	env := make([]string, 0, len(base)+len(set)+1)
	for _, kv := range base {
		i := strings.IndexByte(kv, '=')
		if i < 0 {
			continue
		}
		key := kv[:i]
		if _, ok := set[key]; ok {
			continue
		}
		if key == "PATH" {
			continue
		}
		env = append(env, kv)
	}

	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		env = append(env, k+"="+set[k])
	}

	env = append(env, "PATH="+r.path(base))
	return env
}

// path returns the PATH the go command runs with.
func (r *Runner) path(base []string) string {
	old := ""
	for _, kv := range base {
		if strings.HasPrefix(kv, "PATH=") {
			old = kv[len("PATH="):]
			break
		}
	}
	sep := string(os.PathListSeparator)
	parts := []string{
		r.GoBinDir,
		filepath.Join(r.VersionDir, "bin"),
	}
	if old != "" {
		parts = append(parts, old)
	}
	return strings.Join(parts, sep)
}

// Run runs the go command with the given arguments.
//
// The arguments are passed on unchanged; gip only decides which go command
// runs.
func (r *Runner) Run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	bin := r.GoBinary()
	if _, err := os.Stat(bin); err != nil {
		return fmt.Errorf("cannot run %s: %v", bin, err)
	}
	cmd := osexec.Command(bin, args...)
	cmd.Env = r.Environment(os.Environ())
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

// Output runs the go command and returns its standard output.
func (r *Runner) Output(args []string) ([]byte, error) {
	bin := r.GoBinary()
	if _, err := os.Stat(bin); err != nil {
		return nil, fmt.Errorf("cannot run %s: %v", bin, err)
	}
	cmd := osexec.Command(bin, args...)
	cmd.Env = r.Environment(os.Environ())
	return cmd.Output()
}

// Path returns the PATH the go command runs with.
func (r *Runner) Path() string { return r.path(os.Environ()) }

// RunCommand runs an arbitrary command with the environment of the release.
//
// It is meant for third party tools that drive the go command themselves:
// they find the go command of the release in the PATH and work with the
// GOROOT, GOBIN and GOCACHE that belong to it.
func (r *Runner) RunCommand(name string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	env := r.Environment(os.Environ())
	path, err := lookPathIn(name, r.path(os.Environ()))
	if err != nil {
		return err
	}
	cmd := osexec.Command(path, args...)
	cmd.Env = env
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

// lookPathIn finds an executable in the given PATH.
//
// Unlike exec.LookPath it uses the PATH that is handed to it and not the
// one of the process, and it honours PATHEXT on Windows.
func lookPathIn(file, pathEnv string) (string, error) {
	if strings.ContainsAny(file, `:\/`) {
		if fi, err := os.Stat(file); err == nil && !fi.IsDir() {
			return file, nil
		}
		return "", fmt.Errorf("%s: file does not exist", file)
	}

	exts := []string{""}
	if runtime.GOOS == "windows" {
		exts = nil
		for _, e := range filepath.SplitList(os.Getenv("PATHEXT")) {
			if e != "" {
				exts = append(exts, e)
			}
		}
		if len(exts) == 0 {
			exts = []string{".exe", ".cmd", ".bat"}
		}
	}

	for _, dir := range filepath.SplitList(pathEnv) {
		if dir == "" {
			dir = "."
		}
		for _, ext := range exts {
			path := filepath.Join(dir, file+ext)
			if fi, err := os.Stat(path); err == nil && !fi.IsDir() {
				return path, nil
			}
		}
	}
	return "", fmt.Errorf("executable %q not found in PATH", file)
}

// ExitCode returns the exit status of a command that failed, or 1 if the
// failure was not caused by the command itself.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *osexec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return 1
}
