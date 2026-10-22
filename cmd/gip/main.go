// Command gip manages Go toolchains.
//
// gip installs Go releases side by side and runs the go command of the
// release a module asks for, without relying on the GOTOOLCHAIN mechanism of
// Go 1.21 and later. It therefore also works on machines whose system Go is
// older than 1.21.
//
// Every command that gip does not own is handed over to the go command of
// the selected release, so "gip test ./..." behaves like "go test ./..."
// except that it runs with the right toolchain.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"strings"

	"github.com/go-comm/gip/internal/downloader"
	"github.com/go-comm/gip/internal/exec"
	"github.com/go-comm/gip/internal/installer"
	"github.com/go-comm/gip/internal/resolver"
	"github.com/go-comm/gip/internal/store"
	"github.com/go-comm/gip/pkg/version"
)

// version of gip itself.
const gipVersion = "0.1.0"

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, errUsage) {
			os.Exit(2)
		}
		var exitErr *osexec.ExitError
		if errors.As(err, &exitErr) {
			// The go command has already reported the failure itself.
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintln(os.Stderr, "gip: "+err.Error())
		os.Exit(exec.ExitCode(err))
	}
}

// errUsage signals that the usage message was already printed.
var errUsage = errors.New("usage")

// run dispatches a command line.
//
// The commands gip owns are handled here, everything else is passed on to the
// go command.
func run(args []string) error {
	global, rest := splitGlobalFlags(args)
	if len(rest) > 0 && rest[0] == "--" {
		// The separator only ends the flags of gip itself; the command
		// behind it is still executed.
		rest = rest[1:]
	}
	if len(rest) == 0 {
		// Without a command gip aligns the toolchain with the module in
		// the working directory.
		return cmdAuto(global, nil)
	}

	cmd := rest[0]
	rest = rest[1:]

	switch cmd {
	case "auto":
		return cmdAuto(global, rest)

	case "toolchain", "tc":
		return cmdToolchain(global, rest)

	// The toolchain commands are also reachable without the namespace.
	case "use":
		return cmdToolchainUse(global, rest)
	case "ls":
		return cmdToolchainLs(global, rest)
	case "current":
		return cmdToolchainCurrent(global, rest)
	case "uninstall", "remove":
		return cmdToolchainUninstall(global, rest)

	case "install":
		// "install" belongs to both worlds: a version installs a toolchain,
		// anything else is a package for "go install".
		if len(rest) == 1 && looksLikeToolchainVersion(rest[0]) {
			return cmdToolchainInstall(global, rest)
		}
		return cmdGo(global, append([]string{"install"}, rest...))

	case "version", "--version":
		return cmdVersion(global, rest)

	case "env":
		// Without arguments gip reports its own paths, with arguments the
		// query is answered by the go command.
		if len(rest) == 0 {
			return cmdToolchainEnv(global, rest)
		}
		return cmdGo(global, append([]string{"env"}, rest...))

	case "help", "-h", "--help":
		if len(rest) > 0 {
			return cmdGo(global, append([]string{"help"}, rest...))
		}
		usage()
		return nil

	default:
		// Everything else belongs to the go command.
		return cmdGo(global, append([]string{cmd}, rest...))
	}
}

// usage prints the help text.
func usage() {
	fmt.Fprint(os.Stderr, `gip manages Go toolchains.

Usage:
  gip [command] [arguments]

Without a command gip aligns the toolchain with the version the go.mod or
go.work file of the current directory asks for: the release is downloaded
when it is missing and the current release is switched over to it.

Every command that gip does not own is handed over to the go command of the
selected release, so "gip test ./..." runs "go test ./..." with the toolchain
the module asks for.

Commands of gip:
  auto                     Align the toolchain with the current directory
  toolchain install <version>   Download and install a Go release
  toolchain use <version>       Make a release the current one
  toolchain ls                  List the installed releases
  toolchain current             Print the current release
  toolchain uninstall <version> Remove an installed release
  toolchain env                 Print the paths gip manages

The same commands are also available without the namespace, as long as they
do not clash with a go command:

  use <version>       List: gip toolchain use
  ls                  List: gip toolchain ls
  current             List: gip toolchain current
  uninstall <version> List: gip toolchain uninstall
  install <version>   List: gip toolchain install

"gip install" installs a toolchain when it is given a version and runs
"go install" otherwise, so "gip install golang.org/x/tools/cmd/stringer@latest"
installs a package while "gip install 1.22.0" installs a release.

Flags of gip:
  -go, --go <version>  Use this release instead of the one the module asks for
  -no-download         Fail instead of downloading a missing release
  -mirror <url>        Download site to use (default `+downloader.DefaultMirror+`)

A flag may be written with one or with two dashes, so "gip -go=1.20 run ."
and "gip --go 1.20 run ." mean the same.

"gip ls --all" also lists the releases that can be installed.

The version of a release is given without the "go" prefix, for example
1.21.0. A version such as 1.21 selects the newest release of that series.
`)
}

