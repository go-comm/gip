# gip

English | [简体中文](README_zh-CN.md)

`gip` manages Go toolchains. It installs Go releases side by side and runs
the `go` command of the release a module asks for.

Unlike the `GOTOOLCHAIN` mechanism of Go 1.21 and later, `gip` does not need
a recent Go toolchain to be installed already: it downloads the release it
needs itself. That makes it usable on machines whose system Go is older than
1.21, and it keeps the toolchain selection under the control of the user.

## Install

```console
$ go install github.com/go-comm/gip/cmd/gip@latest
```

Right after a release the checksum database may not know the new version
yet, and the install fails with `invalid version: unknown revision`. The
release itself is fine, so installing it directly works:

```console
$ GONOSUMDB=github.com/go-comm/* go install github.com/go-comm/gip/cmd/gip@v0.2.0
```

## Usage

`gip` is a drop-in replacement for `go`: every command that gip does not own
is handed over to the `go` command of the selected release.

```console
$ gip install 1.22.0        # download and install a release
$ gip use 1.22.0            # make it the current one
$ gip ls                    # list the installed releases
$ gip test ./...            # go test ./... with the right toolchain
$ gip build -o bin/app ./cmd/...
$ gip mod tidy
```

### Commands of gip

gip owns the management of the toolchains:

| Command                             | Meaning                              |
| ----------------------------------- | ------------------------------------ |
| `gip`                               | align the toolchain with the project |
| `gip auto`                          | the same, written out                |
| `gip exec <command> [args...]`      | run a command with the toolchain     |
| `gip doctor`                        | check the environment for problems   |
| `gip toolchain install <version>`   | download and install a release       |
| `gip toolchain use <version>`       | make a release the current one       |
| `gip toolchain ls`                  | list the installed releases          |
| `gip toolchain current`             | print the current release            |
| `gip toolchain uninstall <version>` | remove an installed release          |
| `gip toolchain env`                 | print the paths gip manages          |

The commands that do not clash with a go command are also available without
the namespace: `gip use`, `gip ls`, `gip current` and `gip uninstall`.

`gip ls --all` also lists the releases that can be installed. The version of gip
itself is printed with `gip --version`.

A release is installed with `gip toolchain install <version>`:

```console
$ gip toolchain install 1.22.0
```

No command of gip has the name of a go command. `gip version` is `go version`
and `gip env` is `go env`, both of the selected release, and `gip install` is
`go install`. The version of gip itself is printed with `gip --version`, the
paths gip manages with `gip toolchain env`.

Everything else belongs to the go command and is passed on unchanged:

```console
$ gip test ./...
$ gip vet ./...
$ gip fmt ./...
$ gip mod tidy
$ gip get ./...
$ gip run ./cmd/app
$ gip doc fmt
$ gip help build
```

The flags of gip are recognised anywhere on the command line, the flags of
the go command are left alone and reach it unchanged.

A flag may be written with one or with two dashes, the way the flag package
of the go command allows it:

```console
$ gip -go=1.20 run ./cmd/app
$ gip --go=1.20 run ./cmd/app
$ gip -go 1.20 run ./cmd/app
```

Note that Windows PowerShell 5.1 splits an argument that contains an equals
sign, so `-go=1.20` has to be quoted there:

```console
> gip "-go=1.20" run ./cmd/app
```

PowerShell 7 and the other shells pass it through unchanged.

### Aligning with a project

Without a command, or with `gip auto`, gip looks at the `go.mod` or `go.work`
file of the current directory and aligns the toolchain with it:

```console
$ cd ~/code/myproject
$ gip
switched to go1.21.13
/home/user/code/myproject/go.mod asks for go1.21.0
```

The release the module asks for is downloaded when it is missing, the
directories that belong to it are prepared and the current release is
switched over to it. Since the `current` links are updated, a `go` on the
PATH that points at `~/.gip/current/goroot/bin` is the right one
afterwards.

Moving to a project that asks for another version and running `gip` again
switches over, in both directions.

