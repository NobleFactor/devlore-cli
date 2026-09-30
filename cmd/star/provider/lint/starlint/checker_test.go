// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package starlint_test

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/star/provider/lint/starlint"

	_ "github.com/NobleFactor/devlore-cli/cmd/star/inventory"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"
)

// fixtureRoot is the docker corpus, relative to this package.
const fixtureRoot = "../testdata/docker-package"

// inventoryPath is the expected findings, relative to this package.
const inventoryPath = "../testdata/docker-package.want.tsv"

// TestCheckFixtureMatchesInventory is the regression test the whole issue turns on.
//
// The fixture is `devlore-registry/packages/docker` at `cc87c4f0`, which shipped and could not execute on any
// platform. The inventory beside it records every live defect site by rule, path and line, generated from the
// fixture rather than typed. This asserts the checker reports exactly those -- not a subset, and nothing more.
func TestCheckFixtureMatchesInventory(t *testing.T) {

	want := readInventoryRows(t)
	got := map[string]bool{}

	checker := starlint.NewChecker()

	err := filepath.WalkDir(fixtureRoot, func(path string, entry fs.DirEntry, err error) error {

		if err != nil || entry.IsDir() || filepath.Ext(path) != ".star" {
			return err
		}

		findings, err := checker.CheckFile(path)
		if err != nil {
			return err
		}

		for _, finding := range findings {
			// The inventory's paths are relative to testdata/, so normalize the walked path to match.
			relative := filepath.ToSlash(strings.TrimPrefix(filepath.ToSlash(finding.Path), "../testdata/"))
			got[fmt.Sprintf("%s\t%s\t%d", finding.Rule, relative, finding.Line)] = true
		}

		return nil
	})
	if err != nil {
		t.Fatalf("walking the fixture: %v", err)
	}

	for key := range want {
		if !got[key] {
			t.Errorf("inventory expects %s, and the checker did not report it", key)
		}
	}

	for key := range got {
		if !want[key] {
			t.Errorf("checker reported %s, which the inventory does not expect", key)
		}
	}

	if len(got) != len(want) {
		t.Errorf("checker reported %d findings, inventory expects %d", len(got), len(want))
	}
}

// TestRepositoryIsClean asserts the checker reports nothing across this repository's own `.star` files.
//
// This is the zero-false-positive requirement, kept honest permanently rather than measured once. It caught
// two real dead calls when it was first run -- `plan.fatal`, which no provider has, and
// `plan.pkg.update(manager="")`, whose method takes no parameters at all -- and both are now fixed. A new
// false positive fails here, and so does a new dead call.
func TestRepositoryIsClean(t *testing.T) {

	root := "../../../../.."

	out, err := exec.Command("git", "-C", root, "ls-files", "*.star").Output()
	if err != nil {
		t.Skipf("git ls-files is unavailable: %v", err)
	}

	checker := starlint.NewChecker()
	checked := 0

	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {

		// The fixture is deliberately full of defects; TestCheckFixtureMatchesInventory owns it.
		if line == "" || strings.Contains(line, "testdata/docker-package") {
			continue
		}

		checked++

		findings, err := checker.CheckFile(filepath.Join(root, line))
		if err != nil {
			t.Errorf("%s: %v", line, err)
			continue
		}

		for _, finding := range findings {
			t.Errorf("%s: %s", line, finding.Message)
		}
	}

	if checked < 100 {
		t.Errorf("checked only %d files; the corpus is larger than that, so the sweep did not run", checked)
	}
}

// TestShadowedPlanIsNotResolved covers Requirement 2 across every shape that can bind the name.
//
// `validate.star` and `extract.star` bind `plan = {}` as a local dictionary and then subscript it, and they
// are real files in this repository -- TestRepositoryIsClean would fail on them if this were wrong. They
// cover one shape, so the rest are here. The last case is the one that matters most in the other direction:
// `plan[k] = v` mutates and does NOT bind, so a file doing that to the real plan must still be checked.
func TestShadowedPlanIsNotResolved(t *testing.T) {

	for _, testCase := range []struct {
		name   string
		source string
		want   int
	}{
		{
			name: "local assignment shadows for the whole function",
			source: `
def f():
    plan = {}
    plan.nonexistent_thing()
`,
			want: 0,
		},
		{
			name: "assignment after the use still shadows, because Starlark scopes per function",
			source: `
def f():
    plan.nonexistent_thing()
    plan = {}
`,
			want: 0,
		},
		{
			name: "parameter shadows",
			source: `
def f(plan):
    plan.nonexistent_thing()
`,
			want: 0,
		},
		{
			name: "for-loop variable shadows",
			source: `
def f(items):
    for plan in items:
        plan.nonexistent_thing()
`,
			want: 0,
		},
		{
			name: "an enclosing function's binding shadows a nested one",
			source: `
def outer():
    plan = {}
    def inner():
        plan.nonexistent_thing()
`,
			want: 0,
		},
		{
			name: "a sibling function's binding does not shadow",
			source: `
def bound():
    plan = {}

def unbound():
    plan.nonexistent_thing()
`,
			want: 1,
		},
		{
			name: "subscript assignment mutates and does not bind",
			source: `
def f(key):
    plan[key] = []
    plan.nonexistent_thing()
`,
			want: 1,
		},
		{
			name: "module scope is checked",
			source: `
plan.nonexistent_thing()
`,
			want: 1,
		},
		{
			name: "a module-level binding shadows everywhere",
			source: `
plan = {}

def f():
    plan.nonexistent_thing()
`,
			want: 0,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {

			findings := starlint.NewChecker().Check("scope.star", []byte(testCase.source))

			if len(findings) != testCase.want {
				t.Errorf("got %d findings, want %d: %v", len(findings), testCase.want, findings)
			}
		})
	}
}

