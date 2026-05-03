// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package result

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
)

// region slice-of-structs

func TestCSVFormatterRendersSliceOfStructs(t *testing.T) {

	type point struct {
		X     int
		Y     int
		Label string
	}

	rows := []point{
		{X: 1, Y: 2, Label: "a"},
		{X: 3, Y: 4, Label: "b"},
	}

	got := mustFormatCSV(t, rows)

	wantHeader := "X,Y,Label"
	wantRow1 := "1,2,a"
	wantRow2 := "3,4,b"

	for _, want := range []string{wantHeader, wantRow1, wantRow2} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, got)
		}
	}
}

func TestCSVFormatterHonorsTagOverrideAndSkip(t *testing.T) {

	type row struct {
		Name   string `csv:"name"`
		Hidden string `csv:"-"`
		Public int    // no tag — uses field name
	}

	got := mustFormatCSV(t, []row{{Name: "x", Hidden: "secret", Public: 7}})

	if !strings.Contains(got, "name,Public") {
		t.Errorf("expected header %q in output, got %q", "name,Public", got)
	}
	if strings.Contains(got, "Hidden") {
		t.Errorf("Hidden column should be omitted (csv:\"-\"); output: %q", got)
	}
	if strings.Contains(got, "secret") {
		t.Errorf("Hidden value 'secret' leaked to output: %q", got)
	}
	if !strings.Contains(got, "x,7") {
		t.Errorf("expected data row 'x,7' in output: %q", got)
	}
}

func TestCSVFormatterSkipsUnexportedFields(t *testing.T) {

	type row struct {
		Public  string
		private string //nolint:unused // exercising unexported-skip
	}

	got := mustFormatCSV(t, []row{{Public: "p", private: "x"}})

	if strings.Contains(got, "private") {
		t.Errorf("unexported field leaked: %q", got)
	}
	if !strings.Contains(got, "Public") {
		t.Errorf("expected Public column: %q", got)
	}
}

// endregion

// region slice-of-maps

func TestCSVFormatterRendersSliceOfMaps(t *testing.T) {

	rows := []map[string]any{
		{"a": 1, "b": 2},
		{"a": 3, "c": 4},
	}

	got := mustFormatCSV(t, rows)

	// Headers are the union of keys, sorted alphabetically.
	if !strings.Contains(got, "a,b,c") {
		t.Errorf("expected sorted-union header 'a,b,c'; got %q", got)
	}
	// Row 1 has b but not c.
	if !strings.Contains(got, "1,2,") {
		t.Errorf("row 1 should have '1,2,' (empty c); got %q", got)
	}
	// Row 2 has c but not b.
	if !strings.Contains(got, "3,,4") {
		t.Errorf("row 2 should have '3,,4' (empty b); got %q", got)
	}
}

// endregion

// region HasHeaders opt-in

type tableWithHeaders []map[string]any

func (tableWithHeaders) Headers() []string { return []string{"c", "a", "b"} }

func TestCSVFormatterHonorsHasHeaders(t *testing.T) {

	rows := tableWithHeaders{
		{"a": 1, "b": 2, "c": 3},
	}

	got := mustFormatCSV(t, rows)

	// Headers() takes priority over alphabetical sort.
	wantHeader := "c,a,b"
	if !strings.Contains(got, wantHeader) {
		t.Errorf("expected header %q; got %q", wantHeader, got)
	}
	if !strings.Contains(got, "3,1,2") {
		t.Errorf("expected row '3,1,2' in headers order; got %q", got)
	}
}

// endregion

// region RFC 4180 quoting

func TestCSVFormatterQuotesCommas(t *testing.T) {

	rows := []map[string]any{{"v": "a,b,c"}}
	got := mustFormatCSV(t, rows)

	if !strings.Contains(got, `"a,b,c"`) {
		t.Errorf("comma-bearing value not quoted; got %q", got)
	}
}

func TestCSVFormatterEscapesEmbeddedQuotes(t *testing.T) {

	rows := []map[string]any{{"v": `say "hi"`}}
	got := mustFormatCSV(t, rows)

	// RFC 4180: embedded quotes are doubled inside a quoted field.
	if !strings.Contains(got, `"say ""hi"""`) {
		t.Errorf("embedded quotes not escaped; got %q", got)
	}
}

