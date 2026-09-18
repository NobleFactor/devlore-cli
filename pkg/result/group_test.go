// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package result_test

import (
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/result"
)

// region Tests

// TestGroup_EveryRenderingAnswers walks the registry.
//
// A rendering without a group is one the pager cannot classify, and the compiler cannot catch that alone: every
// formatter satisfies the interface, so a wrong answer is the failure mode, not a missing one.
func TestGroup_EveryRenderingAnswers(t *testing.T) {

	for name, want := range map[string]result.Group{
		"csv":            result.GroupSerialized,
		"json":           result.GroupSerialized,
		"yaml":           result.GroupSerialized,
		"list":           result.GroupRecords,
		"table":          result.GroupRecords,
		"markdown":       result.GroupDocument,
		"terminal":       result.GroupDocument,
		"value":          result.GroupComposed,
		"template={{.}}": result.GroupComposed,
		"none":           result.GroupNothing,
	} {
		t.Run(name, func(t *testing.T) {
			formatter, err := result.FormatterByName(name)
			if err != nil {
				t.Fatalf("FormatterByName(%q): %v", name, err)
			}
			if got := formatter.Group(); got != want {
				t.Errorf("group of %q = %q, want %q", name, got, want)
			}
		})
	}
}

// TestGroup_OneTypeAnswersForBothItsPresets is the case a name-to-group table would have gotten wrong.
//
// `csv` and `value` are both [result.DelimitedFormatter]; the raw preset is the shape a caller composed for a
// pipe, the quoted one is a format a library reads back.
func TestGroup_OneTypeAnswersForBothItsPresets(t *testing.T) {

	if got := result.NewCSVFormatter().Group(); got != result.GroupSerialized {
		t.Errorf("csv preset = %q, want %q", got, result.GroupSerialized)
	}
	if got := result.NewValueFormatter().Group(); got != result.GroupComposed {
		t.Errorf("value preset = %q, want %q", got, result.GroupComposed)
	}
}

// TestGroup_TheGroupsPartitionTheRegistry pins that the set of groups and the set of names stay in step.
//
// Every group is used by at least one rendering, and every rendering lands in exactly one group. A group with
// no members is a heading with nothing under it; a rendering in no group is one the pager cannot classify.
func TestGroup_TheGroupsPartitionTheRegistry(t *testing.T) {

	names := []string{"csv", "json", "list", "markdown", "none", "table", "template={{.}}", "terminal", "value", "yaml"}
	groups := []result.Group{
		result.GroupComposed,
		result.GroupDocument,
		result.GroupNothing,
		result.GroupRecords,
		result.GroupSerialized,
	}

	members := map[result.Group]int{}
	for _, name := range names {
		formatter, err := result.FormatterByName(name)
		if err != nil {
			t.Fatalf("FormatterByName(%q): %v", name, err)
		}
		members[formatter.Group()]++
	}

	if len(members) != len(groups) {
		t.Errorf("the renderings fall into %d groups, want %d: %v", len(members), len(groups), members)
	}
	for _, group := range groups {
		if members[group] == 0 {
			t.Errorf("group %q has no renderings", group)
		}
	}
}

// endregion
