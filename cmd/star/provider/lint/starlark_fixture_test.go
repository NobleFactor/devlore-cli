// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package lint

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// wantPath is the inventory of live defect sites in the docker fixture, and fixturePath is the fixture
// itself. Both are described by testdata/README.md.
//
// The inventory's own paths are relative to testdataDir, not to this package, so that the file stays
// meaningful to the checker -- which will be handed a fixture root and report against it -- rather than
// to the Go package layout that happens to hold it today.
const (
	testdataDir = "testdata"
	fixturePath = testdataDir + "/docker-package"
	wantPath    = testdataDir + "/docker-package.want.tsv"
)

// wantRow is one row of the inventory: the rule that must fire, and where.
type wantRow struct {
	Rule string
	Path string
	Line int
	Call string
}

// TestStarlarkFixtureShape asserts the docker fixture is intact.
//
// The fixture is `devlore-registry/packages/docker` at `cc87c4f0`, copied verbatim because it exists
// nowhere else -- the registry's `develop` no longer carries it. It is the regression corpus for #721,
// so a truncated or flattened copy would silently weaken every test built on it later.
//
// The directory layout is asserted, not just the file count, because Requirement 3b resolves a script's
// phase against the action directory containing it. Flattening the tree would destroy the only evidence
// for that requirement while leaving all 40 files present.
func TestStarlarkFixtureShape(t *testing.T) {

	// Scripts per action directory, from the four platforms the package ships.
	wantPerAction := map[string]int{
		"Deploy":       16,
		"Upgrade":      12,
		"Decommission": 12,
	}

	gotPerAction := map[string]int{}
	total := 0

	err := filepath.WalkDir(fixturePath, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".star" {
			return nil
		}

		total++

		// <fixture>/<platform>/<action>/<phase>.star -- the action is the parent directory's name.
		gotPerAction[filepath.Base(filepath.Dir(path))]++

		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", fixturePath, err)
	}

	if total != 40 {
		t.Errorf("fixture holds %d .star files, want 40", total)
	}

	for action, want := range wantPerAction {
		if got := gotPerAction[action]; got != want {
			t.Errorf("%s holds %d scripts, want %d", action, got, want)
		}
	}

	for action, got := range gotPerAction {
		if _, known := wantPerAction[action]; !known {
			t.Errorf("unexpected action directory %q with %d scripts", action, got)
		}
	}
}

// TestStarlarkFixtureInventory asserts every row of the inventory still points at the text it claims.
//
// The inventory was generated from the fixture, so nothing in it was typed. This test is what keeps the
// two from drifting: it resolves each row's file and line and requires the recorded call to be present
// there, and requires the line not to be a comment -- the inventory holds live sites only, and a call
// that became commented would otherwise stay in it as a defect the checker can never report.
//
// Counts are asserted per rule so that losing rows is a failure rather than a smaller pass.
func TestStarlarkFixtureInventory(t *testing.T) {

	wantPerRule := map[string]int{
		"unknown-namespace":  23, // plan.package.* -- no such namespace; it is pkg, and it takes a list
		"unknown-method":     30, // plan.verify( -- no provider has a Verify method
		"phase-not-in-order": 4,  // Upgrade/install.star -- install is a Deploy phase
	}

	rows := readInventory(t)

	if len(rows) == 0 {
		t.Fatal("inventory is empty")
	}

	gotPerRule := map[string]int{}

	for _, row := range rows {
		gotPerRule[row.Rule]++

		line := lineOf(t, filepath.Join(testdataDir, row.Path), row.Line)

		if !strings.Contains(line, row.Call) {
			t.Errorf("%s:%d does not contain %q: %q", row.Path, row.Line, row.Call, line)
			continue
		}

		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			t.Errorf("%s:%d is a comment, so %q can never be reported: %q", row.Path, row.Line, row.Call, line)
		}
	}

	for rule, want := range wantPerRule {
		if got := gotPerRule[rule]; got != want {
			t.Errorf("rule %s has %d rows, want %d", rule, got, want)
		}
	}

	for rule, got := range gotPerRule {
		if _, known := wantPerRule[rule]; !known {
			t.Errorf("unexpected rule %q with %d rows", rule, got)
		}
	}
}

// readInventory parses testdata/docker-package.want.tsv.
//
// Returns:
//   - `[]wantRow`: every row after the header, in file order.
func readInventory(t *testing.T) []wantRow {

	t.Helper()

	file, err := os.Open(wantPath)
	if err != nil {
		t.Fatalf("opening %s: %v", wantPath, err)
	}
	defer func() { _ = file.Close() }()

	var rows []wantRow

	scanner := bufio.NewScanner(file)
	for number := 1; scanner.Scan(); number++ {
		text := scanner.Text()

		if number == 1 {
			if text != "rule\tpath\tline\tcall" {
				t.Fatalf("%s: unexpected header %q", wantPath, text)
			}
			continue
		}

		if strings.TrimSpace(text) == "" {
			continue
		}

		fields := strings.Split(text, "\t")
		if len(fields) != 4 {
			t.Fatalf("%s:%d: %d fields, want 4: %q", wantPath, number, len(fields), text)
		}

		lineNumber, err := strconv.Atoi(fields[2])
		if err != nil {
			t.Fatalf("%s:%d: line %q is not a number: %v", wantPath, number, fields[2], err)
		}

		rows = append(rows, wantRow{Rule: fields[0], Path: fields[1], Line: lineNumber, Call: fields[3]})
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("reading %s: %v", wantPath, err)
	}

	return rows
}

// lineOf returns one 1-based line of a file.
//
// Returns:
//   - `string`: the line, without its terminator.
func lineOf(t *testing.T, path string, number int) string {

	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for current := 1; scanner.Scan(); current++ {
		if current == number {
			return scanner.Text()
		}
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	t.Fatalf("%s has fewer than %d lines", path, number)

	return ""
}
