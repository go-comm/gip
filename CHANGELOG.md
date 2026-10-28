# Changelog

The notable changes of each release of gip, newest first. The version numbers
follow [semantic versioning](https://semver.org/).

## v0.6.0 - 2026-10-28

### Added

* A `bin` directory in the gip home holds a versioned name for every command of
  every installed release: `go1.20.14` and `gofmt1.20.14` for the commands of
  the release, `gopls-go1.20.14` for the tools of its `GOBIN`. What a name runs
  is gip itself, which reads the name it was started under and runs that command
  of that release with its `GOROOT`, `GOBIN` and `GOCACHE` pinned, so the name
  works whatever the current release is and whatever the working directory
  asks for. Putting one directory on the `PATH` therefore reaches every release.
* The names are created when a release is installed, removed when a release is
  uninstalled and refreshed when `go install` or `go get` leaves a tool in the
  `GOBIN` of a release. A name is a link to the gip binary, or a copy of it on
  a file system that cannot link the two directories.

## v0.5.0 - 2026-10-27

### Changed

* gip builds with Go 1.13 and later, down from Go 1.19. The calls that `os`,
  `io`, `bytes` and `runtime/debug` grew in Go 1.16 and 1.18 now sit behind
  `internal/compat`, which spells them with `io/ioutil`, a hand-written `Cut`
  and no build information when the toolchain is older, and the tests bring
  their own `t.TempDir`, `t.Cleanup` and `t.Setenv`.
* The version of a binary is read from the build information, which the
  runtime only keeps since Go 1.18: a binary built with an older toolchain
  reports the version of the source tree, and `-ldflags "-X
  main.versionOverride=v1.2.3"` still overrides it.

## v0.4.0 - 2026-10-22

### Added

* `gip install <version>` downloads and installs a release. An argument that is
  not a version still goes to `go install`, so the command keeps that meaning
  as well: `gip install 1.22.0` installs a release, `gip install
  golang.org/x/tools/cmd/stringer@latest` installs a package.
* `latest` is accepted by `gip install` and by `--go`, so `gip install latest`
  installs the newest stable release and `gip --go latest run .` runs with it.
  A whole series keeps its meaning as well: `gip install 1.22` installs the
  newest 1.22.x.
* The arguments of `gip version` are handed to `go version`, so the module
  information of a binary stays reachable with `gip version -m ./bin`.

### Changed

* `gip version` prints the version of gip and then runs `go version`; `gip env`
  prints the paths gip manages and then runs `go env`. Both halves of `gip env`
  describe the same release, and no release is downloaded to answer a question
  about it. This replaces the split of v0.3.0, where gip's own data could only
  be seen with `gip --version` and `gip toolchain env`; those two still print
  it alone.
* `gip version` says on stderr why the version of the release is missing
  instead of leaving that line out without a word, and `gip env` fails with the
  same explanation.
* The paths of `gip env` and `gip toolchain env` are aligned in one column, and
  the usage text and the README files describe the commands that gip shares
  with the go command.

### Fixed

* The PATH of the caller was lost on Windows, where the variable is commonly
  spelled `Path`. The go command then ran without it and could not start git, a
  C compiler or any other tool, and `gip exec git` reported that git was not in
  the PATH. The names of environment variables are compared case-insensitively
  there now.
* `gip install latest` failed with "cannot find the path specified" on a fresh
  install, because the release was looked up before the directory of the store
  had been created.

## v0.3.0 - 2026-10-22

### Added

* The README describes how the version of a binary is determined.

### Changed

* `gip version` and `gip env` became the plain `go version` and `go env` of the
  selected release, and gip's own version and paths moved to `gip --version`
  and `gip toolchain env`. (v0.4.0 lets the two commands print both.)
* The version of gip is derived from the build information: the tag of the
  commit, a pseudo-version when the commit is after the latest tag, or the
  version of the source tree when the build carries nothing, as is the case
  for `go run`.

## v0.2.0 - 2026-10-22

First release.

### Added

* `gip` and `gip auto` align the toolchain with the `go` and `toolchain`
  directives of the enclosing go.mod or go.work file, and download the release
  when it is missing. The `toolchain` directive is followed exactly, the `go`
  directive is a minimum, as in the go command.
* Every other go command is forwarded, so `gip test ./...` runs `go test
  ./...` with the toolchain the module asks for. `GOTOOLCHAIN` is set to
  `local`, so the go command does not switch toolchains behind gip's back.
* Releases are installed side by side under `~/.gip/versions`, each with its
  own GOBIN and GOCACHE. The download is verified against the SHA-256 checksum
  of the release manifest, and `GIP_MIRROR` selects a mirror.
* The flags `-go`/`--go`, `-no-download` and `-mirror`, written with one or
  with two dashes and recognised anywhere on the command line. The flags of the
  go command are left alone.
* `gip toolchain install|use|ls|current|uninstall|env`, and the short forms
  `gip use`, `gip ls`, `gip current` and `gip uninstall`.
* `gip exec` runs a command with the environment of the selected release, for
  third party tools such as gopls and golangci-lint.
* `gip doctor` checks GOROOT, GOTOOLCHAIN, the PATH and the links of the
  `current` directory.
* `~/.gip/current/goroot` is a stable link to the GOROOT of the current
  release, for tools that need a GOROOT, such as the `go.goroot` setting of the
  VS Code Go extension. Switching the release switches the link in one step,
  and uninstalling the current release removes it instead of leaving it
  dangling.
