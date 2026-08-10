// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package main

import "testing"

func TestMergeEntries_PreservesMetadata(t *testing.T) {
	existing := []PromptEntry{
		{Name: "a.md", Purpose: "original purpose", Description: "desc"},
	}
	files := []string{"a.md", "b.md"}

	result := mergeEntries(files, existing, func(n string) PromptEntry {
		return PromptEntry{Name: n}
	})

	if len(result) != 2 {
		t.Fatalf("len(result) = %d, want 2", len(result))
	}
	if result[0].Name != "a.md" {
		t.Errorf("result[0].ReceiverName = %q, want %q", result[0].Name, "a.md")
	}
	if result[0].Purpose != "original purpose" {
		t.Errorf("result[0].Purpose = %q, want %q", result[0].Purpose, "original purpose")
	}
	if result[0].Description != "desc" {
		t.Errorf("result[0].Description = %q, want %q", result[0].Description, "desc")
	}
	if result[1].Name != "b.md" {
		t.Errorf("result[1].ReceiverName = %q, want %q", result[1].Name, "b.md")
	}
	if result[1].Purpose != "" {
		t.Errorf("result[1].Purpose = %q, want empty (new entry)", result[1].Purpose)
	}
}

func TestMergeEntries_RemovesDeleted(t *testing.T) {
	existing := []PromptEntry{
		{Name: "a.md"},
		{Name: "b.md"},
	}
	files := []string{"a.md"} // b.md deleted

	result := mergeEntries(files, existing, func(n string) PromptEntry {
		return PromptEntry{Name: n}
	})

	if len(result) != 1 {
		t.Fatalf("len(result) = %d, want 1", len(result))
	}
	if result[0].Name != "a.md" {
		t.Errorf("result[0].ReceiverName = %q, want %q", result[0].Name, "a.md")
	}
}

func TestMergeEntries_OrderFollowsFiles(t *testing.T) {
	existing := []PromptEntry{
		{Name: "c.md", Purpose: "third"},
		{Name: "a.md", Purpose: "first"},
	}
	files := []string{"a.md", "b.md", "c.md"}

	result := mergeEntries(files, existing, func(n string) PromptEntry {
		return PromptEntry{Name: n}
	})

	if len(result) != 3 {
		t.Fatalf("len(result) = %d, want 3", len(result))
	}

	want := []string{"a.md", "b.md", "c.md"}
	for i, w := range want {
		if result[i].Name != w {
			t.Errorf("result[%d].ReceiverName = %q, want %q", i, result[i].Name, w)
		}
	}

	// Verify preserved metadata landed at the correct positions.
	if result[0].Purpose != "first" {
		t.Errorf("result[0].Purpose = %q, want %q", result[0].Purpose, "first")
	}
	if result[1].Purpose != "" {
		t.Errorf("result[1].Purpose = %q, want empty (new entry)", result[1].Purpose)
	}
	if result[2].Purpose != "third" {
		t.Errorf("result[2].Purpose = %q, want %q", result[2].Purpose, "third")
	}
}

func TestMergeEntries_AllTypes(t *testing.T) {
	files := []string{"existing.md", "new.md"}

	t.Run("PromptEntry", func(t *testing.T) {
		existing := []PromptEntry{{Name: "existing.md", Purpose: "kept"}}
		result := mergeEntries(files, existing, func(n string) PromptEntry {
			return PromptEntry{Name: n}
		})
		if len(result) != 2 {
			t.Fatalf("len = %d, want 2", len(result))
		}
		if result[0].Purpose != "kept" {
			t.Errorf("Purpose = %q, want %q", result[0].Purpose, "kept")
		}
	})

	t.Run("SchemaEntry", func(t *testing.T) {
		existing := []SchemaEntry{{Name: "existing.md", Purpose: "kept"}}
		result := mergeEntries(files, existing, func(n string) SchemaEntry {
			return SchemaEntry{Name: n}
		})
		if len(result) != 2 {
			t.Fatalf("len = %d, want 2", len(result))
		}
		if result[0].Purpose != "kept" {
			t.Errorf("Purpose = %q, want %q", result[0].Purpose, "kept")
		}
	})

	t.Run("ExampleEntry", func(t *testing.T) {
		existing := []ExampleEntry{{Name: "existing.md", Purpose: "kept"}}
		result := mergeEntries(files, existing, func(n string) ExampleEntry {
			return ExampleEntry{Name: n}
		})
		if len(result) != 2 {
			t.Fatalf("len = %d, want 2", len(result))
		}
		if result[0].Purpose != "kept" {
			t.Errorf("Purpose = %q, want %q", result[0].Purpose, "kept")
		}
	})

	t.Run("TransformEntry", func(t *testing.T) {
		existing := []TransformEntry{{Name: "existing.md", SourceSystem: "kept"}}
		result := mergeEntries(files, existing, func(n string) TransformEntry {
			return TransformEntry{Name: n}
		})
		if len(result) != 2 {
			t.Fatalf("len = %d, want 2", len(result))
		}
		if result[0].SourceSystem != "kept" {
			t.Errorf("SourceSystem = %q, want %q", result[0].SourceSystem, "kept")
		}
	})

	t.Run("SignatureEntry", func(t *testing.T) {
		existing := []SignatureEntry{{Name: "existing.md", System: "kept"}}
		result := mergeEntries(files, existing, func(n string) SignatureEntry {
			return SignatureEntry{Name: n}
		})
		if len(result) != 2 {
			t.Fatalf("len = %d, want 2", len(result))
		}
		if result[0].System != "kept" {
			t.Errorf("System = %q, want %q", result[0].System, "kept")
		}
	})

	t.Run("SlotEntry", func(t *testing.T) {
		existing := []SlotEntry{{Name: "existing.md", Purpose: "kept"}}
		result := mergeEntries(files, existing, func(n string) SlotEntry {
			return SlotEntry{Name: n}
		})
		if len(result) != 2 {
			t.Fatalf("len = %d, want 2", len(result))
		}
		if result[0].Purpose != "kept" {
			t.Errorf("Purpose = %q, want %q", result[0].Purpose, "kept")
		}
	})
}