// options are the flags understood by every command.
type options struct {
	version    string
	noDownload bool
	mirror     string
}

// splitGlobalFlags extracts the flags of gip from a list of arguments and
// returns the remaining arguments.
//
// Flags are recognised anywhere in the argument list, except after a "--"
// separator, which is passed through untouched.
//
// Like the flag package of the go command, a flag may be written with one
// or with two dashes, so both "-go=1.20" and "--go=1.20" are understood.
//
// Only flags that no go command knows are taken, so that the flags of the go
// command reach it unchanged.
func splitGlobalFlags(args []string) (options, []string) {
	var opts options
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			rest = append(rest, args[i:]...)
			break
		}
		name, value, hasValue := flagName(a)
		switch name {
		case "go":
			if hasValue {
				opts.version = value
			} else if i+1 < len(args) {
				i++
				opts.version = args[i]
			}
			continue
		case "no-download":
			opts.noDownload = true
			continue
		case "mirror":
			if hasValue {
				opts.mirror = value
			} else if i+1 < len(args) {
				i++
				opts.mirror = args[i]
			}
			continue
		}
		rest = append(rest, a)
	}
	return opts, rest
}

// flagName splits an argument into the name of a flag and its value.
//
// The name of the flag is returned without its dashes. The last return
// value reports whether the argument is a flag at all.
func flagName(arg string) (name, value string, hasValue bool) {
	if len(arg) < 2 || arg[0] != '-' {
		return "", "", false
	}
	body := arg[1:]
	if strings.HasPrefix(body, "-") {
		body = body[1:]
	}
	if body == "" {
		return "", "", false
	}
	if i := strings.IndexByte(body, '='); i >= 0 {
		return body[:i], body[i+1:], true
	}
	return body, "", false
}

// looksLikeToolchainVersion reports whether s names a Go release rather than
// a package.
func looksLikeToolchainVersion(s string) bool {
	s = version.TrimGo(strings.TrimSpace(s))
	if !strings.Contains(s, ".") {
		// A version always carries at least a minor number, so "1" is a
		// package name.
		return false
	}
	return version.IsValid(s)
}

// openStore returns the store to work on.
func openStore() (*store.Store, error) {
	return store.Default()
}

// cmdToolchain dispatches the commands of the toolchain namespace.
func cmdToolchain(global options, args []string) error {
	if len(args) == 0 {
		return cmdToolchainLs(global, nil)
	}
	sub := args[0]
	rest := args[1:]
	switch sub {
	case "install", "i":
		return cmdToolchainInstall(global, rest)
	case "use", "u":
		return cmdToolchainUse(global, rest)
	case "ls", "list":
		return cmdToolchainLs(global, rest)
	case "current":
		return cmdToolchainCurrent(global, rest)
	case "uninstall", "remove":
		return cmdToolchainUninstall(global, rest)
	case "env":
		return cmdToolchainEnv(global, rest)
	case "version":
		return cmdVersion(global, rest)
	default:
		usage()
		return fmt.Errorf("unknown toolchain command %q", sub)
	}
}

// cmdToolchainInstall installs a Go release.
func cmdToolchainInstall(global options, args []string) error {
	if len(args) != 1 {
		usage()
		return fmt.Errorf("install takes exactly one version")
	}
	st, err := openStore()
	if err != nil {
		return err
	}
	in := &installer.Installer{Store: st, Mirror: mirrorOf(global), Out: os.Stderr}

	// A full version such as 1.19.10 names one release, a version such as
	// 1.19 stands for the whole series.
	norm := version.TrimGo(strings.TrimSpace(args[0]))
	if !version.IsValid(norm) {
		return fmt.Errorf("invalid Go version %q", args[0])
	}

	// The installer reports what it did on stderr.
	if _, err := in.Install(context.Background(), norm, version.IsRelease(norm)); err != nil {
		return err
	}
	return nil
}

// cmdToolchainUse switches the current release.
func cmdToolchainUse(global options, args []string) error {
	if len(args) != 1 {
		usage()
		return fmt.Errorf("use takes exactly one version")
	}
	st, err := openStore()
	if err != nil {
		return err
	}
	norm := version.TrimGo(strings.TrimSpace(args[0]))
	if !version.IsValid(norm) {
		return fmt.Errorf("invalid Go version %q", args[0])
	}
	release, ok := st.Resolve(norm)
	if !ok {
		return fmt.Errorf("go %s is not installed; run \"gip toolchain install %s\" first", norm, norm)
	}
	if err := st.Use(release); err != nil {
		return err
	}
	fmt.Printf("now using go%s\n", release)
	return nil
}

