// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package devloretest_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/devlore-test/devloretest"
)

// TestFileFind runs the find fixture (step-52 backfill): file.find is otherwise fixture-uncovered.
func TestFileFind(t *testing.T) {
	script := filepath.Join(testdataDir(t), "test_file_find.star")
	runner := devloretest.NewRunner(script, devloretest.WithGraphBuilder())
	result, err := runner.Start(context.Background())
	if err != nil {
		t.Fatalf("runner error: %v", err)
	}
	if !result.Passed {
		for _, f := range result.Failures {
			t.Errorf("FAIL: %s — %s", f.Expectation, f.Message)
		}
	}
	if result.UnitCount != 4 {
		t.Errorf("unit_count = %d, want 4", result.UnitCount)
	}
}

// TestTemplateRenderBytes runs the render_bytes fixture (step-52 backfill): render_bytes otherwise rides render_text's
// fixtures.
func TestTemplateRenderBytes(t *testing.T) {
	script := filepath.Join(testdataDir(t), "test_template_render_bytes.star")
	runner := devloretest.NewRunner(script, devloretest.WithGraphBuilder())
	result, err := runner.Start(context.Background())
	if err != nil {
		t.Fatalf("runner error: %v", err)
	}
	if !result.Passed {
		for _, f := range result.Failures {
			t.Errorf("FAIL: %s — %s", f.Expectation, f.Message)
		}
	}
	if result.UnitCount != 1 {
		t.Errorf("unit_count = %d, want 1", result.UnitCount)
	}
}
