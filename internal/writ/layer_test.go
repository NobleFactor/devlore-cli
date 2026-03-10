// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package writ

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/internal/writ/tree"
)

// --- PartitionByScope ---

func TestPartitionByScope_MixedSystemHome(t *testing.T) {
	t.Helper()

	sources := []tree.LayerSource{
		{Layer: "base", Order: 0, TargetName: "System", TargetRoot: "/", SourceRoot: "/repo/base/System"},
		{Layer: "base", Order: 0, TargetName: "Home", TargetRoot: "/home/user", SourceRoot: "/repo/base/Home"},
		{Layer: "team", Order: 1, TargetName: "System", TargetRoot: "/", SourceRoot: "/repo/team/System"},
		{Layer: "team", Order: 1, TargetName: "Home", TargetRoot: "/home/user", SourceRoot: "/repo/team/Home"},
		{Layer: "personal", Order: 2, TargetName: "Home", TargetRoot: "/home/user", SourceRoot: "/repo/personal/Home"},
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

func TestPartitionByScope_OnlyHome(t *testing.T) {
	t.Helper()

	sources := []tree.LayerSource{
		{Layer: "base", Order: 0, TargetName: "Home", TargetRoot: "/home/user", SourceRoot: "/repo/base/Home"},
		{Layer: "personal", Order: 2, TargetName: "Home", TargetRoot: "/home/user", SourceRoot: "/repo/personal/Home"},
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