// cmdToolchainCurrent prints the current release.
func cmdToolchainCurrent(global options, args []string) error {
	st, err := openStore()
	if err != nil {
		return err
	}
	cur, err := st.Current()
	if err != nil {
		return err
	}
	if cur == "" {
		fmt.Println("no release selected; run \"gip toolchain use <version>\"")
		return nil
	}
	fmt.Println(cur)
	return nil
}

// cmdToolchainLs lists the installed releases.
func cmdToolchainLs(global options, args []string) error {
	showAll := false
	for _, a := range args {
		switch a {
		case "--all", "--remote":
			showAll = true
		default:
			return fmt.Errorf("gip ls: unexpected argument %q", a)
		}
	}

	st, err := openStore()
	if err != nil {
		return err
	}
	installed, err := st.Installed()
	if err != nil {
		return err
	}
	cur, _ := st.Current()

	fmt.Println("INSTALLED")
	if len(installed) == 0 {
		fmt.Println("  (none)")
	}
	for _, v := range installed {
		if v == cur {
			fmt.Printf("* %s   (current)\n", v)
		} else {
			fmt.Printf("  %s\n", v)
		}
	}

	if showAll {
		m, err := downloader.FetchManifest(context.Background(), mirrorOf(global))
		if err != nil {
			return err
		}
		fmt.Println()
		fmt.Println("AVAILABLE")
		for _, rel := range m.Sorted() {
			v := downloader.TrimReleasePrefix(rel.Version)
			if version.Parse(v).Kind != "" {
				continue // skip prereleases
			}
			mark := " "
			if seriesInstalled(installed, v) {
				mark = "*"
			}
			fmt.Printf("%s %s\n", mark, v)
		}
	}
	return nil
}

// cmdToolchainUninstall removes an installed release.
func cmdToolchainUninstall(global options, args []string) error {
	if len(args) != 1 {
		usage()
		return fmt.Errorf("uninstall takes exactly one version")
	}
	st, err := openStore()
	if err != nil {
		return err
	}
	norm := version.TrimGo(strings.TrimSpace(args[0]))
	release, ok := st.Resolve(norm)
	if !ok {
		return fmt.Errorf("go %s is not installed", norm)
	}
	if err := st.Remove(release); err != nil {
		return err
	}
	fmt.Printf("removed go%s\n", release)
	return nil
}

// cmdToolchainEnv prints the paths gip manages.
func cmdToolchainEnv(global options, args []string) error {
	st, err := openStore()
	if err != nil {
		return err
	}
	cur, _ := st.Current()
	fmt.Printf("GIP_HOME  %s\n", st.Root)
	fmt.Printf("CURRENT   %s\n", cur)
	if cur != "" {
		fmt.Printf("GOROOT    %s\n", st.VersionDir(cur))
		fmt.Printf("GOBIN     %s\n", st.GoBinDir(cur))
		fmt.Printf("GOCACHE   %s\n", st.GoCacheDir(cur))
	}
	fmt.Printf("MIRROR    %s\n", mirrorOf(global))
	return nil
}

// cmdVersion prints the version of gip and of the release that would be
// used.
func cmdVersion(global options, args []string) error {
	fmt.Println("gip version", gipVersion)

	st, err := openStore()
	if err != nil {
		return nil
	}
	release, err := resolveRelease(st, global, false)
	if err != nil || release == "" {
		return nil
	}
	runner := newRunner(st, release)
	out, err := runner.Output([]string{"version"})
	if err != nil {
		return nil
	}
	fmt.Print(string(out))
	return nil
}

// cmdGo hands the arguments over to the go command of the selected release.
func cmdGo(global options, args []string) error {
	st, err := openStore()
	if err != nil {
		return err
	}
	release, err := resolveRelease(st, global, true)
	if err != nil {
		return err
	}
	runner := newRunner(st, release)
	return runner.Run(args, os.Stdin, os.Stdout, os.Stderr)
}

// newRunner returns a runner for the go command of a release.
func newRunner(st *store.Store, release string) *exec.Runner {
	return &exec.Runner{
		VersionDir: st.VersionDir(release),
		GoBinDir:   st.GoBinDir(release),
		GoCacheDir: st.GoCacheDir(release),
	}
}

