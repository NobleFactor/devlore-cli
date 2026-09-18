// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/result"
)

// region Tests

// TestOutputUsage_CarriesEveryGroupAsAHeading is the help half of the grouping.
//
// cobra writes a flag's usage verbatim into the generated man page, so a heading missing here is a heading
// missing from the man pages of all four programs.
func TestOutputUsage_CarriesEveryGroupAsAHeading(t *testing.T) {

	for _, group := range []result.Group{
		result.GroupComposed,
		result.GroupDocument,
		result.GroupNothing,
		result.GroupRecords,
		result.GroupSerialized,
	} {
		heading := "\n" + string(group)
		if !strings.Contains(outputUsage, heading) {
			t.Errorf("--output help has no heading for group %q:\n%s", group, outputUsage)
		}
	}
}

// TestOutputUsage_NamesEveryRenderingUnderAGroup pins that the help and the registry cannot drift.
//
// Every name [result.FormatterByName] accepts is listed, indented under the heading of the group its formatter
// reports. A name the help omits is a rendering a user cannot find.
func TestOutputUsage_NamesEveryRenderingUnderAGroup(t *testing.T) {

	groups := map[string]bool{}
	for _, group := range []result.Group{
		result.GroupComposed,
		result.GroupDocument,
		result.GroupNothing,
		result.GroupRecords,
		result.GroupSerialized,
	} {
		groups[string(group)] = true
	}

	// The heading in force when each line is read. Membership is what is asserted, never the layout: the
	// entries have been a two-column list, a two-line list, and a definition list, and the grouping is the
	// invariant under all three.
	heading := map[string]string{}
	current := ""
	for _, line := range strings.Split(outputUsage, "\n") {
		trimmed := strings.TrimSpace(line)
		name, _, _ := strings.Cut(trimmed, " ")
		if groups[name] {
			current = name
			continue
		}
		if trimmed != "" && current != "" {
			heading[strings.TrimSuffix(name, ":")] = current
		}
	}

	for name, spec := range map[string]string{
		"csv":           "csv",
		"json":          "json",
		"list":          "list",
		"markdown":      "markdown",
		"none":          "none",
		"table":         "table",
		"template=BODY": "template={{.}}",
		"terminal":      "terminal",
		"value":         "value",
		"yaml":          "yaml",
	} {
		t.Run(name, func(t *testing.T) {

			formatter, err := result.FormatterByName(spec)
			if err != nil {
				t.Fatalf("FormatterByName(%q): %v", spec, err)
			}

			want := string(formatter.Group())
			switch got, listed := heading[name]; {
			case !listed:
				t.Errorf("%q is not listed in the --output help:\n%s", name, outputUsage)
			case got != want:
				t.Errorf("%q is listed under %q, want %q", name, got, want)
			}
		})
	}
}

// TestOutputUsageMan_NamesEveryRendering keeps the two usage texts from drifting.
//
// The man page shows [outputUsageMan] and a terminal shows [outputUsage]; they are one set of renderings in two
// layouts, so a rendering added to one and forgotten in the other is a defect.
func TestOutputUsageMan_NamesEveryRendering(t *testing.T) {

	for _, name := range []string{
		"csv", "json", "list", "markdown", "none", "table", "template=BODY", "terminal", "value", "yaml",
	} {
		if !strings.Contains(outputUsageMan, "**"+name+"**") {
			t.Errorf("the man usage does not name %q:\n%s", name, outputUsageMan)
		}
	}
}

// endregion
