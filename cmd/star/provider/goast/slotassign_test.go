// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package goast

import "testing"

func TestAssignSlots_ExactMatch(t *testing.T) {
	slots := []string{"path", "suffix"}
	items := []string{"path: the file path", "suffix: the backup suffix"}

	matched, unSlots, unItems := assignSlots(slots, items)
	if len(matched) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matched))
	}
	if len(unSlots) != 0 {
		t.Errorf("expected 0 unmatched slots, got %v", unSlots)
	}
	if len(unItems) != 0 {
		t.Errorf("expected 0 unmatched items, got %v", unItems)
	}

	for _, m := range matched {
		if m.Score != 1.0 {
			t.Errorf("slot %q matched item %q with score %f, expected 1.0", m.Slot, m.Item, m.Score)
		}
		if m.Forced {
			t.Errorf("slot %q should not be forced", m.Slot)
		}
	}
}

func TestAssignSlots_FuzzyMatch(t *testing.T) {
	slots := []string{"backupSuffix"}
	items := []string{"bkupSuffix: the suffix"}

	matched, unSlots, unItems := assignSlots(slots, items)
	if len(matched) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matched))
	}
	if matched[0].Score <= 0.3 {
		t.Errorf("expected score > 0.3, got %f", matched[0].Score)
	}
	if len(unSlots) != 0 {
		t.Errorf("expected 0 unmatched slots, got %v", unSlots)
	}
	if len(unItems) != 0 {
		t.Errorf("expected 0 unmatched items, got %v", unItems)
	}
}

func TestAssignSlots_UnmatchedSlot(t *testing.T) {
	slots := []string{"path", "suffix"}
	items := []string{"path: the file path"}

	matched, unSlots, unItems := assignSlots(slots, items)
	if len(matched) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matched))
	}
	if len(unSlots) != 1 || unSlots[0] != "suffix" {
		t.Errorf("expected unmatched slot 'suffix', got %v", unSlots)
	}
	if len(unItems) != 0 {
		t.Errorf("expected 0 unmatched items, got %v", unItems)
	}
}

func TestAssignSlots_UnmatchedItem(t *testing.T) {
	slots := []string{"path"}
	items := []string{"path: the file path", "extra: not a param"}

	matched, unSlots, unItems := assignSlots(slots, items)
	if len(matched) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matched))
	}
	if len(unSlots) != 0 {
		t.Errorf("expected 0 unmatched slots, got %v", unSlots)
	}
	if len(unItems) != 1 {
		t.Errorf("expected 1 unmatched item, got %v", unItems)
	}
}

func TestAssignSlots_ForcedAssignment(t *testing.T) {
	// "path" matches exactly. "xyz" bears no resemblance to "suffix" — but it's the only pair left.
	slots := []string{"path", "suffix"}
	items := []string{"path: the file path", "xyz: something unrelated"}

	matched, unSlots, unItems := assignSlots(slots, items)

	if len(matched) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matched))
	}
	if len(unSlots) != 0 {
		t.Errorf("expected 0 unmatched slots, got %v", unSlots)
	}
	if len(unItems) != 0 {
		t.Errorf("expected 0 unmatched items, got %v", unItems)
	}

	// Find the forced match.
	forcedCount := 0
	for _, m := range matched {
		if m.Forced {
			forcedCount++
			t.Logf("forced: slot=%q item=%q score=%f", m.Slot, m.Item, m.Score)
		}
	}
	if forcedCount != 1 {
		t.Errorf("expected 1 forced match, got %d", forcedCount)
	}
}

func TestAssignSlots_NoMatch(t *testing.T) {
	slots := []string{"path"}
	items := []string{"xyz: completely unrelated"}

	matched, unSlots, unItems := assignSlots(slots, items)
	// Score for "xyz" vs "path" should be below 0.3, so no greedy match.
	// But 1:1 forced assignment kicks in.
	if len(matched) != 1 {
		t.Fatalf("expected 1 forced match, got %d", len(matched))
	}
	if !matched[0].Forced {
		t.Error("expected forced match")
	}
	if len(unSlots) != 0 {
		t.Errorf("expected 0 unmatched slots, got %v", unSlots)
	}
	if len(unItems) != 0 {
		t.Errorf("expected 0 unmatched items, got %v", unItems)
	}
}

func TestAssignSlots_MultipleNoMatch(t *testing.T) {
	// Two slots, two items, neither matches — no forced assignment (only works for 1:1).
	slots := []string{"path", "suffix"}
	items := []string{"xyz: unrelated", "abc: also unrelated"}

	matched, unSlots, unItems := assignSlots(slots, items)
	if len(matched) != 0 {
		t.Fatalf("expected 0 matches, got %d", len(matched))
	}
	if len(unSlots) != 2 {
		t.Errorf("expected 2 unmatched slots, got %v", unSlots)
	}
	if len(unItems) != 2 {
		t.Errorf("expected 2 unmatched items, got %v", unItems)
	}
}

func TestAssignSlots_Empty(t *testing.T) {
	matched, unSlots, unItems := assignSlots(nil, nil)
	if len(matched) != 0 || len(unSlots) != 0 || len(unItems) != 0 {
		t.Error("expected all empty for nil inputs")
	}
}

func TestAssignSlots_SlotsOnly(t *testing.T) {
	matched, unSlots, unItems := assignSlots([]string{"path", "name"}, nil)
	if len(matched) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matched))
	}
	if len(unSlots) != 2 {
		t.Errorf("expected 2 unmatched slots, got %v", unSlots)
	}
	if len(unItems) != 0 {
		t.Errorf("expected 0 unmatched items, got %v", unItems)
	}
}

func TestAssignSlots_ItemsOnly(t *testing.T) {
	matched, unSlots, unItems := assignSlots(nil, []string{"path: desc", "name: desc"})
	if len(matched) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matched))
	}
	if len(unSlots) != 0 {
		t.Errorf("expected 0 unmatched slots, got %v", unSlots)
	}
	if len(unItems) != 2 {
		t.Errorf("expected 2 unmatched items, got %v", unItems)
	}
}

func TestAssignSlots_ReorderedParams(t *testing.T) {
	slots := []string{"path", "suffix", "mode"}
	items := []string{"mode: the mode", "path: the path", "suffix: the suffix"}

	matched, unSlots, unItems := assignSlots(slots, items)
	if len(matched) != 3 {
		t.Fatalf("expected 3 matches, got %d", len(matched))
	}
	if len(unSlots) != 0 || len(unItems) != 0 {
		t.Errorf("expected no unmatched, got slots=%v items=%v", unSlots, unItems)
	}

	// Verify correct assignment despite reordering.
	for _, m := range matched {
		if m.Score != 1.0 {
			t.Errorf("slot %q matched item %q with score %f, expected 1.0", m.Slot, m.Item, m.Score)
		}
	}
}

func TestAssignSlots_SynonymHeading(t *testing.T) {
	// "Params" should match slot "parameters" via synonym.
	slots := []string{"parameters"}
	items := []string{"Params: the parameters"}

	matched, _, _ := assignSlots(slots, items)
	if len(matched) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matched))
	}
	if matched[0].Score < 0.9 {
		t.Errorf("expected high score for synonym match, got %f", matched[0].Score)
	}
}
