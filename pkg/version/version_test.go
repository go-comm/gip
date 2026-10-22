package version

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		in        string
		wantMajor string
		wantMinor string
		wantPatch string
		wantKind  string
		wantPre   string
		wantValid bool
		wantStr   string
	}{
		{"1.21", "1", "21", "", "", "", true, "1.21"},
		{"go1.21", "1", "21", "", "", "", true, "1.21"},
		{"1.21.0", "1", "21", "0", "", "", true, "1.21.0"},
		{"go1.21.3", "1", "21", "3", "", "", true, "1.21.3"},
		{"1.20", "1", "20", "0", "", "", true, "1.20"},
		{"1.20.5", "1", "20", "5", "", "", true, "1.20.5"},
		{"1.21rc1", "1", "21", "", "rc", "1", true, "1.21rc1"},
		{"1.21.0-rc.1", "", "", "", "", "", false, ""},
		{"go1.22.0", "1", "22", "0", "", "", true, "1.22.0"},
		{"1", "1", "0", "0", "", "", true, "1.0.0"},
		{"", "", "", "", "", "", false, ""},
		{"garbage", "", "", "", "", "", false, ""},
		{"1.21.0.1", "", "", "", "", "", false, ""},
	}
	for _, tt := range tests {
		got := Parse(tt.in)
		if got.IsValid() != tt.wantValid {
			t.Errorf("Parse(%q).IsValid() = %v, want %v", tt.in, got.IsValid(), tt.wantValid)
			continue
		}
		if !tt.wantValid {
			continue
		}
		if got.Major != tt.wantMajor || got.Minor != tt.wantMinor || got.Patch != tt.wantPatch ||
			got.Kind != tt.wantKind || got.Pre != tt.wantPre {
			t.Errorf("Parse(%q) = %+v, want major=%q minor=%q patch=%q kind=%q pre=%q",
				tt.in, got, tt.wantMajor, tt.wantMinor, tt.wantPatch, tt.wantKind, tt.wantPre)
		}
		if got.String() != tt.wantStr {
			t.Errorf("Parse(%q).String() = %q, want %q", tt.in, got.String(), tt.wantStr)
		}
	}
}

// TestCompare pins down Go's toolchain ordering. The ordering of the
// language version 1.21 relative to 1.21rc1 and 1.21.0 is the subtle part:
// the language version sorts *below* both prereleases and releases.
func TestCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.21", "1.21rc1", -1},
		{"1.21rc1", "1.21.0", -1},
		{"1.21", "1.21.0", -1},
		{"1.21.0", "1.21.1", -1},
		{"1.20", "1.20.0", 0},
		{"1.20.0", "1.20", 0},
		{"1.19", "1.19.0", 0},
		{"1.21rc1", "1.21rc2", -1},
		{"1.21", "1.22", -1},
		{"1.9", "1.10", -1},
		{"1.21.10", "1.21.9", 1},
		{"go1.21.0", "1.21.0", 0},
		{"1.21.0", "1.21.0", 0},
		{"1.22", "1.22.0", -1},
		{"1.20", "1.21", -1},
	}
	for _, tt := range tests {
		if got := Compare(tt.a, tt.b); got != tt.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
		if got := Compare(tt.b, tt.a); got != -tt.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", tt.b, tt.a, got, -tt.want)
		}
	}
}

func TestIsLang(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"1.21", true},
		{"go1.21", true},
		{"1.21.0", false},
		{"1.20", false}, // before Go 1.21 "1.20" is a concrete release
		{"1.20.0", false},
		{"1.21rc1", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsLang(tt.in); got != tt.want {
			t.Errorf("IsLang(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestCanonical(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"1.21", "1.21.0"},
		{"go1.21", "1.21.0"},
		{"1.21.0", "1.21.0"},
		{"1.21.3", "1.21.3"},
		{"1.20", "1.20"},
		{"1.20.5", "1.20.5"},
		{"1.19", "1.19"},
		{"1.21rc1", "1.21rc1"},
		{"1.22", "1.22.0"},
	}
	for _, tt := range tests {
		got, ok := Canonical(tt.in)
		if !ok {
			t.Errorf("Canonical(%q) failed", tt.in)
			continue
		}
		if got != tt.want {
			t.Errorf("Canonical(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestFromToolchain(t *testing.T) {
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"go1.2.3", "1.2.3", true},
		{"go1.21.3-bigcorp", "1.21.3", true},
		{"go1.21.3 other", "1.21.3", true},
		{"go1.21", "1.21", true},
		{"1.2.3", "", false},
		{"go", "", false},
		{`go1.2.3\x`, "", false},
		{"", "", false},
	}
	for _, tt := range tests {
		got, ok := FromToolchain(tt.in)
		if got != tt.want || ok != tt.ok {
			t.Errorf("FromToolchain(%q) = %q, %v; want %q, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestIsRelease(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"1.22.0", true},
		{"1.19.10", true},
		{"1.21rc1", true},
		{"go1.22.0", true},
		{"1.22", false},
		{"1.21", false},
		{"1.19", false},
		{"1.20", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsRelease(tt.in); got != tt.want {
			t.Errorf("IsRelease(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestLang(t *testing.T) {
	tests := []struct{ in, want string }{
		{"1.21.3", "1.21"},
		{"1.21", "1.21"},
		{"1.21rc1", "1.21"},
		{"2.0", "2.0"},
	}
	for _, tt := range tests {
		if got := Lang(tt.in); got != tt.want {
			t.Errorf("Lang(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
