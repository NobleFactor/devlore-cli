// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package result_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/result"
)

// region Tests

// TestListFormatter_AlignsKeysWithinARecord pins the S3 rule.
func TestListFormatter_AlignsKeysWithinARecord(t *testing.T) {

	var buffer bytes.Buffer
	record := map[string]any{"name": "x", "state": "active"}
	if err := result.NewListFormatter().Format(record, &buffer); err != nil {
		t.Fatalf("Format: %v", err)
	}

	want := "name  : x\nstate : active\n"
	if buffer.String() != want {
		t.Errorf("got %q, want %q", buffer.String(), want)
	}
}

// TestListFormatter_EachRecordKeepsItsOwnKeys is why this formatter exists.
//
// The delimited formats union keys across the stream and leave holes; a heterogeneous stream renders mostly
// holes. Here each record shows only what it has, and pads to its own widest key rather than the stream's.
func TestListFormatter_EachRecordKeepsItsOwnKeys(t *testing.T) {

	var buffer bytes.Buffer
	stream := []any{
		map[string]any{"name": "x", "state": "active"},
		map[string]any{"kind": "package", "action": "install"},
	}
	if err := result.NewListFormatter().Format(stream, &buffer); err != nil {
		t.Fatalf("Format: %v", err)
	}

	want := "name  : x\nstate : active\n\naction : install\nkind   : package\n"
	if buffer.String() != want {
		t.Errorf("got %q, want %q", buffer.String(), want)
	}
}

// TestListFormatter_ANestedValueIsCompactJSON pins the S4 rule at the list seam.
func TestListFormatter_ANestedValueIsCompactJSON(t *testing.T) {

	var buffer bytes.Buffer
	record := map[string]any{"name": "x", "health": map[string]any{"runs": 3}}
	if err := result.NewListFormatter().Format(record, &buffer); err != nil {
		t.Fatalf("Format: %v", err)
	}

	if !strings.Contains(buffer.String(), `health : {"runs":3}`) {
		t.Errorf("nested value did not render as compact JSON: %q", buffer.String())
	}
}

// TestListFormatter_AScalarCarriesNoKey covers S1, which is what makes `--jq '.name' -o list` useful.
func TestListFormatter_AScalarCarriesNoKey(t *testing.T) {

	var buffer bytes.Buffer
	if err := result.NewListFormatter().Format("active", &buffer); err != nil {
		t.Fatalf("Format: %v", err)
	}

	if buffer.String() != "active\n" {
		t.Errorf("got %q, want %q", buffer.String(), "active\n")
	}
}

// TestListFormatter_EmptyEmitsNothing covers S8.
func TestListFormatter_EmptyEmitsNothing(t *testing.T) {

	var buffer bytes.Buffer
	if err := result.NewListFormatter().Format([]any{}, &buffer); err != nil {
		t.Fatalf("Format: %v", err)
	}

	if buffer.String() != "" {
		t.Errorf("got %q, want empty", buffer.String())
	}
}

// TestDelimitedFormatter_ANestedCellIsCompactJSON pins the S4 rule at the delimited seam.
//
// Before this rule a nested cell went through fmt.Sprint and rendered as Go's own `map[runs:3]` -- a
// notation that names the language rather than the data, and that nothing downstream can parse.
func TestDelimitedFormatter_ANestedCellIsCompactJSON(t *testing.T) {

	var buffer bytes.Buffer
	rows := []map[string]any{{"name": "x", "health": map[string]any{"runs": 3}}}
	if err := result.NewCSVFormatter().Format(rows, &buffer); err != nil {
		t.Fatalf("Format: %v", err)
	}

	if !strings.Contains(buffer.String(), `{""runs"":3}`) {
		t.Errorf("nested cell did not render as compact JSON: %q", buffer.String())
	}
}

// TestDelimitedFormatter_ASelfRenderingCellKeepsItsOwnForm guards the exclusion the compact-JSON rule needs.
//
// A [time.Time] is a struct, and a type whose String method IS its presentation is a struct too. Sending
// either through the JSON encoder would replace a form the type defines for itself -- and for a type whose
// fields are unexported, replace it with "{}".
func TestDelimitedFormatter_ASelfRenderingCellKeepsItsOwnForm(t *testing.T) {

	var buffer bytes.Buffer
	rows := []map[string]any{{"at": time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)}}
	if err := result.NewCSVFormatter().Format(rows, &buffer); err != nil {
		t.Fatalf("Format: %v", err)
	}

	if strings.Contains(buffer.String(), "wall") || strings.Contains(buffer.String(), "{") {
		t.Errorf("time.Time was JSON-encoded rather than rendering itself: %q", buffer.String())
	}
	if !strings.Contains(buffer.String(), "2026-08-31") {
		t.Errorf("time.Time did not render its own form: %q", buffer.String())
	}
}

// endregion
