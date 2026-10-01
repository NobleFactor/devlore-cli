// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package writ

import (
	"testing"

	"github.com/spf13/viper"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/tree"
)

// --- PartitionByScope ---

// TestPartitionByScope_MixedSystemHome proves sources split into a System and a Home partition, each keeping the
// layer order it was given: System holds base then team, Home holds base, team, then personal.
//
// Parameters:
//   - `t`: the test harness.
func TestPartitionByScope_MixedSystemHome(t *testing.T) {
	t.Helper()

	sources := []tree.LayerSource{
		{Layer: "base", Order: 0, ScopeName: "System", TargetRoot: "/", SourceRoot: "/repo/base/System"},
		{Layer: "base", Order: 0, ScopeName: "Home", TargetRoot: "/home/user", SourceRoot: "/repo/base/Home"},
		{Layer: "team", Order: 1, ScopeName: "System", TargetRoot: "/", SourceRoot: "/repo/team/System"},
		{Layer: "team", Order: 1, ScopeName: "Home", TargetRoot: "/home/user", SourceRoot: "/repo/team/Home"},
		{Layer: "personal", Order: 2, ScopeName: "Home", TargetRoot: "/home/user", SourceRoot: "/repo/personal/Home"},
	}

	partitions := PartitionByScope(sources)

	// Two keys: System and Home
	if len(partitions) != 2 {
		t.Fatalf("got %d partitions, want 2", len(partitions))
	}

	// System partition: base, team
	sys := partitions["System"]
	if len(sys) != 2 {
		t.Fatalf("System partition has %d entries, want 2", len(sys))
	}
	if sys[0].Layer != "base" {
		t.Errorf("System[0].Layer = %q, want %q", sys[0].Layer, "base")
	}
	if sys[1].Layer != "team" {
		t.Errorf("System[1].Layer = %q, want %q", sys[1].Layer, "team")
	}

	// Home partition: base, team, personal
	home := partitions["Home"]
	if len(home) != 3 {
		t.Fatalf("Home partition has %d entries, want 3", len(home))
	}
	if home[0].Layer != "base" {
		t.Errorf("Home[0].Layer = %q, want %q", home[0].Layer, "base")
	}
	if home[1].Layer != "team" {
		t.Errorf("Home[1].Layer = %q, want %q", home[1].Layer, "team")
	}
	if home[2].Layer != "personal" {
		t.Errorf("Home[2].Layer = %q, want %q", home[2].Layer, "personal")
	}
}

// TestPartitionByScope_OnlyHome proves Home-only sources yield one Home partition in the given order and no System
// partition.
//
// Parameters:
//   - `t`: the test harness.
func TestPartitionByScope_OnlyHome(t *testing.T) {
	t.Helper()

	sources := []tree.LayerSource{
		{Layer: "base", Order: 0, ScopeName: "Home", TargetRoot: "/home/user", SourceRoot: "/repo/base/Home"},
		{Layer: "personal", Order: 2, ScopeName: "Home", TargetRoot: "/home/user", SourceRoot: "/repo/personal/Home"},
	}

	partitions := PartitionByScope(sources)

	if len(partitions) != 1 {
		t.Fatalf("got %d partitions, want 1", len(partitions))
	}

	home := partitions["Home"]
	if len(home) != 2 {
		t.Fatalf("Home partition has %d entries, want 2", len(home))
	}
	if home[0].Layer != "base" {
		t.Errorf("Home[0].Layer = %q, want %q", home[0].Layer, "base")
	}
	if home[1].Layer != "personal" {
		t.Errorf("Home[1].Layer = %q, want %q", home[1].Layer, "personal")
	}

	if _, ok := partitions["System"]; ok {
		t.Error("System partition should not exist when no System sources provided")
	}
}

// TestPartitionByScope_EmptySources proves a nil and an empty source list each yield an empty map.
//
// Parameters:
//   - `t`: the test harness.
func TestPartitionByScope_EmptySources(t *testing.T) {
	t.Helper()

	partitions := PartitionByScope(nil)

	if len(partitions) != 0 {
		t.Fatalf("got %d partitions, want 0", len(partitions))
	}

	partitions = PartitionByScope([]tree.LayerSource{})

	if len(partitions) != 0 {
		t.Fatalf("got %d partitions for empty slice, want 0", len(partitions))
	}
}

// --- ScopeHome, ScopeSystem ---

// TestScopeRoots_ReadWritScopes proves the scope roots come from `writ.scopes` (#925), keyed by the scope's name as
// the defaults document it; viper matches keys without case, so `Home` and `home` are one key.
//
// Parameters:
//   - `t`: the test harness.
func TestScopeRoots_ReadWritScopes(t *testing.T) {

	t.Cleanup(viper.Reset)
	home, system := t.TempDir(), t.TempDir()
	viper.Set("writ.scopes.Home", home)
	viper.Set("writ.scopes.System", system)

	if got := ScopeHome(); got != home {
		t.Errorf("ScopeHome() = %q, want %q from writ.scopes.Home", got, home)
	}
	if got := ScopeSystem(); got != system {
		t.Errorf("ScopeSystem() = %q, want %q from writ.scopes.System", got, system)
	}
}
