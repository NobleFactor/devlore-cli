// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package platform

import (
	"strings"
	"testing"
)

// region PURL.String

func TestPURLStringMinimal(t *testing.T) {

	p := PURL{Type: "brew", Name: "jq"}

	got := p.String()
	want := "pkg:brew/jq"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestPURLStringWithVersion(t *testing.T) {

	p := PURL{Type: "brew", Name: "jq", Version: "1.7"}

	got := p.String()
	want := "pkg:brew/jq@1.7"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestPURLStringWithNamespace(t *testing.T) {

	p := PURL{Type: "winget", Namespace: "Microsoft", Name: "VisualStudioCode"}

	got := p.String()
	want := "pkg:winget/Microsoft/VisualStudioCode"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestPURLStringWithQualifiersSorted(t *testing.T) {

	p := PURL{
		Type: "deb",
		Name: "jq",
		Qualifiers: map[string]string{
			"distro": "ubuntu",
			"arch":   "amd64",
		},
	}

	got := p.String()
	// Qualifiers are alphabetized; arch comes before distro.
	want := "pkg:deb/jq?arch=amd64&distro=ubuntu"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestPURLStringWithSubpath(t *testing.T) {

	p := PURL{Type: "deb", Name: "jq", Subpath: "src/main.go"}

	got := p.String()
	want := "pkg:deb/jq#src/main.go"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// endregion

// region ParsePURL

func TestParsePURLMinimal(t *testing.T) {

	got, err := ParsePURL("pkg:brew/jq")
	if err != nil {
		t.Fatalf("ParsePURL: %v", err)
	}

	if got.Type != "brew" {
		t.Errorf("Type = %q, want brew", got.Type)
	}
	if got.Name != "jq" {
		t.Errorf("Name = %q, want jq", got.Name)
	}
	if got.Version != "" {
		t.Errorf("Version = %q, want empty", got.Version)
	}
}

func TestParsePURLWithVersion(t *testing.T) {

	got, err := ParsePURL("pkg:brew/jq@1.7")
	if err != nil {
		t.Fatalf("ParsePURL: %v", err)
	}

	if got.Version != "1.7" {
		t.Errorf("Version = %q, want 1.7", got.Version)
	}
}

func TestParsePURLWithNamespace(t *testing.T) {

	got, err := ParsePURL("pkg:winget/Microsoft/VisualStudioCode")
	if err != nil {
		t.Fatalf("ParsePURL: %v", err)
	}

	if got.Type != "winget" {
		t.Errorf("Type = %q, want winget", got.Type)
	}
	if got.Namespace != "Microsoft" {
		t.Errorf("Namespace = %q, want Microsoft", got.Namespace)
	}
	if got.Name != "VisualStudioCode" {
		t.Errorf("Name = %q, want VisualStudioCode", got.Name)
	}
}

func TestParsePURLWithQualifiers(t *testing.T) {

	got, err := ParsePURL("pkg:deb/jq?arch=amd64&distro=ubuntu")
	if err != nil {
		t.Fatalf("ParsePURL: %v", err)
	}

	if got.Qualifiers["arch"] != "amd64" {
		t.Errorf("Qualifiers[arch] = %q, want amd64", got.Qualifiers["arch"])
	}
	if got.Qualifiers["distro"] != "ubuntu" {
		t.Errorf("Qualifiers[distro] = %q, want ubuntu", got.Qualifiers["distro"])
	}
}

func TestParsePURLWithSubpath(t *testing.T) {

	got, err := ParsePURL("pkg:deb/jq#src/main.go")
	if err != nil {
		t.Fatalf("ParsePURL: %v", err)
	}

	if got.Subpath != "src/main.go" {
		t.Errorf("Subpath = %q, want src/main.go", got.Subpath)
	}
}

// endregion

// region ParsePURL error cases

func TestParsePURLErrorsOnMissingScheme(t *testing.T) {

	_, err := ParsePURL("brew/jq")

	if err == nil {
		t.Fatal("ParsePURL returned nil error, want missing-scheme error")
	}
	if !strings.Contains(err.Error(), "missing pkg") {
		t.Errorf("error text = %q, want substring %q", err.Error(), "missing pkg")
	}
}

func TestParsePURLErrorsOnMissingType(t *testing.T) {

	_, err := ParsePURL("pkg:")

	if err == nil {
		t.Fatal("ParsePURL returned nil error, want missing-type error")
	}
	if !strings.Contains(err.Error(), "missing type") {
		t.Errorf("error text = %q, want substring %q", err.Error(), "missing type")
	}
}

func TestParsePURLErrorsOnMissingName(t *testing.T) {

	_, err := ParsePURL("pkg:brew/")

	if err == nil {
		t.Fatal("ParsePURL returned nil error, want missing-name error")
	}
	if !strings.Contains(err.Error(), "missing name") {
		t.Errorf("error text = %q, want substring %q", err.Error(), "missing name")
	}
}

// endregion

// region Round-trip

func TestPURLRoundTrip(t *testing.T) {

	for _, tc := range []struct {
		name string
		raw  string
	}{
		{"minimal", "pkg:brew/jq"},
		{"with version", "pkg:brew/jq@1.7"},
		{"with namespace", "pkg:winget/Microsoft/VisualStudioCode"},
		{"with subpath", "pkg:deb/jq#src/main.go"},
		{"with qualifiers", "pkg:deb/jq?arch=amd64&distro=ubuntu"},
	} {
		t.Run(tc.name, func(t *testing.T) {

			p, err := ParsePURL(tc.raw)
			if err != nil {
				t.Fatalf("ParsePURL: %v", err)
			}

			got := p.String()
			if got != tc.raw {
				t.Errorf("round-trip = %q, want %q", got, tc.raw)
			}
		})
	}
}

// endregion
