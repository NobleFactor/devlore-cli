// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package segment

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/selector"
)

// ubuntuArm64 is the owner's machine: Ubuntu, whose lineage is Debian, on arm64.
//
// Returns:
//   - `Segments`: its built-in segments.
func ubuntuArm64() Segments {

	return Segments{
		{Name: "OS", Value: "Linux"},
		{Name: "DISTRO", Value: "Ubuntu", Lineage: []string{"Debian"}},
		{Name: "ARCH", Value: "arm64"},
	}
}

// layer makes a layer tree with the named directories, and returns its root.
//
// Parameters:
//   - `t`: the test that owns the directory.
//   - `names`: the directories to make.
//
// Returns:
//   - `string`: the root.
func layer(t *testing.T, names ...string) string {

	t.Helper()
	root := t.TempDir()
	for _, name := range names {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatalf("make %s: %v", name, err)
		}
	}
	return root
}

// --- DetectSegments ---

func TestDetectSegments_Builtins(t *testing.T) {
	segs := DetectSegments()

	if len(segs) != 3 {
		t.Fatalf("DetectSegments() returned %d segments, want OS, DISTRO and ARCH", len(segs))
	}
	if segs.Get("OS") == "" {
		t.Error("OS segment is empty")
	}
	if segs.Get("ARCH") == "" {
		t.Error("ARCH segment is empty")
	}

	// DISTRO is set on a Linux host whose os-release names its distribution (#959).
	if runtime.GOOS == "linux" {
		release, ok := selector.ReadOSRelease()
		if ok && release.ID != "" && segs.Get("DISTRO") == "" {
			t.Error("DISTRO segment is empty, though os-release carries an ID")
		}
	}
}

// --- Segments.Host ---

func TestSegmentsHost_ChainFollowsTheLineage(t *testing.T) {
	host := ubuntuArm64().Host()

	if want := []string{"Unix", "Linux", "Debian", "Ubuntu"}; !reflect.DeepEqual(host.Chain, want) {
		t.Errorf("Chain = %v, want %v", host.Chain, want)
	}
}

func TestSegmentsHost_ADistroOverrideKeepsTheLineage(t *testing.T) {
	host := ubuntuArm64().Set("DISTRO", "Mint").Host()

	if want := []string{"Unix", "Linux", "Debian", "Mint"}; !reflect.DeepEqual(host.Chain, want) {
		t.Errorf("Chain = %v, want %v", host.Chain, want)
	}
}

// --- MatchDirectories ---

func TestMatchDirectories_SelectsInOrderOfApplication(t *testing.T) {
	root := layer(t, "common.Ubuntu", "common", "common.Unix", "common.Darwin", "common.Debian.arm64", "common.Debian",
		"noblefactor", "other.Ubuntu", ".git")

	matches, grammarErrors, err := MatchDirectories(root, []string{"common", "noblefactor"}, ubuntuArm64())
	if err != nil {
		t.Fatalf("MatchDirectories: %v", err)
	}
	if len(grammarErrors) != 0 {
		t.Errorf("grammar errors: %v", grammarErrors)
	}

	var got []string
	for _, m := range matches {
		got = append(got, filepath.Base(m.Path))
	}
	want := []string{"common", "common.Unix", "common.Debian", "common.Debian.arm64", "common.Ubuntu", "noblefactor"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("order of application:\n got %v\nwant %v", got, want)
	}
}

func TestMatchDirectories_ReportsEveryGrammarError(t *testing.T) {
	root := layer(t, "common.Linux.Debian", "common.arm64.Debian", "common.Debain", "unrequested.Debain", "common")

	matches, grammarErrors, err := MatchDirectories(root, []string{"common"}, ubuntuArm64())
	if err != nil {
		t.Fatalf("MatchDirectories: %v", err)
	}
	if len(matches) != 1 {
		t.Errorf("matches = %v, want common alone", matches)
	}

	var names []string
	for _, e := range grammarErrors {
		names = append(names, e.Name)
	}
	want := []string{"common.Debain", "common.Linux.Debian", "common.arm64.Debian", "unrequested.Debain"}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("grammar errors = %v, want %v: every directory in the layer is judged", names, want)
	}
}

// --- Resolve ---

func TestResolve_ValuesFromConfigurationEnvironmentAndFlags(t *testing.T) {
	declared := []selector.Segment{
		{Name: "ROLE", Values: []string{"desktop", "server"}, Value: "desktop"},
		{Name: "SITE", Values: []string{"aws", "home"}, Value: "home"},
	}
	t.Setenv(EnvVarPrefix+"SITE", "aws")

	segs, err := Resolve(declared, []string{"ROLE=server"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := segs.Get("ROLE"); got != "server" {
		t.Errorf("ROLE = %q, want server: the flag beats configuration", got)
	}
	if got := segs.Get("SITE"); got != "aws" {
		t.Errorf("SITE = %q, want aws: the environment beats configuration", got)
	}
	if extras := segs.Extras(); len(extras) != 2 || extras[0].Name != "ROLE" || extras[1].Name != "SITE" {
		t.Errorf("Extras = %+v, want ROLE then SITE, in configured order", extras)
	}
}

func TestResolve_BuiltinsTakeAValueWithoutADeclaration(t *testing.T) {
	segs, err := Resolve(nil, []string{"DISTRO=Debian"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got := segs.Get("DISTRO"); got != "Debian" {
		t.Errorf("DISTRO = %q, want Debian", got)
	}
}

func TestResolve_Refusals(t *testing.T) {
	declared := []selector.Segment{{Name: "ROLE", Values: []string{"desktop", "server"}}}

	tests := []struct {
		name     string
		declared []selector.Segment
		flags    []string
		env      string
		want     string
	}{
		{"an undeclared segment", declared, []string{"SITE=aws"}, "", "SITE is not a declared segment"},
		{"an undeclared value", declared, []string{"ROLE=laptop"}, "", `"laptop" is not one of ROLE's`},
		{"an undeclared variable", declared, nil, "SITE=aws", "SITE is not a declared segment"},
		{"a flag without a value", declared, []string{"ROLE"}, "", "expected NAME=value"},
		{"a malformed declaration", []selector.Segment{{Name: "OS", Values: []string{"a"}}}, nil, "", "built in"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				name, value, _ := strings.Cut(tt.env, "=")
				t.Setenv(EnvVarPrefix+name, value)
			}
			_, err := Resolve(tt.declared, tt.flags)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Resolve: %v, want an error containing %q", err, tt.want)
			}
		})
	}
}

// --- Refusal ---

func TestRefusal_ListsEveryName(t *testing.T) {
	refusal := &Refusal{Entries: []RefusalEntry{
		{Root: "/base/Home", Err: &selector.GrammarError{Name: "common.Debain", Word: "Debain", Violation: selector.UnknownWord}},
		{Root: "/personal/Home", Err: &selector.GrammarError{Name: "common.Linux.Debian", Word: "Debian",
			Violation: selector.RepeatedPart, Part: "OS"}},
	}}

	var target *Refusal
	if !errors.As(error(refusal), &target) {
		t.Fatal("a Refusal is not an error")
	}
	message := refusal.Error()
	for _, want := range []string{"2 directory name(s)", "/base/Home: common.Debain", "/personal/Home: common.Linux.Debian"} {
		if !strings.Contains(message, want) {
			t.Errorf("Error() = %q, want it to contain %q", message, want)
		}
	}
}
