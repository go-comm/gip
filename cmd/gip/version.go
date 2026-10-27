package main

import "github.com/go-comm/gip/internal/compat"

// defaultVersion is the version of gip when the build information does not
// carry one.
const defaultVersion = "v0.5.0"

// versionOverride can be set at build time with
// -ldflags "-X main.versionOverride=v1.2.3".
var versionOverride = ""

// buildVersion returns the version of gip.
//
// The build information carries the version when gip was installed with
// "go install github.com/go-comm/gip/cmd/gip@v0.5.0", and when it was built
// from a repository: the tag of the commit the binary was built from, or a
// pseudo-version when the commit is after the latest tag. A modified working
// tree adds a "+dirty" suffix.
//
// A build that carries no version information at all, which is the case for
// "go run" and for a toolchain older than Go 1.18, reports the version of the
// source tree.
func buildVersion() string {
	if versionOverride != "" {
		return versionOverride
	}
	if v, ok := compat.BuildVersion(defaultVersion); ok {
		return v
	}
	return defaultVersion
}
