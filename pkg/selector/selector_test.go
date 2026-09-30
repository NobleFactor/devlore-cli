// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package selector

import (
	"reflect"
	"strings"
	"testing"
)

// ubuntuArm64 is the owner's machine: Ubuntu, whose os-release names Debian as its lineage, on arm64.
//
// Parameters:
//   - `t`: the test.
//
// Returns:
//   - `Host`: the host.
func ubuntuArm64(t *testing.T) Host {

	t.Helper()
	return hostFrom(t, "linux", "arm64", "ID=ubuntu\nID_LIKE=debian\n")
}

// selectedNames returns the names of a selection, in its order.
//
// Parameters:
//   - `selected`: the selection.
//
// Returns:
//   - `[]string`: the names.
func selectedNames(selected []Selected) []string {

	names := make([]string, 0, len(selected))
	for _, s := range selected {
		names = append(names, s.Name)
	}
	return names
}

// violations maps each grammar error's name to its violation.
//
// Parameters:
//   - `errs`: the grammar errors.
//
// Returns:
//   - `map[string]Violation`: each name's violation.
func violations(errs []*GrammarError) map[string]Violation {

	found := make(map[string]Violation, len(errs))
	for _, e := range errs {
		found[e.Name] = e.Violation
	}
	return found
}

// --- Select ---

// TestSelect_OrderOfApplication pins the ruled order (Q3): the OS link's depth first, then the architecture, each part
// narrowing the one to its left. The names are given shuffled.
func TestSelect_OrderOfApplication(t *testing.T) {
	want := []string{
		"common", "common.arm64", "common.Unix", "common.Unix.arm64", "common.Linux", "common.Linux.arm64",
		"common.Debian", "common.Debian.arm64", "common.Ubuntu", "common.Ubuntu.arm64",
	}
	shuffled := []string{
		"common.Ubuntu.arm64", "common.Linux", "common", "common.Debian.arm64", "common.Unix", "common.Ubuntu",
		"common.arm64", "common.Linux.arm64", "common.Debian", "common.Unix.arm64",
	}

	selected, errs := Selector{Host: ubuntuArm64(t), Projects: []string{"common"}}.Select(shuffled)
	if len(errs) != 0 {
		t.Fatalf("grammar errors: %v", errs)
	}
	if got := selectedNames(selected); !reflect.DeepEqual(got, want) {
		t.Errorf("order of application:\n got %v\nwant %v", got, want)
	}
}

func TestSelect_BothSpellingsOfAnArchitecture(t *testing.T) {
	selected, errs := Selector{Host: ubuntuArm64(t), Projects: []string{"common"}}.Select([]string{"common.aarch64"})
	if len(errs) != 0 || len(selected) != 1 {
		t.Errorf("common.aarch64 on arm64: selected %v, errors %v; want it selected", selectedNames(selected), errs)
	}
}

// TestSelect_OtherMachinesAreExcludedSilently is the shape of the owner's personal layer on this machine.
func TestSelect_OtherMachinesAreExcludedSilently(t *testing.T) {
	names := []string{
		"common.Darwin", "common.Windows", "microsoft.Windows", "common.Fedora", "common.amd64", "common.x86_64",
		"common.Darwin.arm64", "common.Debian",
	}

	selected, errs := Selector{Host: ubuntuArm64(t), Projects: []string{"common", "microsoft"}}.Select(names)
	if len(errs) != 0 {
		t.Errorf("grammar errors: %v; want none", errs)
	}
	if got, want := selectedNames(selected), []string{"common.Debian"}; !reflect.DeepEqual(got, want) {
		t.Errorf("selected %v, want %v", got, want)
	}
}

// TestSelect_GrammarErrors: out of order, two OS words, and words outside the vocabulary are all reported, together.
func TestSelect_GrammarErrors(t *testing.T) {
	names := []string{"common.arm64.Debian", "common.Linux.Debian", "common.Debain", "common.Nixos", "common.Ubuntu"}

	selected, errs := Selector{Host: ubuntuArm64(t), Projects: []string{"common"}}.Select(names)
	want := map[string]Violation{
		"common.arm64.Debian": OutOfOrder,
		"common.Linux.Debian": RepeatedPart,
		"common.Debain":       UnknownWord,
		"common.Nixos":        UnknownWord,
	}
	if got := violations(errs); !reflect.DeepEqual(got, want) {
		t.Errorf("violations = %v, want %v", got, want)
	}
	if got := selectedNames(selected); !reflect.DeepEqual(got, []string{"common.Ubuntu"}) {
		t.Errorf("selected %v, want [common.Ubuntu]", got)
	}
	for _, e := range errs {
		if !strings.Contains(e.Error(), e.Name) {
			t.Errorf("error %q doesn't name its directory %q", e.Error(), e.Name)
		}
	}
}

// TestSelect_TheHostsOwnDistributionIsAWord: a distribution the table doesn't list names itself on its own host.
func TestSelect_TheHostsOwnDistributionIsAWord(t *testing.T) {
	host := hostFrom(t, "linux", "amd64", "ID=nixos\n")

	selected, errs := Selector{Host: host, Projects: []string{"common"}}.Select([]string{"common.Nixos"})
	if len(errs) != 0 || len(selected) != 1 {
		t.Errorf("common.Nixos on NixOS: selected %v, errors %v; want it selected", selectedNames(selected), errs)
	}
}

