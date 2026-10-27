//go:build go1.18
// +build go1.18

package compat

// The runtime carries the build information since Go 1.18; buildinfo_legacy.go
// covers the older toolchains, which gip still builds with.

import "runtime/debug"

// BuildVersion returns the version the build information of the binary
// carries, and reports whether the information carried one.
//
// fallback is the version of the source tree. A binary built from a repository
// carries no version of its own, so it reports fallback with the revision of
// the commit appended, and a "+dirty" suffix when the working tree was
// modified.
func BuildVersion(fallback string) (string, bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v, true
	}
	if rev := buildSetting(info, "vcs.revision"); rev != "" {
		v := fallback + "+" + shortRevision(rev)
		if buildSetting(info, "vcs.modified") == "true" {
			v += ".dirty"
		}
		return v, true
	}
	return "", false
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
