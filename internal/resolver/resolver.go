// Package resolver determines which Go toolchain version a command should
// use, by reading the go.mod or go.work file that encloses the current
// directory.
//
// The rules implemented here follow the official Go toolchain selection
// algorithm (cmd/go/internal/toolchain/select.go) so that gip and the go
// command always agree on the version that a module asks for.
package resolver

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-comm/gip/internal/compat"
	"github.com/go-comm/gip/pkg/version"
)

// ToolchainDefault is the special toolchain directive value meaning "always
// use the default toolchain".
const ToolchainDefault = "default"

// Spec describes the version directives found in a go.mod or go.work file.
type Spec struct {
	// File is the absolute path of the file the directives were read from.
	File string
	// IsWork reports whether File is a go.work file.
	IsWork bool
	// GoVersion is the raw value of the "go" directive, empty if absent.
	GoVersion string
	// Toolchain is the raw value of the "toolchain" directive, empty if
	// absent. It is either "default" or a toolchain name such as "go1.21.3".
	Toolchain string
}

// Source describes where a resolved version came from, for diagnostics.
func (s *Spec) Source() string {
	if s == nil {
		return ""
	}
	return s.File
}

// Find locates the go.work or go.mod file that governs dir and returns the
// version directives it contains.
//
// A go.work file takes precedence over a go.mod file, and the GOWORK
// environment variable overrides the search entirely. Find returns
// (nil, nil) when neither file exists.
func Find(dir string) (*Spec, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}

	switch gw := os.Getenv("GOWORK"); {
	case gw == "off":
		// Workspace mode is explicitly disabled; only go.mod is consulted.

	case gw != "" && gw != "auto":
		path := gw
		if !filepath.IsAbs(path) {
			path = filepath.Join(abs, path)
		}
		return ParseFile(path)

	default:
		if p := findUp(abs, "go.work"); p != "" {
			return ParseFile(p)
		}
	}

	if p := findUp(abs, "go.mod"); p != "" {
		return ParseFile(p)
	}
	return nil, nil
}

// findUp walks up from dir looking for a file called name.
func findUp(dir, name string) string {
	for {
		p := filepath.Join(dir, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// ParseFile reads path and extracts its go and toolchain directives.
func ParseFile(path string) (*Spec, error) {
	data, err := compat.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(path, data)
}

// Parse extracts the go and toolchain directives from the contents of a
// go.mod or go.work file.
func Parse(path string, data []byte) (*Spec, error) {
	spec := &Spec{
		File:      path,
		IsWork:    filepath.Base(path) == "go.work",
		GoVersion: Lookup(data, "go"),
		Toolchain: Lookup(data, "toolchain"),
	}
	return spec, nil
}

// Lookup returns the value associated with the first line of data that starts
// with key.
//
// It mirrors cmd/go/internal/gover.GoModLookup, which is what the go command
// uses to read the go and toolchain directives. Only non-factored directives
// such as "go" and "toolchain" may be looked up this way.
func Lookup(data []byte, key string) string {
	for len(data) > 0 {
		line, rest, _ := compat.Cut(data, []byte("\n"))
		line = bytes.TrimSpace(line)
		if v, ok := parseKey(line, key); ok {
			return v
		}
		data = rest
	}
	return ""
}

func parseKey(line []byte, key string) (string, bool) {
	if !bytes.HasPrefix(line, []byte(key)) {
		return "", false
	}
	s := string(line[len(key):])
	if len(s) == 0 || (s[0] != ' ' && s[0] != '\t') {
		return "", false
	}
	if i := bytes.Index([]byte(s), []byte("//")); i >= 0 {
		s = s[:i]
	}
	return string(bytes.TrimSpace([]byte(s))), true
}

// Requirement is the release a module asks for.
type Requirement struct {
	// Version is the version the module asks for, for example "1.21" or
	// "1.21.3".
	Version string
	// Exact reports whether the release is pinned to one exact version.
	//
	// The toolchain directive names one exact release, while the go
	// directive only states a minimum: any release that is not older
	// satisfies it.
	Exact bool
}

// Target computes the release a module asks for.
//
// The toolchain directive wins over the go directive, unless it names an
// older release. The result is empty when the module does not ask for a
// particular release, which is the case for a file without directives and
// for the "toolchain default" directive.
func Target(spec *Spec) (Requirement, error) {
	if spec == nil {
		return Requirement{}, nil
	}

	if spec.Toolchain == ToolchainDefault {
		// "default" means always use the default toolchain, so the go
		// directive is not consulted at all.
		return Requirement{}, nil
	}

	var req Requirement

	if spec.Toolchain != "" {
		toolVers, ok := version.FromToolchain(spec.Toolchain)
		if !ok {
			return Requirement{}, fmt.Errorf("invalid toolchain %q in %s", spec.Toolchain, spec.File)
		}
		if version.IsLang(toolVers) && version.Compare(toolVers, "1.21") >= 0 {
			return Requirement{}, fmt.Errorf("invalid toolchain: %s is a language version but not a toolchain version (%s.x)",
				spec.Toolchain, toolVers)
		}
		req = Requirement{Version: toolVers, Exact: true}
	}

	if spec.GoVersion != "" {
		if !version.IsValid(spec.GoVersion) {
			return Requirement{}, fmt.Errorf("invalid go version %q in %s", spec.GoVersion, spec.File)
		}
		if req.Version == "" || version.Compare(spec.GoVersion, req.Version) > 0 {
			req = Requirement{Version: spec.GoVersion}
		}
	}

	return req, nil
}
