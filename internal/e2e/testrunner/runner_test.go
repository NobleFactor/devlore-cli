// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package testrunner_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/NobleFactor/devlore-cli/internal/e2e/testrunner"
)

// testdataDir returns the absolute path to the data/ directory.
func testdataDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	return filepath.Join(filepath.Dir(file), "data")
}

func TestWriteText(t *testing.T) {
	script := filepath.Join(testdataDir(t), "test_write_text.star")
	runner := testrunner.New(script)
	result, err := runner.Start(context.Background())
	if err != nil {
		t.Fatalf("runner error: %v", err)
	}
	if !result.Passed {
		for _, f := range result.Failures {
			t.Errorf("FAIL: %s — %s", f.Expectation, f.Message)
		}
	}
	if result.NodeCount != 1 {
		t.Errorf("node_count = %d, want 1", result.NodeCount)
	}
	if result.ExpectationCount != 2 {
		t.Errorf("expectation_count = %d, want 2", result.ExpectationCount)
	}
}

func TestCopy(t *testing.T) {
	script := filepath.Join(testdataDir(t), "test_copy.star")
	runner := testrunner.New(script)
	result, err := runner.Start(context.Background())
	if err != nil {
		t.Fatalf("runner error: %v", err)
	}
	if !result.Passed {
		for _, f := range result.Failures {
			t.Errorf("FAIL: %s — %s", f.Expectation, f.Message)
		}
	}
	if result.NodeCount != 2 {
		t.Errorf("node_count = %d, want 2", result.NodeCount)
	}
}

func TestWriteAndRead(t *testing.T) {
	script := filepath.Join(testdataDir(t), "test_write_and_read.star")
	runner := testrunner.New(script)
	result, err := runner.Start(context.Background())
	if err != nil {
		t.Fatalf("runner error: %v", err)
	}
	if !result.Passed {
		for _, f := range result.Failures {
			t.Errorf("FAIL: %s — %s", f.Expectation, f.Message)
		}
	}
}

func TestCompensation(t *testing.T) {
	script := filepath.Join(testdataDir(t), "test_compensation.star")
	runner := testrunner.New(script)
	result, err := runner.Start(context.Background())
	if err != nil {
		t.Fatalf("runner error: %v", err)
	}
	if !result.Passed {
		for _, f := range result.Failures {
			t.Errorf("FAIL: %s — %s", f.Expectation, f.Message)
		}
	}
}

func TestTrace(t *testing.T) {
	script := filepath.Join(testdataDir(t), "test_write_text.star")
	runner := testrunner.New(script, testrunner.WithTrace())
	result, err := runner.Start(context.Background())
	if err != nil {
		t.Fatalf("runner error: %v", err)
	}
	if !result.Passed {
		for _, f := range result.Failures {
			t.Errorf("FAIL: %s — %s", f.Expectation, f.Message)
		}
	}
	if len(result.Trace) == 0 {
		t.Error("trace enabled but no trace entries recorded")
	}
}

func TestDryRun(t *testing.T) {
	script := filepath.Join(testdataDir(t), "test_write_text.star")
	runner := testrunner.New(script, testrunner.WithDryRun())
	result, err := runner.Start(context.Background())
	if err != nil {
		t.Fatalf("runner error: %v", err)
	}
	// In dry-run mode, the file should NOT exist (no side effects).
	// The expect_file expectation should fail because the file wasn't written.
	if result.Passed {
		t.Error("dry-run should cause file expectation to fail")
	}
}