func TestCSVFormatterQuotesNewlines(t *testing.T) {

	rows := []map[string]any{{"v": "line1\nline2"}}
	got := mustFormatCSV(t, rows)

	// A newline-bearing field is quoted, with the literal newline inside the quotes.
	if !strings.Contains(got, `"line1`+"\n"+`line2"`) {
		t.Errorf("newline-bearing value not quoted properly; got %q", got)
	}
}

// endregion

// region edge cases

func TestCSVFormatterEmptySliceProducesNoOutput(t *testing.T) {

	var buf bytes.Buffer
	if err := (CSVFormatter{}).Format([]map[string]any{}, &buf); err != nil {
		t.Fatalf("Format: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("empty slice should render zero bytes; got %q", buf.String())
	}
}

func TestCSVFormatterNilProducesNoOutput(t *testing.T) {

	var buf bytes.Buffer
	if err := (CSVFormatter{}).Format(nil, &buf); err != nil {
		t.Fatalf("Format: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("nil should render zero bytes; got %q", buf.String())
	}
}

func TestCSVFormatterRejectsScalar(t *testing.T) {

	var buf bytes.Buffer
	err := (CSVFormatter{}).Format(42, &buf)
	if err == nil {
		t.Fatal("expected error for scalar input; got nil")
	}
	if !strings.Contains(err.Error(), "expected slice or array") {
		t.Errorf("error text = %q, want substring 'expected slice or array'", err.Error())
	}
}

func TestCSVFormatterRejectsSliceOfScalars(t *testing.T) {

	var buf bytes.Buffer
	err := (CSVFormatter{}).Format([]int{1, 2, 3}, &buf)
	if err == nil {
		t.Fatal("expected error for slice of scalars; got nil")
	}
	if !strings.Contains(err.Error(), "not struct or map") {
		t.Errorf("error text = %q, want substring 'not struct or map'", err.Error())
	}
}

func TestCSVFormatterAcceptsArrayOfStructs(t *testing.T) {

	type row struct{ A, B int }
	got := mustFormatCSV(t, [2]row{{A: 1, B: 2}, {A: 3, B: 4}})

	if !strings.Contains(got, "A,B") {
		t.Errorf("array header missing: %q", got)
	}
	if !strings.Contains(got, "1,2") || !strings.Contains(got, "3,4") {
		t.Errorf("array rows missing: %q", got)
	}
}

func TestCSVFormatterRoundTripsThroughCsvReader(t *testing.T) {

	type row struct {
		Name string `csv:"name"`
		Note string `csv:"note"`
	}
	in := []row{
		{Name: "alice", Note: "has, comma"},
		{Name: "bob", Note: `has "quote"`},
	}

	var buf bytes.Buffer
	if err := (CSVFormatter{}).Format(in, &buf); err != nil {
		t.Fatalf("Format: %v", err)
	}

	reader := csv.NewReader(&buf)
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("csv.ReadAll: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("got %d records (incl. header), want 3", len(records))
	}
	if records[0][0] != "name" || records[0][1] != "note" {
		t.Errorf("header = %v, want [name note]", records[0])
	}
	if records[1][1] != "has, comma" {
		t.Errorf("row 1 col 1 = %q, want %q", records[1][1], "has, comma")
	}
	if records[2][1] != `has "quote"` {
		t.Errorf("row 2 col 1 = %q, want %q", records[2][1], `has "quote"`)
	}
}

// endregion

// region pointer + Stringer

type stringerCell struct{ raw string }

func (s stringerCell) String() string { return "<" + s.raw + ">" }

func TestCSVFormatterRendersStringerCells(t *testing.T) {

	type row struct {
		V stringerCell
	}
	got := mustFormatCSV(t, []row{{V: stringerCell{raw: "x"}}})

	if !strings.Contains(got, "<x>") {
		t.Errorf("Stringer not honored in cell rendering; got %q", got)
	}
}

func TestCSVFormatterDereferencesPointerSlice(t *testing.T) {

	type row struct{ A int }
	rows := []*row{{A: 1}, {A: 2}}

	got := mustFormatCSV(t, rows)

	if !strings.Contains(got, "A") || !strings.Contains(got, "1") || !strings.Contains(got, "2") {
		t.Errorf("pointer-element slice rendered wrong: %q", got)
	}
}

// endregion

// region helpers

func mustFormatCSV(t *testing.T, value any) string {

	t.Helper()
	var buf bytes.Buffer
	if err := (CSVFormatter{}).Format(value, &buf); err != nil {
		t.Fatalf("Format: %v", err)
	}
	return buf.String()
}

// endregion
