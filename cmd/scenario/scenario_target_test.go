// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package scenario

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// scenarioGates are the environment variables a scenario test reads to run: unset, it skips. A test that reads one, in
// its own body or through a function of its package it calls, is a scenario, whatever its name.
var scenarioGates = map[string]bool{"DEVLORE_SCENARIO_RUN": true, "WRIT_SCENARIO_RUN": true}

// scenarioPackages are the packages that hold scenario tests, relative to the repository's root.
var scenarioPackages = []string{"./cmd/scenario", "./cmd/writ"}

// runLine matches a `go test` line of the Makefile's test-scenario recipe: its -run pattern and the package it runs.
var runLine = regexp.MustCompile(`go test .*-run (\S+) .*?(\./\S+)\s*$`)

// TestScenarioTargetRunsEveryScenario fails for each scenario test that `make test-scenario` does not run (#1045).
//
// A scenario runs only when `make test-scenario` names it with -run and sets its gate, and nothing noticed one left
// out: TestSelfUninstallFromInstalledCopyScenario (#1003) never ran, and #949's lane 26 reported it passed in CI. So
// every test in the scenario packages that reads a gate, itself or through a function it calls (cmd/writ's read it in
// newScenarioSandbox and newJourney), is a scenario, found by Go's parser rather than by its name, and each must be
// matched by a -run pattern of the recipe for its own package. This test is not a scenario: it
// runs under `make test`, so a scenario left out fails every ordinary run, CI's included.
func TestScenarioTargetRunsEveryScenario(t *testing.T) {

	root := filepath.Join("..", "..")
	patterns := scenarioRunPatterns(t, filepath.Join(root, "Makefile"))
	if len(patterns) == 0 {
		t.Fatal("the Makefile's test-scenario recipe has no go test -run line")
	}

	found := 0
	for _, pkg := range scenarioPackages {
		for _, name := range scenarioTests(t, filepath.Join(root, pkg)) {
			found++
			if !runsTest(patterns[pkg], name) {
				t.Errorf("make test-scenario does not run %s in %s: no -run pattern for that package matches it "+
					"(patterns: %s)", name, pkg, strings.Join(patterns[pkg], ", "))
			}
		}
	}

	if found == 0 {
		t.Fatalf("no scenario test found in %s: none reads %s", strings.Join(scenarioPackages, ", "), gateNames())
	}
}

// region HELPER FUNCTIONS

// scenarioRunPatterns reads the -run patterns of the Makefile's test-scenario recipe, by package.
//
// The recipe is the tab-indented lines after `test-scenario:`, up to the first line that is neither indented nor
// blank.
//
// Parameters:
//   - `t`: the test, failed when the Makefile cannot be read.
//   - `makefile`: the Makefile's path.
//
// Returns:
//   - `map[string][]string`: each package's -run patterns, as written.
func scenarioRunPatterns(t *testing.T, makefile string) map[string][]string {

	t.Helper()

	file, err := os.Open(makefile) //nolint:gosec // G304: the repository's own Makefile, at a path built here
	if err != nil {
		t.Fatalf("open %s: %v", makefile, err)
	}
	defer func() { _ = file.Close() }() //nolint:errcheck // a read-only file; nothing to do if closing fails

	patterns := map[string][]string{}
	inRecipe := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		if strings.HasPrefix(line, "test-scenario:") {
			inRecipe = true
			continue
		}
		if !inRecipe {
			continue
		}
		if line != "" && !strings.HasPrefix(line, "\t") {
			break
		}
		if match := runLine.FindStringSubmatch(line); match != nil {
			patterns[match[2]] = append(patterns[match[2]], match[1])
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("read %s: %v", makefile, err)
	}

	return patterns
}

// scenarioTests names the test functions of a package directory that read a scenario gate, in their own body or
// through the package's functions they call, however deep.
//
// Parameters:
//   - `t`: the test, failed when a file cannot be parsed.
//   - `dir`: the package's directory.
//
// Returns:
//   - `[]string`: the scenario tests' names, sorted.
func scenarioTests(t *testing.T, dir string) []string {

	t.Helper()

	files, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
	if err != nil {
		t.Fatalf("list %s: %v", dir, err)
	}

	// Every function of the package's tests, by name: whether it names a gate itself, and the functions it calls.
	gated := map[string]bool{}
	calls := map[string][]string{}
	fileSet := token.NewFileSet()
	for _, path := range files {
		parsed, err := parser.ParseFile(fileSet, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, decl := range parsed.Decls {
			function, ok := decl.(*ast.FuncDecl)
			if !ok || function.Recv != nil || function.Body == nil {
				continue
			}
			name := function.Name.Name
			gated[name] = readsGate(function.Body)
			calls[name] = calledNames(function.Body)
		}
	}

	// A function that calls a gated one is gated too, until nothing changes.
	for changed := true; changed; {
		changed = false
		for name, callees := range calls {
			if gated[name] {
				continue
			}
			for _, callee := range callees {
				if gated[callee] {
					gated[name] = true
					changed = true
					break
				}
			}
		}
	}

	var names []string
	for name, isGated := range gated {
		if isGated && strings.HasPrefix(name, "Test") {
			names = append(names, name)
		}
	}

	sort.Strings(names)
	return names
}

// calledNames lists the plain function names a body calls: `f(...)`, not a method or a package's function.
//
// Parameters:
//   - `body`: the function's body.
//
// Returns:
//   - `[]string`: the names called, as often as called.
func calledNames(body *ast.BlockStmt) []string {

	var names []string
	ast.Inspect(body, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			if ident, ok := call.Fun.(*ast.Ident); ok {
				names = append(names, ident.Name)
			}
		}
		return true
	})

	return names
}

// readsGate reports whether a function body holds a scenario gate's name as a string literal.
//
// Parameters:
//   - `body`: the function's body.
//
// Returns:
//   - `bool`: whether it names a gate.
func readsGate(body *ast.BlockStmt) bool {

	reads := false
	ast.Inspect(body, func(node ast.Node) bool {
		literal, ok := node.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return !reads
		}
		if value, err := strconv.Unquote(literal.Value); err == nil && scenarioGates[value] {
			reads = true
		}
		return !reads
	})

	return reads
}

// runsTest reports whether any -run pattern selects a top-level test, as `go test -run` does: the pattern's first
// slash-separated element is a regular expression, matched unanchored against the test's name.
//
// Parameters:
//   - `patterns`: the -run patterns.
//   - `name`: the test's name.
//
// Returns:
//   - `bool`: whether some pattern selects it.
func runsTest(patterns []string, name string) bool {

	for _, pattern := range patterns {
		first, _, _ := strings.Cut(pattern, "/")
		if matched, err := regexp.MatchString(first, name); err == nil && matched {
			return true
		}
	}

	return false
}

// gateNames lists the scenario gates, for a message.
//
// Returns:
//   - `string`: the gates' names, sorted, comma-separated.
func gateNames() string {

	names := make([]string, 0, len(scenarioGates))
	for name := range scenarioGates {
		names = append(names, name)
	}
	sort.Strings(names)

	return strings.Join(names, ", ")
}

// endregion
