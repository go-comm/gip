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
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/go-comm/gip/internal/downloader"
	"github.com/go-comm/gip/internal/exec"
	"github.com/go-comm/gip/internal/installer"
	"github.com/go-comm/gip/internal/resolver"
	"github.com/go-comm/gip/internal/store"
	"github.com/go-comm/gip/pkg/version"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, errUsage) {
			os.Exit(2)
		}
		if errors.Is(err, errProblems) {
			// The doctor has already explained the problems.
			os.Exit(1)
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

	case "exec":
		return cmdExec(global, rest)

	case "doctor":
		return cmdDoctor(global, rest)

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

	case "--version":
		// The version of gip itself. It is a flag and not a command, so
		// that the "version" command stays the one of the go command.
		fmt.Println("gip version", buildVersion())
		return nil

	case "install":
		// "install" belongs to both worlds: a version installs a toolchain,
		// anything else is a package for "go install". Every argument that
		// "go install" accepts behaves the same way, so nothing of the go
		// command is shadowed.
		if len(rest) == 1 && looksLikeToolchainVersion(rest[0]) {
			return cmdToolchainInstall(global, rest)
		}
		return cmdGo(global, append([]string{"install"}, rest...))

	case "version":
		// Without arguments the version of gip, followed by the version of
		// the release that would be used. With arguments the go command
		// answers, so "gip version -m ./bin" stays "go version -m ./bin".
		if len(rest) > 0 {
			return cmdGo(global, append([]string{"version"}, rest...))
		}
		return cmdVersion(global, rest)

	case "env":
		// The paths gip manages, followed by the environment of the
		// selected release. Reporting never downloads a release.
		if len(rest) == 0 {
			return cmdEnv(global)
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
the module asks for. The three commands gip shares with the go command only
print or do something in addition and then run the go command itself, so none
of its behaviour is lost.

Commands of gip:
  auto                     Align the toolchain with the current directory
  exec <command> [args...] Run a command with the toolchain of the release
  doctor                   Check the environment for problems
  toolchain install <version>   Download and install a Go release
  toolchain use <version>       Make a release the current one
  toolchain ls                  List the installed releases
  toolchain current             Print the current release
  toolchain uninstall <version> Remove an installed release
  toolchain env                 Print the paths gip manages

The toolchain commands are also available without the namespace, as long as
they do not clash with a go command:

  use <version>       List: gip toolchain use
  ls                  List: gip toolchain ls
  current             List: gip toolchain current
  uninstall <version> List: gip toolchain uninstall

Commands shared with the go command:
  version            The version of gip, then "go version"
  env                The paths gip manages, then "go env"
  install <version>  Download and install a Go release; every other argument
                     is a package for "go install"

Both of them hand their arguments over unchanged when they are given any, so
"gip version -m ./bin" is "go version -m ./bin" and "gip env GOROOT" answers
with the value of the variable alone.

Flags of gip:
  -go, --go <version>  Use this release instead of the one the module asks for
  -no-download         Fail instead of downloading a missing release
  -mirror <url>        Download site to use (default `+downloader.DefaultMirror+`)
  --version            Print the version of gip itself

A flag may be written with one or with two dashes, so "gip -go=1.20 run ."
and "gip --go 1.20 run ." mean the same.

"gip ls --all" also lists the releases that can be installed.

The version of a release is given without the "go" prefix, for example
1.21.0. A version such as 1.21 selects the newest release of that series, and
"latest" the newest stable release.
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

// isLatest reports whether a request asks for the newest stable release
// rather than for a particular version.
func isLatest(s string) bool {
	return strings.EqualFold(strings.TrimSpace(s), "latest")
}

// looksLikeToolchainVersion reports whether s names a Go release rather than
// a package.
func looksLikeToolchainVersion(s string) bool {
	s = strings.TrimSpace(s)
	if isLatest(s) {
		return true
	}
	s = version.TrimGo(s)
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
	// 1.19 stands for the whole series, and "latest" is the newest stable
	// release.
	norm := version.TrimGo(strings.TrimSpace(args[0]))
	if !isLatest(norm) && !version.IsValid(norm) {
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

// cmdEnv prints the paths gip manages and then the environment of the release
// the go command would run with.
//
// Both halves describe the same release, so the GOROOT gip reports and the
// GOROOT the go command reports cannot disagree. Reporting never downloads.
func cmdEnv(global options) error {
	st, err := openStore()
	if err != nil {
		return err
	}
	release, resolveErr := resolveRelease(st, global, false)
	if release == "" {
		// Nothing is selected, so the store itself is described.
		release, _ = st.Current()
	}
	if err := printToolchainEnv(st, release, mirrorOf(global)); err != nil {
		return err
	}
	if resolveErr != nil {
		return resolveErr
	}
	fmt.Println()
	return cmdGoNoDownload(global, []string{"env"})
}

// cmdToolchainEnv prints the paths gip manages.
func cmdToolchainEnv(global options, args []string) error {
	st, err := openStore()
	if err != nil {
		return err
	}
	cur, _ := st.Current()
	return printToolchainEnv(st, cur, mirrorOf(global))
}

// printToolchainEnv prints the paths gip manages for a release. An empty
// release describes the store without a selection.
func printToolchainEnv(st *store.Store, release, mirror string) error {
	// A tab writer keeps the values in one column whatever the length of
	// the names is.
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "GIP_HOME\t%s\n", st.Root)
	fmt.Fprintf(w, "CURRENT\t%s\n", release)
	if release != "" {
		fmt.Fprintf(w, "GOROOT\t%s\n", st.VersionDir(release))
		fmt.Fprintf(w, "GOBIN\t%s\n", st.GoBinDir(release))
		fmt.Fprintf(w, "GOCACHE\t%s\n", st.GoCacheDir(release))
	}
	fmt.Fprintf(w, "GOROOT_LINK\t%s\n", st.GorootLink())
	fmt.Fprintf(w, "GO_BIN\t%s\n", st.GorootBinDir())
	fmt.Fprintf(w, "MIRROR\t%s\n", mirror)
	return w.Flush()
}

// cmdVersion prints the version of gip and the version of the release that
// would be used.
func cmdVersion(global options, args []string) error {
	fmt.Println("gip version", buildVersion())

	st, err := openStore()
	if err != nil {
		return nil
	}
	// Reporting never downloads, so a release that is missing is reported on
	// stderr and gip stays quiet on stdout.
	release, err := resolveRelease(st, global, false)
	if err != nil || release == "" {
		if err != nil {
			fmt.Fprintln(os.Stderr, "gip: "+err.Error())
		}
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

// cmdExec runs a command with the environment of the selected release.
//
// Third party tools that drive the go command themselves find the go command
// of the release in the PATH and work with the GOROOT, GOBIN and GOCACHE
// that belong to it.
func cmdExec(global options, args []string) error {
	if len(args) == 0 {
		usage()
		return fmt.Errorf("exec needs a command to run")
	}
	st, err := openStore()
	if err != nil {
		return err
	}
	release, err := resolveRelease(st, global, true)
	if err != nil {
		return err
	}
	runner := newRunner(st, release)
	return runner.RunCommand(args[0], args[1:], os.Stdin, os.Stdout, os.Stderr)
}

// cmdDoctor reports how well the environment is set up for gip.
//
// It looks for the things that keep a third party tool from using the
// toolchain that gip selected.
func cmdDoctor(global options, args []string) error {
	st, err := openStore()
	if err != nil {
		return err
	}
	cur, _ := st.Current()
	fmt.Printf("gip version %s\n", buildVersion())
	fmt.Printf("GIP_HOME   %s\n", st.Root)
	fmt.Printf("CURRENT    %s\n", cur)

	var problems int

	// A GOROOT in the environment overrides the toolchain of the go command
	// that is found, so the two can get out of sync.
	if root := os.Getenv("GOROOT"); root != "" {
		want := ""
		if cur != "" {
			want = st.VersionDir(cur)
		}
		if want != "" && !sameDir(root, want) {
			problems++
			fmt.Printf("\n! GOROOT is set to %s in the environment.\n", root)
			fmt.Printf("  It overrides the toolchain of every go command, so a tool\n")
			fmt.Printf("  would build against %s instead of %s.\n", root, want)
			fmt.Printf("  Remove it: setx GOROOT \"\"\n")
		} else {
			fmt.Printf("\n  GOROOT is set to %s\n", root)
		}
	}

	// A toolchain switch of the go command would take the version selection
	// away from gip.
	if tc := os.Getenv("GOTOOLCHAIN"); tc != "" && tc != "local" {
		problems++
		fmt.Printf("\n! GOTOOLCHAIN is set to %q in the environment.\n", tc)
		fmt.Printf("  The go command would switch to another toolchain on its own.\n")
		fmt.Printf("  Use GOTOOLCHAIN=local to keep gip in charge.\n")
	}

	// The bin directory of the current release has to come first in the PATH
	// for a tool that calls go to find the right one.
	currentBin := st.GorootBinDir()
	if !onPath(currentBin) {
		problems++
		fmt.Printf("\n! %s is not in the PATH.\n", currentBin)
		fmt.Printf("  A tool that calls go would find another one.\n")
		fmt.Printf("  Add it in front of the PATH:\n")
		fmt.Printf("    setx PATH \"%s;%%PATH%%\"\n", currentBin)
	}

	if cur == "" {
		problems++
		fmt.Printf("\n! No release is selected.\n")
		fmt.Printf("  Run \"gip\" in a project to select the release it asks for.\n")
	} else {
		// The GOROOT link has to point at the current release: it is what
		// tools like the VS Code Go extension are pointed at.
		v, ok := st.GorootVersion()
		switch {
		case !ok:
			problems++
			fmt.Printf("\n! The GOROOT link %s is missing.\n", st.GorootLink())
			fmt.Printf("  Run \"gip toolchain use %s\" to create it.\n", cur)
		case v != cur:
			problems++
			fmt.Printf("\n! The GOROOT link %s points at go%s instead of go%s.\n", st.GorootLink(), v, cur)
			fmt.Printf("  Run \"gip toolchain use %s\" to fix it.\n", cur)
		}
	}

	if problems == 0 {
		fmt.Printf("\nEverything is set up for gip.\n")
		return nil
	}
	fmt.Printf("\n%d problem(s) found.\n", problems)
	return errProblems
}

// errProblems signals that the doctor found problems in the environment.
var errProblems = errors.New("problems found")

// sameDir reports whether two paths point at the same directory.
func sameDir(a, b string) bool {
	ua, err := filepath.Abs(a)
	if err != nil {
		return false
	}
	ub, err := filepath.Abs(b)
	if err != nil {
		return false
	}
	return strings.EqualFold(filepath.Clean(ua), filepath.Clean(ub))
}

// onPath reports whether a directory is part of the PATH.
func onPath(dir string) bool {
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if p == "" {
			continue
		}
		if sameDir(p, dir) {
			return true
		}
	}
	return false
}

// cmdGo hands the arguments over to the go command of the selected release.
func cmdGo(global options, args []string) error {
	return runGo(global, args, true)
}

// cmdGoNoDownload is cmdGo for the commands that only report: it never
// downloads a release, so that asking for information stays cheap.
func cmdGoNoDownload(global options, args []string) error {
	return runGo(global, args, false)
}

// runGo runs the go command of the selected release.
func runGo(global options, args []string, allowDownload bool) error {
	st, err := openStore()
	if err != nil {
		return err
	}
	release, err := resolveRelease(st, global, allowDownload)
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
	// The --no-download flag turns every download into an error.
	allowDownload = allowDownload && !opts.noDownload

	if opts.version != "" {
		norm := version.TrimGo(strings.TrimSpace(opts.version))
		if isLatest(norm) {
			return installLatest(st, opts, allowDownload)
		}
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

// installLatest turns the request "latest" into a release, downloading the
// newest stable one when it is missing.
//
// The newest release can only be found in the manifest of the download site,
// so the request cannot be answered without the network.
func installLatest(st *store.Store, opts options, allowDownload bool) (string, error) {
	if !allowDownload {
		// The newest release is only known to the manifest, which needs the
		// network. The newest installed release is the best answer that can
		// be given without it.
		if v := latestInstalled(st); v != "" {
			return v, nil
		}
		return "", errors.New(`"latest" can only be resolved by downloading a release`)
	}
	in := &installer.Installer{Store: st, Mirror: mirrorOf(opts), Out: os.Stderr}
	return in.Install(context.Background(), "latest", false)
}

// resolveRequirement turns a requirement of a module into a release,
// downloading it when it is missing.
func resolveRequirement(st *store.Store, req resolver.Requirement, opts options, allowDownload bool) (string, error) {
	allowDownload = allowDownload && !opts.noDownload

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
		if isLatest(norm) {
			// A requirement has to name a release, so "latest" is resolved
			// to the newest stable one first.
			rel, err := installLatest(st, global, !global.noDownload)
			if err != nil {
				return err
			}
			norm = rel
		}
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