// resolveRelease determines the release a command has to run with.
//
// An explicit --go wins. Otherwise the go.mod or go.work file enclosing the
// working directory decides. When the module does not ask for a particular
// release the current one is used.
func resolveRelease(st *store.Store, opts options, allowDownload bool) (string, error) {
	if opts.version != "" {
		norm := version.TrimGo(strings.TrimSpace(opts.version))
		if !version.IsValid(norm) {
			return "", fmt.Errorf("invalid Go version %q", opts.version)
		}
		if release, ok := st.Resolve(norm); ok {
			return release, nil
		}
		if !allowDownload {
			return "", fmt.Errorf("go %s is not installed", norm)
		}
		in := &installer.Installer{Store: st, Mirror: mirrorOf(opts), Out: os.Stderr}
		return in.Install(context.Background(), norm, version.IsRelease(norm))
	}

	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	spec, err := resolver.Find(wd)
	if err != nil {
		return "", err
	}
	req, err := resolver.Target(spec)
	if err != nil {
		return "", err
	}
	if req.Version == "" {
		// The module does not ask for a particular release, so the current
		// one is used.
		if cur, err := st.Current(); err == nil && cur != "" {
			return cur, nil
		}
		if latest := latestInstalled(st); latest != "" {
			return latest, nil
		}
		return "", fmt.Errorf("cannot determine a Go version: no go.mod or go.work file was found and no release is selected; run \"gip toolchain use <version>\"")
	}
	return resolveRequirement(st, req, opts, allowDownload)
}

// resolveRequirement turns a requirement of a module into a release,
// downloading it when it is missing.
func resolveRequirement(st *store.Store, req resolver.Requirement, opts options, allowDownload bool) (string, error) {
	if req.Exact {
		// The toolchain directive names one exact release.
		if st.IsInstalled(req.Version) {
			return req.Version, nil
		}
		if !allowDownload {
			return "", fmt.Errorf("go %s is not installed", req.Version)
		}
		in := &installer.Installer{Store: st, Mirror: mirrorOf(opts), Out: os.Stderr}
		return in.Install(context.Background(), req.Version, true)
	}

	// The go directive states a minimum: any release of the same series
	// that is not older satisfies it.
	if release, ok := st.ResolveSeries(req.Version); ok {
		return release, nil
	}
	if !allowDownload {
		return "", fmt.Errorf("go %s is not installed", req.Version)
	}
	in := &installer.Installer{Store: st, Mirror: mirrorOf(opts), Out: os.Stderr}
	return in.Install(context.Background(), req.Version, false)
}

// cmdAuto aligns the toolchain with the version the module in the working
// directory asks for.
//
// The release is downloaded when it is missing, the directories that belong
// to it are prepared and the current release is switched over to it.
func cmdAuto(global options, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("auto takes no arguments")
	}
	st, err := openStore()
	if err != nil {
		return err
	}
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	var req resolver.Requirement
	var source string

	if global.version != "" {
		// An explicit version replaces the requirement of the module.
		norm := version.TrimGo(strings.TrimSpace(global.version))
		if !version.IsValid(norm) {
			return fmt.Errorf("invalid Go version %q", global.version)
		}
		req = resolver.Requirement{Version: norm, Exact: version.IsRelease(norm)}
		source = "--go"
	} else {
		spec, err := resolver.Find(wd)
		if err != nil {
			return err
		}
		if spec == nil {
			cur, _ := st.Current()
			if cur == "" {
				return fmt.Errorf("no go.mod or go.work file found in %s or any parent directory", wd)
			}
			fmt.Printf("no go.mod or go.work file found, staying on go%s\n", cur)
			return nil
		}
		req, err = resolver.Target(spec)
		if err != nil {
			return err
		}
		if req.Version == "" {
			cur, _ := st.Current()
			if cur == "" {
				return fmt.Errorf("%s does not ask for a particular release and no release is selected", spec.File)
			}
			fmt.Printf("%s does not ask for a particular release, staying on go%s\n", spec.File, cur)
			return nil
		}
		source = spec.File
	}

	release, err := resolveRequirement(st, req, global, !global.noDownload)
	if err != nil {
		return err
	}

	// Prepare the directories of the release and switch over to it.
	if err := st.EnsureVersionDirs(release); err != nil {
		return err
	}
	cur, _ := st.Current()
	if cur != release {
		if err := st.Use(release); err != nil {
			return err
		}
		fmt.Printf("switched to go%s\n", release)
	} else {
		fmt.Printf("already using go%s\n", release)
	}
	fmt.Printf("%s asks for go%s\n", source, req.Version)
	return nil
}

// seriesInstalled reports whether a release of the same series as v is
// installed.
func seriesInstalled(installed []string, v string) bool {
	for _, r := range installed {
		if version.Lang(r) == version.Lang(v) && version.Compare(r, v) >= 0 {
			return true
		}
	}
	return false
}

// latestInstalled returns the newest installed release.
func latestInstalled(st *store.Store) string {
	installed, err := st.Installed()
	if err != nil || len(installed) == 0 {
		return ""
	}
	return installed[len(installed)-1]
}

// mirrorOf returns the download site to use.
func mirrorOf(opts options) string {
	if opts.mirror != "" {
		return opts.mirror
	}
	return downloader.Mirror()
}
