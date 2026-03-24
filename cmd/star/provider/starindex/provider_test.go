// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starindex

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

func TestIndexSimple(t *testing.T) {
	root := testdataDir(t)
	files := captureFiles(t, root, "simple.star")

	idx, err := (&Provider{Root: root}).IndexFiles(files, true, true)
	if err != nil {
		t.Fatalf("IndexFiles: %v", err)
	}

	if idx.Totals.FileCount != 1 {
		t.Fatalf("FileCount = %d, want 1", idx.Totals.FileCount)
	}

	f := idx.Files[0]

	// simple.star has: greet, add
	if len(f.Functions) != 2 {
		t.Fatalf("Functions = %d, want 2", len(f.Functions))
	}

	// Verify greet has docstring
	var foundGreet bool
	for _, fn := range f.Functions {
		if fn.Name == "greet" {
			foundGreet = true
			if !fn.HasDocstring {
				t.Error("greet should have a docstring")
			}
			if fn.Params != 1 {
				t.Errorf("greet params = %d, want 1", fn.Params)
			}
		}
	}
	if !foundGreet {
		t.Error("function 'greet' not found")
	}

	// simple.star has: load("@stdlib//json", "json")
	if len(f.Loads) != 1 {
		t.Fatalf("Loads = %d, want 1", len(f.Loads))
	}
	if f.Loads[0].Module != "@stdlib//json" {
		t.Errorf("load module = %q, want %q", f.Loads[0].Module, "@stdlib//json")
	}

	// simple.star has: MESSAGE = "hello"
	if len(f.Globals) != 1 {
		t.Fatalf("Globals = %d, want 1", len(f.Globals))
	}
	if f.Globals[0].Name != "MESSAGE" {
		t.Errorf("global name = %q, want %q", f.Globals[0].Name, "MESSAGE")
	}
}

func TestIndexWithoutDocstrings(t *testing.T) {
	root := testdataDir(t)
	files := captureFiles(t, root, "simple.star")

	idx, err := (&Provider{Root: root}).IndexFiles(files, false, true)
	if err != nil {
		t.Fatalf("IndexFiles: %v", err)
	}

	for _, fn := range idx.Files[0].Functions {
		if fn.HasDocstring {
			t.Errorf("function %q should not have docstring when withDocstrings=false", fn.Name)
		}
	}
}

func TestIndexWithoutGlobals(t *testing.T) {
	root := testdataDir(t)
	files := captureFiles(t, root, "simple.star")

	idx, err := (&Provider{Root: root}).IndexFiles(files, true, false)
	if err != nil {
		t.Fatalf("IndexFiles: %v", err)
	}

	if len(idx.Files[0].Globals) != 0 {
		t.Errorf("Globals = %d, want 0 when withGlobals=false", len(idx.Files[0].Globals))
	}
}

func TestIndexWithGlobals(t *testing.T) {
	root := testdataDir(t)
	files := captureFiles(t, root, "with_globals.star")

	idx, err := (&Provider{Root: root}).IndexFiles(files, true, true)
	if err != nil {
		t.Fatalf("IndexFiles: %v", err)
	}

	f := idx.Files[0]

	// with_globals.star has: VERSION, DEBUG, MAX_RETRIES, DEFAULT_CONFIG, ENABLED_FEATURES
	if len(f.Globals) != 5 {
		t.Errorf("Globals = %d, want 5", len(f.Globals))
	}

	// Has 1 function: configure
	if len(f.Functions) != 1 {
		t.Errorf("Functions = %d, want 1", len(f.Functions))
	}

	// Has 1 load: @stdlib//os
	if len(f.Loads) != 1 {
		t.Errorf("Loads = %d, want 1", len(f.Loads))
	}
}

func TestIndexComplex(t *testing.T) {
	root := testdataDir(t)
	files := captureFiles(t, root, "complex.star")

	idx, err := (&Provider{Root: root}).IndexFiles(files, true, true)
	if err != nil {
		t.Fatalf("IndexFiles: %v", err)
	}

	f := idx.Files[0]

	// complex.star has: simple_func, branching, looping, complex_logic, with_comprehension
	if len(f.Functions) != 5 {
		t.Fatalf("Functions = %d, want 5", len(f.Functions))
	}

	// Has 2 loads: json, yaml
	if len(f.Loads) != 2 {
		t.Errorf("Loads = %d, want 2", len(f.Loads))
	}

	// Has 2 globals: THRESHOLD, MAX_ITEMS
	if len(f.Globals) != 2 {
		t.Errorf("Globals = %d, want 2", len(f.Globals))
	}
}

func TestIndexTotals(t *testing.T) {
	root := testdataDir(t)
	files := captureFiles(t, root, "*.star")

	idx, err := (&Provider{Root: root}).IndexFiles(files, true, true)
	if err != nil {
		t.Fatalf("IndexFiles: %v", err)
	}

	// Verify totals aggregate correctly
	var funcSum, loadSum, globalSum int
	for _, f := range idx.Files {
		funcSum += len(f.Functions)
		loadSum += len(f.Loads)
		globalSum += len(f.Globals)
	}

	if idx.Totals.Functions != funcSum {
		t.Errorf("Totals.Functions = %d, sum = %d", idx.Totals.Functions, funcSum)
	}
	if idx.Totals.Loads != loadSum {
		t.Errorf("Totals.Loads = %d, sum = %d", idx.Totals.Loads, loadSum)
	}
	if idx.Totals.Globals != globalSum {
		t.Errorf("Totals.Globals = %d, sum = %d", idx.Totals.Globals, globalSum)
	}
}

func TestIndexEmptyFile(t *testing.T) {
	root := testdataDir(t)
	files := captureFiles(t, root, "empty.star")

	idx, err := (&Provider{Root: root}).IndexFiles(files, true, true)
	if err != nil {
		t.Fatalf("IndexFiles: %v", err)
	}

	f := idx.Files[0]
	if len(f.Functions) != 0 {
		t.Errorf("empty file Functions = %d, want 0", len(f.Functions))
	}
	if len(f.Loads) != 0 {
		t.Errorf("empty file Loads = %d, want 0", len(f.Loads))
	}
	if len(f.Globals) != 0 {
		t.Errorf("empty file Globals = %d, want 0", len(f.Globals))
	}
}
