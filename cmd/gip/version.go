package main

import (
	"runtime/debug"
)

// defaultVersion is the version of gip when the build information does not
// carry one.
const defaultVersion = "v0.2.0"

// versionOverride can be set at build time with
// -ldflags "-X main.versionOverride=v1.2.3".
var versionOverride = ""

// buildVersion returns the version of gip.
//
// The build information carries the version when gip was installed with
// "go install github.com/go-comm/gip/cmd/gip@v0.2.0", and when it was built
// from a tagged repository, where the tag becomes the version and a modified
// working tree adds a "+dirty" suffix. A build from a repository without a
// tag still knows its commit.
func buildVersion() string {
	if versionOverride != "" {
		return versionOverride
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return defaultVersion
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	if rev := buildSetting(info, "vcs.revision"); rev != "" {
		return defaultVersion + "+" + shortRevision(rev)
	}
	return defaultVersion
}

// buildSetting returns the value of a setting of the build information.
func buildSetting(info *debug.BuildInfo, key string) string {
	for _, s := range info.Settings {
		if s.Key == key {
			return s.Value
		}
	}
	return ""
}

// shortRevision shortens a commit hash.
func shortRevision(rev string) string {
	if len(rev) > 7 {
		return rev[:7]
	}
	return rev
}
