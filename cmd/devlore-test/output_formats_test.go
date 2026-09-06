// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package main_test

import (
	"encoding/csv"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// region Tests

// writeThenRemove is the two-operation fixture: write a file, then remove it, with the removal consuming the
// write's promise so the graph has one real edge. Deliberately small -- what these tests exercise is the
// presentation of a result, not the graph.
func writeThenRemove(t *testing.T) string {
	t.Helper()
	return filepath.Join(filepath.Dir(scriptPath), "test_write_then_remove.star")
}

// TestOutputFormats_EveryFormatRenders runs one graph through every value of --output.
//
// The point is coverage of the SET, not of any one rendering: a format that is registered but broken, or
// that silently falls through to another, fails here. `writ reconcile` (as `writ status` then) shipped for months answering `-o yaml`
// with a human dashboard because nothing asserted across the whole set (#754).
func TestOutputFormats_EveryFormatRenders(t *testing.T) {

	script := writeThenRemove(t)

	for _, tc := range []struct {
		format string
		verify func(t *testing.T, stdout string)
	}{
		{"json", func(t *testing.T, out string) {
			var v map[string]any
			if err := json.Unmarshal([]byte(out), &v); err != nil {
				t.Fatalf("not JSON: %v\n%s", err, out)
			}
			if v["unit_count"] != float64(2) {
				t.Errorf("unit_count = %v, want 2", v["unit_count"])
			}
		}},

		{"yaml", func(t *testing.T, out string) {
			var v map[string]any
			if err := yaml.Unmarshal([]byte(out), &v); err != nil {
				t.Fatalf("not YAML: %v\n%s", err, out)
			}
			if v["unit_count"] != 2 {
				t.Errorf("unit_count = %v, want 2", v["unit_count"])
			}
		}},

		{"csv", func(t *testing.T, out string) {
			records, err := csv.NewReader(strings.NewReader(out)).ReadAll()
			if err != nil {
				t.Fatalf("not CSV: %v\n%s", err, out)
			}
			if len(records) != 2 {
				t.Fatalf("records = %d, want 2 (a header and one row)", len(records))
			}
			if !contains(records[0], "unit_count") {
				t.Errorf("header lacks unit_count: %v", records[0])
			}
		}},

		{"table", func(t *testing.T, out string) {
			lines := strings.Split(strings.TrimSpace(out), "\n")
			if len(lines) != 2 {
				t.Fatalf("lines = %d, want 2 (a header and one row):\n%s", len(lines), out)
			}
			if !strings.Contains(lines[0], "UNIT_COUNT") {
				t.Errorf("header is not upper-cased json names: %q", lines[0])
			}
		}},

		{"list", func(t *testing.T, out string) {
			if !strings.Contains(out, "unit_count") || !strings.Contains(out, " : ") {
				t.Errorf("not list-shaped:\n%s", out)
			}
			if strings.Contains(out, "UnitCount") {
				t.Errorf("leaked a Go field name:\n%s", out)
			}
		}},

		{"value", func(t *testing.T, out string) {
			if strings.Contains(out, "unit_count") {
				t.Errorf("value emitted headers, which it must not:\n%s", out)
			}
			if !strings.Contains(out, "\t") {
				t.Errorf("value is not tab-separated:\n%q", out)
			}
		}},

		{"none", func(t *testing.T, out string) {
			if out != "" {
				t.Errorf("none emitted %q, want nothing", out)
			}
		}},

		{"template={{.unit_count}}", func(t *testing.T, out string) {
			if strings.TrimSpace(out) != "2" {
				t.Errorf("template rendered %q, want 2", strings.TrimSpace(out))
			}
		}},
	} {
		t.Run(tc.format, func(t *testing.T) {

			stdout, stderr, code := runIn(t.TempDir(),
				"run", "--store", t.TempDir(), "-o", tc.format, script)

			if code != 0 {
				t.Fatalf("exit %d\nstderr: %s", code, stderr)
			}
			tc.verify(t, stdout)
		})
	}
}

// TestOutputFormats_AnUnknownFormatIsRejected is the other half of set coverage.
//
// A format the set does not contain must fail loudly and name what is accepted, rather than fall through to
// a default -- which is what made a typo indistinguishable from a request in `writ` (#754).
func TestOutputFormats_AnUnknownFormatIsRejected(t *testing.T) {

	_, stderr, code := runIn(t.TempDir(),
		"run", "--store", t.TempDir(), "-o", "bogus", writeThenRemove(t))

	if code == 0 {
		t.Error("an unknown formatter exited 0, want non-zero")
	}
	for _, name := range []string{"bogus", "csv", "json", "list", "none", "table", "value", "yaml"} {
		if !strings.Contains(stderr, name) {
			t.Errorf("the error does not name %q: %s", name, stderr)
		}
	}
}

// endregion

// region Helpers

// contains reports whether values holds want.
func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// endregion