### Selecting a release

The release is chosen in this order:

1. An explicit `--go <version>` flag wins over everything else and selects
   exactly that release.
2. The `go.work` file of the enclosing directory takes precedence over the
   `go.mod` file.
3. Inside one file the `toolchain` directive takes precedence over the `go`
   directive, unless it names an older release.
4. The `toolchain` directive names one exact release, while the `go`
   directive states a *minimum*: any release of the same series that is not
   older satisfies it.
5. A `toolchain default` directive means "use the default release", so the
   `go` directive is ignored and no download is triggered.

The current release takes no part in the choice: a module that asks for an
older toolchain gets one, even when a newer release is selected. `gip use`
only decides what happens when a module does not ask for anything.

If the selected release is not installed, `gip` downloads it. Use
`--no-download` to fail instead, which is useful in CI.

When there is no `go.mod` or `go.work` file at all, and no release has been
selected with `gip use`, the newest installed release is used.

### Versions

Versions are written without the `go` prefix, for example `1.22.0`.

* A full version such as `1.22.0` selects exactly that release.
* A version without a patch number such as `1.22` selects the newest release
  of that series, so `gip install 1.22` installs the latest 1.22.x.

The rules that decide which release satisfies a requirement follow the Go
standard library:

```
1.21 < 1.21rc1 < 1.21.0
```

The language version `1.21` sorts *below* both the release candidate and the
release, and before Go 1.21 a missing patch number meant `0`, so `1.20` and
`1.20.0` are the same version.

## Directory layout

Everything lives under `~/.gip`, or `$GIP_HOME` if it is set:

```
~/.gip/
├── versions/
│   ├── 1.19.10/          <- GOROOT of that release
│   └── 1.22.0/
├── gobin/
│   ├── 1.19.10/          <- GOBIN of that release
│   └── 1.22.0/
├── gocache/
│   ├── 1.19.10/          <- GOCACHE of that release
│   └── 1.22.0/
└── current/
    ├── gobin  -> ../gobin/1.22.0
    └── goroot -> ../versions/1.22.0
```

`GOROOT` always points at `versions/<version>`, never at the `current`
directory itself, which holds links and no `src`, `pkg` and `lib`
directories. The `goroot` link of that directory is the exception: it
points at a whole GOROOT and is meant for tools that need one. The links
exist as a convenience for people who put `~/.gip/current/goroot/bin` on
their `PATH`.

Every go command that gip runs has the `GOROOT`, `GOBIN` and `GOCACHE` of
the selected release pinned, and the bin directories of the release are put
in front of `PATH`. The build cache is kept per release because sharing it
across toolchain versions can lead to subtle differences in the produced
artefacts.

Both links of `current` are switched together. They are staged under
temporary names and renamed into place, and a failure of a rename restores
the previous selection, so it stays intact.

## Third party tools

Linters, generators and IDE plugins drive the go command themselves: they
look for `go` in the PATH and work with the `GOROOT` it reports. There are
two ways to let them cooperate with gip.

### gip exec

`gip exec` runs a command with the environment of the selected release:

```console
$ gip exec golangci-lint run ./...
$ gip exec gopls check ./...
```

The command finds the go command of the release in the PATH and works with
the `GOROOT`, `GOBIN` and `GOCACHE` that belong to it. `GOTOOLCHAIN` is set
to `local`, so the go command does not switch to another toolchain behind
your back.

### The current release in the PATH

A tool that is started without `gip exec` finds the go command in the PATH.
Put the bin directory of the current release in front of it:

```
~/.gip/current/goroot/bin
```

Running `gip` in a project switches the current release, so a tool that is
started afterwards works with the version the project asks for.

### The GOROOT link

A tool that needs a GOROOT, such as the VS Code Go extension, cannot use
the `current` directory itself: it holds the `gobin` and `goroot` links, and
none of them is a directory with the `src`, `pkg` and `lib` subdirectories a
GOROOT consists of.

