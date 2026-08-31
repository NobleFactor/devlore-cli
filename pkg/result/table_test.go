// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package result_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/result"
)

// region Tests

// TestTableFormatter_AlignsColumns is the rendering the format exists for.
func TestTableFormatter_AlignsColumns(t *testing.T) {

	var buffer bytes.Buffer
	rows := []struct {
		Name    string `csv:"name"`
		Version string `csv:"version"`
	}{{"alpha", "1.0"}, {"a-much-longer-name", "2.0"}}

	if err := result.NewTableFormatter().Format(rows, &buffer); err != nil {
		t.Fatalf("Format: %v", err)
	}

	lines := strings.Split(strings.TrimRight(buffer.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3 (header + two rows): %q", len(lines), buffer.String())
	}
	if !strings.HasPrefix(lines[0], "NAME") {
		t.Errorf("header = %q, want it to start with the upper-cased column name", lines[0])
	}

	// The property alignment means: the second column starts at the same offset on every row.
	offset := strings.Index(lines[0], "VERSION")
	for i, line := range lines[1:] {
		if strings.Index(line, "1.0") != offset && strings.Index(line, "2.0") != offset {
			t.Errorf("row %d does not align its second column at %d: %q", i, offset, line)
		}
	}
}

// TestTableFormatter_AlignsPastMultiByteCharacters is #741's defect, at the shared formatter.
//
// A hand-rolled `%-30s` pads by BYTE count, so a row containing a multi-byte character is short by the
// difference and every column after it shifts. tabwriter measures in runes, so the columns hold.
func TestTableFormatter_AlignsPastMultiByteCharacters(t *testing.T) {

	var buffer bytes.Buffer
	rows := []struct {
		Name  string `csv:"name"`
		State string `csv:"state"`
	}{
		{"ascii-name", "current"},
		{"日本語テキスト", "drifted"}, // 7 runes, 21 bytes
	}

	if err := result.NewTableFormatter().Format(rows, &buffer); err != nil {
		t.Fatalf("Format: %v", err)
	}

	lines := strings.Split(strings.TrimRight(buffer.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3: %q", len(lines), buffer.String())
	}

	// Measured in runes, as a terminal measures. A byte-counting formatter fails this: the second row's
	// name is 7 runes but 21 bytes, so byte padding would leave its state column 14 columns adrift.
	firstState := strings.Index(lines[1], "current")
	secondState := strings.Index(lines[2], "drifted")
	if firstState < 0 || secondState < 0 {
		t.Fatalf("state column missing: %q", buffer.String())
	}

	firstOffset := len([]rune(lines[1][:firstState]))
	secondOffset := len([]rune(lines[2][:secondState]))
	if firstOffset != secondOffset {
		t.Errorf("the state column starts at rune %d and %d:\n  %q\n  %q",
			firstOffset, secondOffset, lines[1], lines[2])
	}
}

// TestTableFormatter_SharesColumnInferenceWithCSV is why one formatter family rather than two.
//
// The same value must produce the same columns whichever presentation is asked for; only the separator or
// the padding differs. Divergent column logic is how one rendering ends up showing a field the other hides.
func TestTableFormatter_SharesColumnInferenceWithCSV(t *testing.T) {

	rows := []struct {
		Name   string `csv:"name"`
		Hidden string `csv:"-"`
		Extra  string `csv:"extra"`
	}{{"alpha", "secret", "x"}}

	var table, csv bytes.Buffer
	if err := result.NewTableFormatter().Format(rows, &table); err != nil {
		t.Fatalf("table: %v", err)
	}
	if err := result.NewCSVFormatter().Format(rows, &csv); err != nil {
		t.Fatalf("csv: %v", err)
	}

	if strings.Contains(table.String(), "secret") {
		t.Error(`table rendered a csv:"-" field; the tag must be honored by both`)
	}
	for _, column := range []string{"NAME", "EXTRA"} {
		if !strings.Contains(table.String(), column) {
			t.Errorf("table is missing column %q: %q", column, table.String())
		}
	}
	if !strings.Contains(csv.String(), "name,extra") {
		t.Errorf("csv headers = %q, want name,extra", csv.String())
	}
}

// TestFormatterByName_TableRegisters covers the selection seam.
func TestFormatterByName_TableRegisters(t *testing.T) {

	formatter, err := result.FormatterByName("table")
	if err != nil {
		t.Fatalf("FormatterByName(table): %v", err)
	}
	if _, ok := formatter.(result.TableFormatter); !ok {
		t.Errorf("FormatterByName(table) = %T, want result.TableFormatter", formatter)
	}
}

// endregion
