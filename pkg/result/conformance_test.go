// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package result_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/result"
	"github.com/NobleFactor/devlore-cli/pkg/sink"
)

// region Fixture

// deployment is the conformance fixture: one Go type carrying every property the formatting rules turn on.
//
//   - `json:` tags that differ from the Go field names, so stage 1 is observable. These are the names the
//     Starlark surface shows a customer, and they are what every presentation must use.
//   - a nested object and an array, so the compact-JSON cell rule is observable.
//   - an integer past 2^53, so a float64 round trip is observable as lost digits.
//   - a bool and an empty string, so the difference between "false" and "" is observable.
//
// Every formatter is exercised against this one value. A formatter that disagrees with the others about a
// field's NAME, or about what a nested value looks like, fails here rather than in a command months later.
type deployment struct {
	LayerName string         `json:"layer_name"`
	FileCount int64          `json:"file_count"`
	Active    bool           `json:"active"`
	Note      string         `json:"note"`
	Health    map[string]any `json:"health"`
	Tags      []string       `json:"tags"`
}

// fixture returns the canonical record. Kept as a function so no test can mutate another's input.
func fixture() deployment {
	return deployment{
		LayerName: "personal",
		FileCount: 9007199254740993, // 2^53 + 1: the first integer a float64 cannot hold
		Active:    true,
		Note:      "",
		Health:    map[string]any{"runs": 3},
		Tags:      []string{"a", "b"},
	}
}

// emit runs the fixture through a real pipeline, which is the only path that applies stage 1.
func emit(t *testing.T, format string, value any) string {
	t.Helper()

	formatter, err := result.FormatterByName(format)
	if err != nil {
		t.Fatalf("FormatterByName(%q): %v", format, err)
	}

	var buffer bytes.Buffer
	if err := result.NewPipeline(nil, formatter, sink.New(&buffer)).Emit(value); err != nil {
		t.Fatalf("Emit through %q: %v", format, err)
	}
	return buffer.String()
}

// endregion

// region Tests

// TestConformance_EveryFormatterUsesTheJSONNames is the cross-formatter invariant.
//
// `none` is excluded because it emits nothing by definition, and `template` because its output is whatever
// the caller's template says. Every other formatter presents the same JSON and must agree on the names.
func TestConformance_EveryFormatterUsesTheJSONNames(t *testing.T) {

	for _, format := range []string{"csv", "json", "list", "table", "value", "yaml"} {
		t.Run(format, func(t *testing.T) {

			got := emit(t, format, fixture())

			// Case-insensitive: `table` upper-cases its headers, matching aws and kubectl. What matters is
			// the separator -- "layer_name" is the json tag, "layername" is the Go identifier flattened.
			folded := strings.ToLower(got)

			// `value` is raw and headerless by design: it carries the values, never the names.
			if format != "value" && !strings.Contains(folded, "layer_name") {
				t.Errorf("missing the json name %q:\n%s", "layer_name", got)
			}
			if strings.Contains(folded, "layername") {
				t.Errorf("leaked the Go field name %q:\n%s", "LayerName", got)
			}
		})
	}
}

// TestConformance_EveryFormatterKeepsIntegerPrecision pins the rule that normalization must not round.
//
// Decoding to float64 renders 9007199254740993 as 9007199254740992 -- a wrong answer that looks right, and
// the defect issue #712 records against the document codec.
func TestConformance_EveryFormatterKeepsIntegerPrecision(t *testing.T) {

	for _, format := range []string{"csv", "json", "list", "table", "value", "yaml"} {
		t.Run(format, func(t *testing.T) {

			got := emit(t, format, fixture())

			if !strings.Contains(got, "9007199254740993") {
				t.Errorf("integer was rounded:\n%s", got)
			}
		})
	}
}

// TestConformance_NestedValuesAreCompactJSON pins the S4 rule across the presenters that lay data out.
//
// `json` and `yaml` render structure natively and are excluded; `value` is raw and carries the same cell
// text as `csv`.
func TestConformance_NestedValuesAreCompactJSON(t *testing.T) {

	for _, format := range []string{"csv", "list", "table", "value"} {
		t.Run(format, func(t *testing.T) {

			got := emit(t, format, fixture())

			if !strings.Contains(got, `{"runs":3}`) && !strings.Contains(got, `{""runs"":3}`) {
				t.Errorf("nested object did not render as compact JSON:\n%s", got)
			}
			if strings.Contains(got, "map[runs:3]") {
				t.Errorf("nested object rendered as Go's own notation:\n%s", got)
			}
		})
	}
}

// TestConformance_NoneEmitsNothing pins the one formatter whose contract is silence.
func TestConformance_NoneEmitsNothing(t *testing.T) {

	if got := emit(t, "none", fixture()); got != "" {
		t.Errorf("none emitted %q, want empty", got)
	}
}

// TestConformance_ASingleRecordIsOneRow pins S3 for the presenters that lay data out.
//
// A lone object is a sequence of one, not one cell holding the whole object and not an error. `table`
// rejected it outright until the rules were written down, and `csv` rendered the whole record into a single
// cell -- two different wrong answers to a shape a command produces whenever it reports on one thing.
func TestConformance_ASingleRecordIsOneRow(t *testing.T) {

	for _, format := range []string{"csv", "table"} {
		t.Run(format, func(t *testing.T) {

			lines := strings.Split(strings.TrimSpace(emit(t, format, fixture())), "\n")

			if len(lines) != 2 {
				t.Fatalf("got %d lines, want 2 (a header and one row):\n%s", len(lines), strings.Join(lines, "\n"))
			}
			if !strings.Contains(strings.ToLower(lines[0]), "layer_name") {
				t.Errorf("line 1 is not a header: %q", lines[0])
			}
			if !strings.Contains(lines[1], "personal") {
				t.Errorf("line 2 is not the record: %q", lines[1])
			}
		})
	}
}

// endregion
