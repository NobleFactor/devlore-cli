// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package selector

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// --- ParseOSRelease ---

func TestParseOSRelease_Fields(t *testing.T) {
	content := strings.Join([]string{
		"# a comment, then a blank line",
		"",
		`NAME="Ubuntu"`,
		"ID=ubuntu",
		"ID_LIKE=debian",
		`VERSION_ID="26.04"`,
		"VARIANT_ID='server'",
	}, "\n")

	got, err := ParseOSRelease(strings.NewReader(content))
	if err != nil {
		t.Fatalf("ParseOSRelease: %v", err)
	}
	want := OSRelease{ID: "ubuntu", IDLike: []string{"debian"}, VersionID: "26.04", VariantID: "server"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseOSRelease = %+v, want %+v", got, want)
	}
}

func TestParseOSRelease_Quoting(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{"bare", "ID=fedora", "fedora"},
		{"double quotes", `ID="fedora"`, "fedora"},
		{"single quotes", "ID='fedora'", "fedora"},
		{"escapes in double quotes", `ID="fe\"do\\ra"`, `fe"do\ra`},
		{"single quotes keep backslashes", `ID='fe\dora'`, `fe\dora`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseOSRelease(strings.NewReader(tt.line + "\n"))
			if err != nil {
				t.Fatalf("ParseOSRelease: %v", err)
			}
			if got.ID != tt.want {
				t.Errorf("ID = %q, want %q", got.ID, tt.want)
			}
		})
	}
}

func TestParseOSRelease_IDLikeIsClosestFirst(t *testing.T) {
	got, err := ParseOSRelease(strings.NewReader("ID=rocky\nID_LIKE=\"rhel centos  fedora\"\n"))
	if err != nil {
		t.Fatalf("ParseOSRelease: %v", err)
	}
	if want := []string{"rhel", "centos", "fedora"}; !reflect.DeepEqual(got.IDLike, want) {
		t.Errorf("IDLike = %v, want %v", got.IDLike, want)
	}
}

// --- ReadOSRelease ---

func TestReadOSRelease_FallsBackToTheSecondPath(t *testing.T) {
	dir := t.TempDir()
	second := filepath.Join(dir, "usr-lib-os-release")
	if err := os.WriteFile(second, []byte("ID=debian\n"), 0o644); err != nil {
		t.Fatalf("write the fixture: %v", err)
	}

	got, ok := ReadOSRelease(filepath.Join(dir, "etc-os-release"), second)
	if !ok || got.ID != "debian" {
		t.Errorf("ReadOSRelease = %+v, %v; want ID debian, true", got, ok)
	}
}

func TestReadOSRelease_NoFile(t *testing.T) {
	if got, ok := ReadOSRelease(filepath.Join(t.TempDir(), "absent")); ok || got.ID != "" {
		t.Errorf("ReadOSRelease of a missing file = %+v, %v; want the zero value, false", got, ok)
	}
}