The `goroot` link of the current directory points at a whole GOROOT of the
current release:

```
~/.gip/current/goroot -> ../versions/1.22.0
```

Its path does not change when the release changes, so it can be used in a
configuration once and for all. It is switched together with the current
release.

```console
$ gip toolchain env
GIP_HOME   /home/user/.gip
CURRENT    1.22.0
GOROOT     /home/user/.gip/versions/1.22.0
GOBIN      /home/user/.gip/gobin/1.22.0
GOCACHE    /home/user/.gip/gocache/1.22.0
GOROOT_LINK /home/user/.gip/current/goroot
GO_BIN      /home/user/.gip/current/goroot/bin
MIRROR     https://go.dev/dl
```

### The GOROOT in the environment

A `GOROOT` in the environment overrides the toolchain of every go command,
no matter where that command comes from. A tool would then build against the
standard library of that other Go installation.

The same is true for `GOTOOLCHAIN`: a value other than `local` lets the go
command pick another toolchain on its own.

`gip doctor` reports both problems and shows how to fix them. It exits with
a status of 1 when it finds any, which makes it usable in a script:

```console
$ gip doctor
gip version v0.2.0
GIP_HOME   /home/user/.gip
CURRENT    1.22.0

! GOROOT is set to /usr/local/go in the environment.
  It overrides the toolchain of every go command, so a tool
  would build against /usr/local/go instead of /home/user/.gip/versions/1.22.0.
  Remove it: unset GOROOT

! /home/user/.gip/current/goroot/bin is not in the PATH.
  A tool that calls go would find another one.
  Add it in front of the PATH:
    export PATH="/home/user/.gip/current/goroot/bin:$PATH"

2 problem(s) found.
```

Tools that are installed with `gip install` end up in the GOBIN of the
release, `~/.gip/gobin/<version>`. That directory is part of the PATH that
`gip exec` builds, so such a tool is found right away.

## Environment variables

| Variable    | Meaning                                                      |
| ----------- | ------------------------------------------------------------ |
| `GIP_HOME`  | Directory holding the installs, `~/.gip` by default          |
| `GIP_MIRROR` | Download site, `https://go.dev/dl` by default               |
| `GOWORK`    | Honoured while looking for the `go.work` file                |

A mirror has to serve the same manifest format, for example
`https://golang.google.cn/dl`:

```console
$ GIP_MIRROR=https://golang.google.cn/dl gip install 1.22.0
```

## Downloads

Releases are taken from the official manifest at
`https://go.dev/dl/?mode=json&include=all`, which carries the SHA-256
checksum of every archive. The archive is downloaded to a temporary file,
verified against that checksum and only then unpacked, so an interrupted
download never leaves a broken installation behind. The unpacking happens in
a temporary directory that is renamed into place afterwards.

Two terminals asking for the same missing release at the same time are
serialized with a lock file, so the release is downloaded only once.

## Platform notes

On Unix the archives are `.tar.gz` files, on Windows they are `.zip` files;
the downloader picks the right one from the manifest.

Creating a symbolic link on Windows needs a privilege that is not granted by
default, so `gip` falls back to a directory junction there. A junction
stores an absolute path, which is the only difference from the layout shown
above.

## Development

```console
$ go test ./...
```

The project has no dependencies outside the standard library.

### Version

`gip --version` reports the version from the build information:

* A binary installed with `go install github.com/go-comm/gip/cmd/gip@v0.2.0`
  reports `v0.2.0`.
* A binary built from a repository reports the version of the commit it
  was built from: the tag when the commit carries one, and a pseudo-version
  such as `v0.2.1-0.20261022133637-62a53e2482f2` when the commit is after the
  latest tag. A modified working tree adds a `+dirty` suffix.
* `go run` carries no version information at all and reports the version of
  the source tree, `v0.2.0`.

The version can be set at build time with

```console
$ go build -ldflags "-X main.versionOverride=v1.2.3" ./cmd/gip
```