// TestPhaseScriptRequirements covers 3a and 3b, and the runtime each file is judged against.
//
// A file's location decides both. Inside an action directory it is a phase script: the phase order applies,
// an entry point is required, and lore's hermetic planning runtime is modeled. Anywhere else none of that
// holds -- which is why `plan.save_definition` is a finding in the first case and not the last.
func TestPhaseScriptRequirements(t *testing.T) {

	for _, testCase := range []struct {
		name   string
		path   string
		source string
		rules  []string
	}{
		{
			name:   "a valid deploy phase script",
			path:   "Darwin/Deploy/install.star",
			source: "def install(package, phase):\n    plan.pkg.install(packages=[\"curl\"])\n",
			rules:  nil,
		},
		{
			name:   "install is not an upgrade phase",
			path:   "Darwin/Upgrade/install.star",
			source: "def install(package, phase):\n    pass\n",
			rules:  []string{"phase-not-in-order"},
		},
		{
			name:   "the entry point must be named for the phase",
			path:   "Darwin/Deploy/install.star",
			source: "def setup(package, phase):\n    pass\n",
			rules:  []string{"missing-phase-entry-point"},
		},
		{
			name:   "the entry point takes package and phase",
			path:   "Darwin/Deploy/install.star",
			source: "def install(package):\n    pass\n",
			rules:  []string{"phase-entry-point-arity"},
		},
		{
			name:   "a phase script runs hermetically",
			path:   "Darwin/Deploy/install.star",
			source: "def install(package, phase):\n    plan.save_definition(graph=None, path=\"x\")\n",
			rules:  []string{"not-deterministic"},
		},
		{
			name:   "a phase script may not orchestrate",
			path:   "Darwin/Deploy/install.star",
			source: "def install(package, phase):\n    plan.run()\n",
			rules:  []string{"denied"},
		},
		{
			name:   "an ambient script is not hermetic and needs no entry point",
			path:   "data/test_thing.star",
			source: "plan.save_definition(graph=None, path=\"x\")\n",
			rules:  nil,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {

			directory := filepath.Join(t.TempDir(), filepath.Dir(testCase.path))
			if err := os.MkdirAll(directory, 0o755); err != nil {
				t.Fatalf("creating %s: %v", directory, err)
			}

			path := filepath.Join(directory, filepath.Base(testCase.path))
			if err := os.WriteFile(path, []byte(testCase.source), 0o644); err != nil {
				t.Fatalf("writing %s: %v", path, err)
			}

			findings, err := starlint.NewChecker().CheckFile(path)
			if err != nil {
				t.Fatalf("checking %s: %v", path, err)
			}

			var got []string
			for _, finding := range findings {
				got = append(got, finding.Rule)
			}
			sort.Strings(got)

			want := append([]string(nil), testCase.rules...)
			sort.Strings(want)

			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Errorf("rules %v, want %v (%v)", got, want, findings)
			}
		})
	}
}

// TestParseErrorIsAFinding asserts an unparseable file is reported rather than returned as an error.
//
// A corpus is checked as a whole, and one bad file must not stop the other 168.
func TestParseErrorIsAFinding(t *testing.T) {

	findings := starlint.NewChecker().Check("broken.star", []byte("def oops(\n"))

	if len(findings) != 1 {
		t.Fatalf("got %d findings, want 1: %v", len(findings), findings)
	}

	if findings[0].Rule != "parse-error" {
		t.Errorf("rule is %q, want %q", findings[0].Rule, "parse-error")
	}
}

// readInventoryRows reads the expected findings as "rule\tpath\tline" keys.
//
// Returns:
//   - `map[string]bool`: one key per inventory row.
func readInventoryRows(t *testing.T) map[string]bool {

	t.Helper()

	file, err := os.Open(inventoryPath)
	if err != nil {
		t.Fatalf("opening %s: %v", inventoryPath, err)
	}
	defer func() { _ = file.Close() }()

	rows := map[string]bool{}

	scanner := bufio.NewScanner(file)
	for number := 1; scanner.Scan(); number++ {

		text := scanner.Text()
		if number == 1 || strings.TrimSpace(text) == "" {
			continue
		}

		fields := strings.Split(text, "\t")
		if len(fields) != 4 {
			t.Fatalf("%s:%d: %d fields, want 4", inventoryPath, number, len(fields))
		}

		if _, err := strconv.Atoi(fields[2]); err != nil {
			t.Fatalf("%s:%d: line %q is not a number", inventoryPath, number, fields[2])
		}

		rows[fmt.Sprintf("%s\t%s\t%s", fields[0], fields[1], fields[2])] = true
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("reading %s: %v", inventoryPath, err)
	}

	if len(rows) == 0 {
		t.Fatalf("%s holds no rows", inventoryPath)
	}

	return rows
}
