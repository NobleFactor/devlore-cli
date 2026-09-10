// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package platform

import (
	"reflect"
	"strings"
	"testing"
)

// TestParseIdentifier pins the one grammar for package identifiers across its three forms and each platform's
// canonical types: the purl, the manager prefix, and the bare name on the default manager, with the version split
// off and carried, and the namespace and qualifiers kept.
func TestParseIdentifier(t *testing.T) {

	cases := []struct {
		name string
		spec *Spec
		raw  string
		want PURL
	}{
		{"darwin/bare name on the default", Darwin(), "jq",
			PURL{Type: "brew", Name: "jq"}},
		{"darwin/bare name with version", Darwin(), "jq@1.7",
			PURL{Type: "brew", Name: "jq", Version: "1.7"}},
		{"darwin/manager prefix", Darwin(), "port:wget",
			PURL{Type: "port", Name: "wget"}},
		{"darwin/manager prefix with version", Darwin(), "brew:git@2.39.0",
			PURL{Type: "brew", Name: "git", Version: "2.39.0"}},
		{"darwin/purl", Darwin(), "pkg:brew/jq@1.7",
			PURL{Type: "brew", Name: "jq", Version: "1.7"}},
		{"debian/manager name resolves to the purl type", Debian(), "apt:curl",
			PURL{Type: "deb", Name: "curl"}},
		{"debian/purl by type", Debian(), "pkg:deb/curl",
			PURL{Type: "deb", Name: "curl"}},
		{"windows/purl with namespace", Windows(), "pkg:winget/jqlang/jq",
			PURL{Type: "winget", Namespace: "jqlang", Name: "jq"}},
		{"windows/purl with qualifiers", Windows(), "pkg:winget/Vim/Vim?scope=machine",
			PURL{Type: "winget", Namespace: "Vim", Name: "Vim", Qualifiers: map[string]string{"scope": "machine"}}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {

			p, err := New(c.spec)
			if err != nil {
				t.Fatalf("New(%s): %v", c.name, err)
			}

			got, err := ParseIdentifier(p, c.raw)
			if err != nil {
				t.Fatalf("ParseIdentifier(%q): %v", c.raw, err)
			}

			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("ParseIdentifier(%q) = %+v, want %+v", c.raw, got, c.want)
			}
		})
	}
}

// TestParseIdentifier_Refusals pins the refusals: a malformed purl reports the parser's message and is never re-read
// as a manager prefix, and a purl type or a manager prefix no manager on the platform answers to is refused naming
// it.
func TestParseIdentifier_Refusals(t *testing.T) {

	cases := []struct {
		name      string
		spec      *Spec
		raw       string
		wantIn    string
		wantNotIn string
	}{
		{"malformed purl", Darwin(), "pkg:", "purl", "unknown package manager"},
		{"unknown purl type", Darwin(), "pkg:nixpkgs/jq", "nixpkgs", ""},
		{"unknown manager prefix", Darwin(), "apt:jq", "apt", ""},
		{"off-platform purl type", Debian(), "pkg:brew/jq", "brew", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {

			p, err := New(c.spec)
			if err != nil {
				t.Fatalf("New(%s): %v", c.name, err)
			}

			_, err = ParseIdentifier(p, c.raw)
			if err == nil {
				t.Fatalf("ParseIdentifier(%q) parsed; want a refusal", c.raw)
			}

			if !strings.Contains(err.Error(), c.wantIn) {
				t.Errorf("refusal %q does not carry %q", err, c.wantIn)
			}

			if c.wantNotIn != "" && strings.Contains(err.Error(), c.wantNotIn) {
				t.Errorf("refusal %q carries %q", err, c.wantNotIn)
			}
		})
	}
}
