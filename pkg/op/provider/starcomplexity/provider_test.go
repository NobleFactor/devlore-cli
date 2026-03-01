// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starcomplexity

import (
	"path/filepath"
	"sort"
	"testing"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("../starcode/testdata")
	if err != nil {
		t.Fatalf("abs testdata: %v", err)
	}
	return dir
}

func captureFiles(t *testing.T, root, pattern string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(root, pattern))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	sort.Strings(matches)
	return matches
}

func TestComplexitySimpleFunc(t *testing.T) {
	root := testdataDir(t)
	files := captureFiles(t, root, "complex.star")

	report, err := (&Provider{Root: root}).ComputeComplexity(files)
	if err != nil {
		t.Fatalf("ComputeComplexity: %v", err)
	}

	if len(report.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(report.Files))
	}

	funcs := report.Files[0].Functions

	funcMap := make(map[string]FunctionComplexity)
	for _, fn := range funcs {
		funcMap[fn.Name] = fn
	}

	// simple_func: just "return True" -> cyclomatic 1, cognitive 0, nesting 0
	if fn, ok := funcMap["simple_func"]; ok {
		if fn.Cyclomatic != 1 {
			t.Errorf("simple_func cyclomatic = %d, want 1", fn.Cyclomatic)
		}
		if fn.Cognitive != 0 {
			t.Errorf("simple_func cognitive = %d, want 0", fn.Cognitive)
		}
		if fn.MaxNesting != 0 {
			t.Errorf("simple_func maxNesting = %d, want 0", fn.MaxNesting)
		}
	} else {
		t.Error("simple_func not found")
	}

	// branching: has if/elif/elif/else with nesting
	if fn, ok := funcMap["branching"]; ok {
		if fn.Cyclomatic != 5 {
			t.Errorf("branching cyclomatic = %d, want 5", fn.Cyclomatic)
		}
		if fn.MaxNesting < 2 {
			t.Errorf("branching maxNesting = %d, want >= 2", fn.MaxNesting)
		}
	} else {
		t.Error("branching not found")
	}

	// looping: for + if + elif
	if fn, ok := funcMap["looping"]; ok {
		if fn.Cyclomatic != 4 {
			t.Errorf("looping cyclomatic = %d, want 4", fn.Cyclomatic)
		}
	} else {
		t.Error("looping not found")
	}

	// complex_logic: if + and + if + or + for + if + and
	if fn, ok := funcMap["complex_logic"]; ok {
		if fn.Cyclomatic != 8 {
			t.Errorf("complex_logic cyclomatic = %d, want 8", fn.Cyclomatic)
		}
		if fn.MaxNesting < 3 {
			t.Errorf("complex_logic maxNesting = %d, want >= 3", fn.MaxNesting)
		}
	} else {
		t.Error("complex_logic not found")
	}

	// with_comprehension: list comprehension + if clause in comprehension
	if fn, ok := funcMap["with_comprehension"]; ok {
		if fn.Cyclomatic != 3 {
			t.Errorf("with_comprehension cyclomatic = %d, want 3", fn.Cyclomatic)
		}
	} else {
		t.Error("with_comprehension not found")
	}
}

func TestComplexityEmptyFile(t *testing.T) {
	root := testdataDir(t)
	files := captureFiles(t, root, "empty.star")

	report, err := (&Provider{Root: root}).ComputeComplexity(files)
	if err != nil {
		t.Fatalf("ComputeComplexity: %v", err)
	}

	if len(report.Files[0].Functions) != 0 {
		t.Errorf("empty file should have 0 functions, got %d", len(report.Files[0].Functions))
	}
}

func TestComplexityParams(t *testing.T) {
	root := testdataDir(t)
	files := captureFiles(t, root, "complex.star")

	report, err := (&Provider{Root: root}).ComputeComplexity(files)
	if err != nil {
		t.Fatalf("ComputeComplexity: %v", err)
	}

	funcMap := make(map[string]FunctionComplexity)
	for _, fn := range report.Files[0].Functions {
		funcMap[fn.Name] = fn
	}

	if fn, ok := funcMap["branching"]; ok {
		if fn.Params != 3 {
			t.Errorf("branching params = %d, want 3", fn.Params)
		}
	}

	if fn, ok := funcMap["complex_logic"]; ok {
		if fn.Params != 4 {
			t.Errorf("complex_logic params = %d, want 4", fn.Params)
		}
	}
}
