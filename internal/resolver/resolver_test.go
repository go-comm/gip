package resolver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-comm/gip/internal/compat"
	"github.com/go-comm/gip/internal/testutil"
)

func TestMain(m *testing.M) {
	code := m.Run()
	testutil.Cleanup()
	os.Exit(code)
}

func TestLookup(t *testing.T) {
	tests := []struct {
		data string
		key  string
		want string
	}{
		{"module m\n\ngo 1.21\n", "go", "1.21"},
		{"module m\n\ngo 1.21.0\n", "go", "1.21.0"},
		{"module m\n\ngo 1.21 // comment\n", "go", "1.21"},
		{"module m\n\ngo\t1.21\n", "go", "1.21"},
		{"module m\n\ntoolchain go1.21.3\n", "toolchain", "go1.21.3"},
		{"module m\n\ntoolchain default\n", "toolchain", "default"},
		{"module m\n", "go", ""},
		{"module m\n\nrequire (\n\tgolang.org/x/text v0.3.0\n)\n", "go", ""},
		// A line that merely starts with the key is not a directive.
		{"module m\n\ngodebug default=go1.21\n", "go", ""},
		{"module m\n\ntoolchainfoo bar\n", "toolchain", ""},
	}
	for _, tt := range tests {
		if got := Lookup([]byte(tt.data), tt.key); got != tt.want {
			t.Errorf("Lookup(%q, %q) = %q, want %q", tt.data, tt.key, got, tt.want)
		}
	}
}

func TestFind(t *testing.T) {
	root := testutil.TempDir(t)
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := compat.WriteFile(filepath.Join(root, "go.mod"), []byte("module m\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}

	spec, err := Find(sub)
	if err != nil {
		t.Fatal(err)
	}
	if spec == nil {
		t.Fatal("Find returned no spec")
	}
	if spec.GoVersion != "1.21" {
		t.Errorf("GoVersion = %q, want 1.21", spec.GoVersion)
	}
	if spec.IsWork {
		t.Error("IsWork = true, want false")
	}

	// A go.work file takes precedence over a go.mod file.
	if err := compat.WriteFile(filepath.Join(root, "go.work"), []byte("go 1.22\n"), 0644); err != nil {
		t.Fatal(err)
	}
	spec, err = Find(sub)
	if err != nil {
		t.Fatal(err)
	}
	if !spec.IsWork {
		t.Error("IsWork = false, want true")
	}
	if spec.GoVersion != "1.22" {
		t.Errorf("GoVersion = %q, want 1.22", spec.GoVersion)
	}

	// GOWORK=off disables the workspace.
	testutil.Setenv(t, "GOWORK", "off")
	spec, err = Find(sub)
	if err != nil {
		t.Fatal(err)
	}
	if spec.IsWork {
		t.Error("IsWork = true, want false with GOWORK=off")
	}
	if spec.GoVersion != "1.21" {
		t.Errorf("GoVersion = %q, want 1.21", spec.GoVersion)
	}
}

// TestTarget pins the toolchain selection rules.
func TestTarget(t *testing.T) {
	tests := []struct {
		name     string
		spec     *Spec
		want     string
		wantExct bool
		wantErr  bool
	}{
		{
			name: "no spec asks for nothing",
			spec: nil,
			want: "",
		},
		{
			name: "the go line states a minimum",
			spec: &Spec{GoVersion: "1.21.0"},
			want: "1.21.0",
		},
		{
			name: "a language version states a minimum",
			spec: &Spec{GoVersion: "1.21"},
			want: "1.21",
		},
		{
			name: "the toolchain line wins over an older go line",
			spec: &Spec{GoVersion: "1.21.0", Toolchain: "go1.23.0"},
			want: "1.23.0", wantExct: true,
		},
		{
			name: "an older toolchain line loses to the go line",
			spec: &Spec{GoVersion: "1.21", Toolchain: "go1.20.5"},
			want: "1.21",
		},
		{
			name: "a newer go line wins over an older toolchain line",
			spec: &Spec{GoVersion: "1.22", Toolchain: "go1.21.5"},
			want: "1.22",
		},
		{
			name: "toolchain default asks for nothing",
			spec: &Spec{GoVersion: "1.24.0", Toolchain: "default"},
			want: "",
		},
		{
			name:    "an invalid toolchain is an error",
			spec:    &Spec{GoVersion: "1.21.0", Toolchain: "1.23.0"},
			wantErr: true,
		},
		{
			name:    "a language version is not a toolchain",
			spec:    &Spec{GoVersion: "1.21.0", Toolchain: "go1.22"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Target(tt.spec)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Target = %+v, want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Target: %v", err)
			}
			if got.Version != tt.want {
				t.Errorf("Target().Version = %q, want %q", got.Version, tt.want)
			}
			if got.Exact != tt.wantExct {
				t.Errorf("Target().Exact = %v, want %v", got.Exact, tt.wantExct)
			}
		})
	}
}
