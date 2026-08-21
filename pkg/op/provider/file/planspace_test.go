// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package file

import (
	"strings"
	"testing"
)

// The plan-space little language (#584, ruled 2026-08-20): `foo/bar` ≡ `/foo/bar`; volume, UNC, and
// backslash spellings refuse; `@name/…` is reserved for the named multi-root design; escapes and the bare
// root refuse. One table, every rule.

func TestNormalizePlanSpacePath_TheLittleLanguage(t *testing.T) {

	accepted := []struct {
		name string
		in   string
		want string
	}{
		{"bare rel", "foo/bar", "foo/bar"},
		{"anchored spelling names the same rel", "/foo/bar", "foo/bar"},
		{"dot segments collapse", "a/./b/../c.txt", "a/c.txt"},
		{"duplicate slashes collapse", "a//b", "a/b"},
		{"colon beyond the drive position rides as a path", "https://example.com/x", "https:/example.com/x"},
	}

	for _, tc := range accepted {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizePlanSpacePath(tc.in)
			if err != nil {
				t.Fatalf("NormalizePlanSpacePath(%q): unexpected refusal: %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("NormalizePlanSpacePath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}

	refused := []struct {
		name   string
		in     string
		reason string
	}{
		{"drive letter, backslash form", `C:\x`, "volume"},
		{"drive letter, slash form", "C:/x", "volume"},
		{"drive-relative", "C:x", "volume"},
		{"UNC, backslash form", `\\server\share`, "UNC"},
		{"UNC, slash form", "//server/share", "UNC"},
		{"bare backslash separator", `foo\bar`, "backslash"},
		{"root-qualified is reserved", "@config/foo", "root-qualified"},
		{"escape", "../secrets", "escapes"},
		{"escape after cleaning", "a/../../secrets", "escapes"},
		{"the root itself", "/", "root"},
		{"empty", "", "root"},
		{"dot", ".", "root"},
	}

	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NormalizePlanSpacePath(tc.in)
			if err == nil {
				t.Fatalf("NormalizePlanSpacePath(%q): expected refusal, got acceptance", tc.in)
			}
			if !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("NormalizePlanSpacePath(%q): refusal %q does not name %q", tc.in, err, tc.reason)
			}
		})
	}
}