// TestSelect_ProjectsInTheCallersOrder pins Q27: projects first, in the order given, then the platform ranking.
func TestSelect_ProjectsInTheCallersOrder(t *testing.T) {
	names := []string{"thenobles", "noblefactor", "common.Ubuntu", "common", "other.Ubuntu"}

	sel := Selector{Host: ubuntuArm64(t), Projects: []string{"common", "noblefactor", "thenobles"}}
	selected, errs := sel.Select(names)
	if len(errs) != 0 {
		t.Fatalf("grammar errors: %v", errs)
	}
	want := []string{"common", "common.Ubuntu", "noblefactor", "thenobles"}
	if got := selectedNames(selected); !reflect.DeepEqual(got, want) {
		t.Errorf("order of application:\n got %v\nwant %v", got, want)
	}
}

func TestSelect_Extras(t *testing.T) {
	sel := Selector{
		Host:     ubuntuArm64(t),
		Projects: []string{"noblefactor"},
		Segments: []Segment{
			{Name: "ROLE", Values: []string{"desktop", "server"}, Value: "desktop"},
			{Name: "SITE", Values: []string{"aws", "home"}, Value: "home"},
		},
	}
	names := []string{
		"noblefactor.Debian.arm64.desktop", "noblefactor.desktop", "noblefactor.server", "noblefactor.desktop.home",
		"noblefactor.home.desktop", "noblefactor.desktop.Debian", "noblefactor.dekstop",
	}

	selected, errs := sel.Select(names)
	wantSelected := []string{"noblefactor.desktop", "noblefactor.desktop.home", "noblefactor.Debian.arm64.desktop"}
	if got := selectedNames(selected); !reflect.DeepEqual(got, wantSelected) {
		t.Errorf("selected:\n got %v\nwant %v", got, wantSelected)
	}
	wantErrs := map[string]Violation{
		"noblefactor.home.desktop":   OutOfOrder,
		"noblefactor.desktop.Debian": OutOfOrder,
		"noblefactor.dekstop":        UnknownWord,
	}
	if got := violations(errs); !reflect.DeepEqual(got, wantErrs) {
		t.Errorf("violations = %v, want %v", got, wantErrs)
	}
}

func TestSelect_AnUnsetSegmentMatchesNothing(t *testing.T) {
	sel := Selector{
		Host:     ubuntuArm64(t),
		Projects: []string{"noblefactor"},
		Segments: []Segment{{Name: "SITE", Values: []string{"aws", "home"}}},
	}

	selected, errs := sel.Select([]string{"noblefactor", "noblefactor.home"})
	if len(errs) != 0 {
		t.Errorf("grammar errors: %v", errs)
	}
	if got := selectedNames(selected); !reflect.DeepEqual(got, []string{"noblefactor"}) {
		t.Errorf("selected %v, want [noblefactor]", got)
	}
}

// TestSelect_LoreNames: lore's names carry no project; Common is the name with no selector words (Q24).
func TestSelect_LoreNames(t *testing.T) {
	names := []string{"Ubuntu", "Common", "Debian.arm64", "Darwin", "Linux", "arm64", "Debian", "Unix", "Linux.Debian"}

	selected, errs := Selector{Host: ubuntuArm64(t), Base: "Common"}.Select(names)
	want := []string{"Common", "arm64", "Unix", "Linux", "Debian", "Debian.arm64", "Ubuntu"}
	if got := selectedNames(selected); !reflect.DeepEqual(got, want) {
		t.Errorf("order of application:\n got %v\nwant %v", got, want)
	}
	if got := violations(errs); !reflect.DeepEqual(got, map[string]Violation{"Linux.Debian": RepeatedPart}) {
		t.Errorf("violations = %v, want Linux.Debian: RepeatedPart", got)
	}
}

// --- ValidateSegments ---

func TestValidateSegments(t *testing.T) {
	tests := []struct {
		name     string
		segments []Segment
		wantErr  string
	}{
		{"valid", []Segment{
			{Name: "ROLE", Values: []string{"desktop", "server"}, Value: "desktop"},
			{Name: "SITE", Values: []string{"aws", "home"}},
		}, ""},
		{"a repeated name", []Segment{{Name: "ROLE", Values: []string{"a"}}, {Name: "ROLE", Values: []string{"b"}}},
			"ROLE"},
		{"a built-in name", []Segment{{Name: "DISTRO", Values: []string{"a"}}}, "DISTRO"},
		{"a value two segments share", []Segment{
			{Name: "ROLE", Values: []string{"server"}}, {Name: "SITE", Values: []string{"server"}},
		}, "server"},
		{"an OS word", []Segment{{Name: "ROLE", Values: []string{"Linux"}}}, "Linux"},
		{"a distribution word", []Segment{{Name: "ROLE", Values: []string{"Debian"}}}, "Debian"},
		{"an architecture word", []Segment{{Name: "ROLE", Values: []string{"aarch64"}}}, "aarch64"},
		{"a value not declared", []Segment{{Name: "ROLE", Values: []string{"desktop"}, Value: "server"}},
			"server"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSegments(tt.segments)
			switch {
			case tt.wantErr == "" && err != nil:
				t.Errorf("ValidateSegments: %v, want nil", err)
			case tt.wantErr != "" && err == nil:
				t.Errorf("ValidateSegments: nil, want an error naming %q", tt.wantErr)
			case tt.wantErr != "" && !strings.Contains(err.Error(), tt.wantErr):
				t.Errorf("ValidateSegments: %v, want an error naming %q", err, tt.wantErr)
			}
		})
	}
}
