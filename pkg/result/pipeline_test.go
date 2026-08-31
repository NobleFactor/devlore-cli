// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package result_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/result"
	"github.com/NobleFactor/devlore-cli/pkg/sink"
)

// region Tests

// summary is a result shaped the way a command's is: Go names on the fields, `json:` tags naming what a
// customer sees -- in the documents, and on the Starlark surface.
type summary struct {
	UnitCount        int    `json:"unit_count"`
	ExpectationCount int    `json:"expectation_count"`
	State            string `json:"state"`
}

// TestPipeline_EmitNormalizesBeforeRendering pins stage 1.
//
// Every presentation is a presentation of the JSON, so a field carries its `json:` name in `list`, `table`,
// `csv`, and `value` exactly as it does in `json`. Before normalization moved to [Pipeline.Emit] only the jq
// filter normalized, so `-o list` printed UnitCount and `--jq . -o list` printed unit_count -- the same
// field named two ways depending on an unrelated flag.
func TestPipeline_EmitNormalizesBeforeRendering(t *testing.T) {

	for _, format := range []string{"list", "csv", "table"} {
		t.Run(format, func(t *testing.T) {

			var buffer bytes.Buffer
			formatter, err := result.FormatterByName(format)
			if err != nil {
				t.Fatalf("FormatterByName(%s): %v", format, err)
			}

			pipeline := result.NewPipeline(nil, formatter, sink.New(&buffer))
			if err := pipeline.Emit(summary{UnitCount: 1, ExpectationCount: 2, State: "ok"}); err != nil {
				t.Fatalf("Emit: %v", err)
			}

			got := strings.ToLower(buffer.String())
			if !strings.Contains(got, "unit_count") {
				t.Errorf("%s did not use the json name: %q", format, buffer.String())
			}
			if strings.Contains(buffer.String(), "UnitCount") {
				t.Errorf("%s leaked the Go field name: %q", format, buffer.String())
			}
		})
	}
}

// TestPipeline_EmitKeepsIntegerPrecision guards the one thing normalization must not do.
//
// Decoding to float64 rounds any integer past 2^53, which is the defect issue #712 records against the
// document codec. [normalize] decodes with UseNumber so a presenter renders the literal digits; gojq's
// conversion to int64/float64 stays inside the jq filter, where it is gojq's requirement and not ours.
func TestPipeline_EmitKeepsIntegerPrecision(t *testing.T) {

	const large = 9007199254740993 // 2^53 + 1, the first integer a float64 cannot hold

	var buffer bytes.Buffer
	pipeline := result.NewPipeline(nil, result.NewValueFormatter(), sink.New(&buffer))
	if err := pipeline.Emit(json.Number("9007199254740993")); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	if !strings.Contains(buffer.String(), "9007199254740993") {
		t.Errorf("integer lost precision: got %q, want the literal digits of %d", buffer.String(), large)
	}
}

// endregion
