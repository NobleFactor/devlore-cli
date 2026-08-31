// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package result_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/result"
)

// region Fixture

type row struct {
	Name    string `csv:"name"`
	Version string `csv:"version"`
}

func rows() []row {
	return []row{{Name: "alpha", Version: "1.0"}, {Name: "beta", Version: "2.0"}}
}

// endregion

// region Tests

// TestCSVPreset_SpreadsheetShape pins the comma-and-heading preset.
//
// A spreadsheet wants the header row: it becomes the column names on open.
func TestCSVPreset_SpreadsheetShape(t *testing.T) {

	var buffer bytes.Buffer
	if err := result.NewCSVFormatter().Format(rows(), &buffer); err != nil {
		t.Fatalf("Format: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buffer.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3 (header + two rows): %q", len(lines), buffer.String())
	}
	if lines[0] != "name,version" {
		t.Errorf("header = %q, want %q", lines[0], "name,version")
	}
	if lines[1] != "alpha,1.0" {
		t.Errorf("first row = %q, want %q", lines[1], "alpha,1.0")
	}
}

// TestValuePreset_PipelineShape pins the tab-and-no-heading preset.
//
// `awk '{print $2}'` should work on line one. A header row would be one more line every consumer has to
// skip, which is why the two presets differ in delimiter AND headings rather than only the delimiter.
func TestValuePreset_PipelineShape(t *testing.T) {

	var buffer bytes.Buffer
	if err := result.NewValueFormatter().Format(rows(), &buffer); err != nil {
		t.Fatalf("Format: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buffer.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2 (no header): %q", len(lines), buffer.String())
	}
	if lines[0] != "alpha\t1.0" {
		t.Errorf("first row = %q, want %q", lines[0], "alpha\t1.0")
	}

	// The property the preset exists for: field two is reachable by position, with no parser.
	if got := strings.Split(lines[0], "\t")[1]; got != "1.0" {
		t.Errorf("field 2 = %q, want %q", got, "1.0")
	}
}

// TestFormatterByName_BothPresetsRegister covers the selection seam.
func TestFormatterByName_BothPresetsRegister(t *testing.T) {

	for name, wantSeparator := range map[string]string{"csv": ",", "value": "\t"} {
		formatter, err := result.FormatterByName(name)
		if err != nil {
			t.Fatalf("FormatterByName(%s): %v", name, err)
		}

		var buffer bytes.Buffer
		if err := formatter.Format(rows(), &buffer); err != nil {
			t.Fatalf("Format(%s): %v", name, err)
		}
		if !strings.Contains(buffer.String(), "alpha"+wantSeparator+"1.0") {
			t.Errorf("%s did not separate with %q: %q", name, wantSeparator, buffer.String())
		}
	}
}

// endregion
