// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package pkg

import (
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/platform"
)

// TestNewResource_AcceptsTheCanonicalPurlForm is #813's core: the constructor emits `purl.String()` as the
// resource's URI and could not read that form back, because it cut the string at its first colon and read the
// purl scheme as a package manager. Each row is a form the provider must accept.
func TestNewResource_AcceptsTheCanonicalPurlForm(t *testing.T) {

	for _, testCase := range []struct {
		name      string
		manager   string
		value     string
		wantType  string
		wantSpace string
		wantName  string
		wantVer   string
		wantQuals map[string]string
	}{
		{
			name: "a purl with a namespace and a qualifier", manager: "winget",
			value:    "pkg:winget/Vim/Vim?scope=machine",
			wantType: "winget", wantSpace: "Vim", wantName: "Vim",
			wantQuals: map[string]string{"scope": "machine"},
		},
		{
			name: "a purl with a version", manager: "brew",
			value:    "pkg:brew/jq@1.7",
			wantType: "brew", wantName: "jq", wantVer: "1.7",
		},
		{
			name: "a purl with neither", manager: "brew",
			value: "pkg:brew/jq", wantType: "brew", wantName: "jq",
		},
		{
			name: "the manager-prefix form still works", manager: "brew",
			value: "brew:jq", wantType: "brew", wantName: "jq",
		},
		{
			name: "the bare form still works", manager: "apt",
			value: "jq", wantType: "apt", wantName: "jq",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			built, err := NewResource(newTestRuntimeEnvironment(testCase.manager), "", testCase.value)
			if err != nil {
				t.Fatalf("NewResource(%q) = %v; want a resource", testCase.value, err)
			}
			got := built.(*resource)
			if got.typ != testCase.wantType || got.name != testCase.wantName {
				t.Errorf("type/name = %q/%q; want %q/%q", got.typ, got.name, testCase.wantType, testCase.wantName)
			}
			if got.namespace != testCase.wantSpace {
				t.Errorf("namespace = %q; want %q", got.namespace, testCase.wantSpace)
			}
			if got.version != testCase.wantVer {
				t.Errorf("version = %q; want %q", got.version, testCase.wantVer)
			}
			for key, want := range testCase.wantQuals {
				if got.qualifiers[key] != want {
					t.Errorf("qualifier %q = %q; want %q", key, got.qualifiers[key], want)
				}
			}
		})
	}
}

// TestNewResource_AMalformedPurlReportsTheParsersMessage pins the refusal: once a string declares the `pkg:`
// scheme it is a purl, so a parse failure is reported as one and never re-read as a manager prefix -- a fallback
// would turn a typo into a package named after it.
func TestNewResource_AMalformedPurlReportsTheParsersMessage(t *testing.T) {

	_, err := NewResource(newTestRuntimeEnvironment("brew"), "", "pkg:")
	if err == nil {
		t.Fatal("NewResource(\"pkg:\") built a resource; want a refusal")
	}
	if !strings.Contains(err.Error(), "purl") {
		t.Errorf("refusal %q does not carry the parser's message", err)
	}
	if strings.Contains(err.Error(), "unknown package manager") {
		t.Errorf("refusal %q read the scheme as a manager prefix", err)
	}
}

// TestNewResource_AnUnknownPurlTypeIsRefused pins the other half: a well-formed purl whose type no manager on this
// platform answers to is refused naming the type, as an unknown manager prefix is today.
func TestNewResource_AnUnknownPurlTypeIsRefused(t *testing.T) {

	_, err := NewResource(newTestRuntimeEnvironment("brew"), "", "pkg:nixpkgs/jq")
	if err == nil {
		t.Fatal("NewResource with an unknown purl type built a resource; want a refusal")
	}
	if !strings.Contains(err.Error(), "nixpkgs") {
		t.Errorf("refusal %q does not name the type", err)
	}
}

// TestResource_URIRoundTripsThroughTheConstructor is #813's acceptance: the constructor must read what it writes,
// so a resource built from a purl and one built from that resource's URI are the same catalog entry.
func TestResource_URIRoundTripsThroughTheConstructor(t *testing.T) {

	environment := newTestRuntimeEnvironment("winget")
	built, err := NewResource(environment, "", "pkg:winget/Vim/Vim?scope=machine")
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}
	// The contract `unpackCatalog` relies on is the REACHABILITY uri -- the scheme-specific part a provider emits
	// and reads back -- not the tag-wrapped `URI()`. For pkg that part is the canonical purl itself, so #813's
	// round-trip criterion and its parse criterion are the same property read twice.
	specific := concrete(t, built).ReachabilityURI()
	if specific != "pkg:winget/Vim/Vim?scope=machine" {
		t.Fatalf("ReachabilityURI() = %q; want the canonical purl it was built from", specific)
	}
	rebuilt, err := DiscoverResource(environment, specific)
	if err != nil {
		t.Fatalf("DiscoverResource(%q): %v", specific, err)
	}
	if concrete(t, rebuilt).ReachabilityURI() != specific {
		t.Errorf("round trip: %q became %q", specific, concrete(t, rebuilt).ReachabilityURI())
	}
}

// TestToPURL_CarriesNamespaceAndQualifiers is the half that reaches the driver. Parsing a namespace changes
// nothing while every dispatch path rebuilds a PURL from name and type alone: winget would still be asked for
// "Vim" rather than "Vim.Vim", which `windows_managers.go` joins from the namespace.
func TestToPURL_CarriesNamespaceAndQualifiers(t *testing.T) {

	environment := newTestRuntimeEnvironment("winget")
	built, err := NewResource(environment, "", "pkg:winget/Vim/Vim?scope=machine")
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	projected := toPURL(environment.Platform, built)
	if projected.Namespace != "Vim" {
		t.Errorf("projected namespace = %q; want Vim -- the driver joins Publisher.Name from it", projected.Namespace)
	}
	if projected.Qualifiers["scope"] != "machine" {
		t.Errorf("projected qualifier scope = %q; want machine", projected.Qualifiers["scope"])
	}
	if want := (platform.PURL{Type: "winget", Namespace: "Vim", Name: "Vim",
		Qualifiers: map[string]string{"scope": "machine"}}).String(); projected.String() != want {
		t.Errorf("projected purl = %q; want %q", projected.String(), want)
	}
}
