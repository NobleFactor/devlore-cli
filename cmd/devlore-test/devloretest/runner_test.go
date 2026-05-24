// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package devloretest_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/NobleFactor/devlore-cli/cmd/devlore-test/devloretest"

	_ "github.com/NobleFactor/devlore-cli/cmd/star/inventory"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/inventory"
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
	if result.ExpectationCount != 2 {
		t.Errorf("expectation_count = %d, want 2", result.ExpectationCount)
	}
}

func TestCopy(t *testing.T) {
	script := filepath.Join(testdataDir(t), "test_copy.star")
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
	if result.UnitCount != 2 {
		t.Errorf("unit_count = %d, want 2", result.UnitCount)
	}
}

func TestWriteAndRead(t *testing.T) {
	runScript(t, "test_write_and_read.star")
}

func TestCompensation(t *testing.T) {
	runScript(t, "test_compensation.star")
}

func TestTrace(t *testing.T) {
	script := filepath.Join(testdataDir(t), "test_write_text.star")
	runner := devloretest.NewRunner(script, devloretest.WithTrace(), devloretest.WithGraphBuilder())
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

func TestHello(t *testing.T) {
	runScript(t, "test_hello.star")
}

func TestFileLifecycle(t *testing.T) {
	runScript(t, "test_file_lifecycle.star")
}

func TestMkdirAndRemoveAll(t *testing.T) {
	runScript(t, "test_mkdir_and_remove_all.star")
}

func TestShellExec(t *testing.T) {
	runScript(t, "test_shell_exec.star")
}

func TestSource(t *testing.T) {
	runScript(t, "test_source.star")
}

func TestGather(t *testing.T) {
	runScript(t, "test_gather.star")
}

func TestMove(t *testing.T) {
	runScript(t, "test_move.star")
}

func TestLink(t *testing.T) {
	runScript(t, "test_link.star")
}

func TestWriteBytes(t *testing.T) {
	runScript(t, "test_write_bytes.star")
}

func TestBackup(t *testing.T) {
	runScript(t, "test_backup.star")
}

func TestChooseExists(t *testing.T) {
	runScript(t, "test_choose_exists.star")
}

func TestChooseNotExists(t *testing.T) {
	runScript(t, "test_choose_not_exists.star")
}

func TestIsDir(t *testing.T) {
	runScript(t, "test_is_dir.star")
}

func TestIsFile(t *testing.T) {
	runScript(t, "test_is_file.star")
}

// --- plan.choose comprehensive coverage (Go-test-style table coverage across literal / lambda /
//     planned-predicate When values; first-match-wins; multi-case + zero-case forms) ---

func TestChooseLambdas(t *testing.T) {
	runScript(t, "test_choose_lambdas.star")
}

func TestChooseLiterals(t *testing.T) {
	runScript(t, "test_choose_literals.star")
}

func TestChoosePredicates(t *testing.T) {
	runScript(t, "test_choose_predicates.star")
}

// runScript runs a .star test script with all providers and fails on any expectation failures.
func runScript(t *testing.T, name string) {
	t.Helper()
	script := filepath.Join(testdataDir(t), name)
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
}

// runScriptDryRun runs a .star test script in dry-run mode with graph builder.
func runScriptDryRun(t *testing.T, name string) {
	t.Helper()
	script := filepath.Join(testdataDir(t), name)
	runner := devloretest.NewRunner(script, devloretest.WithDryRun(), devloretest.WithGraphBuilder())
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

// runScriptImm runs a .star test script in immediate mode (no graph builder).
func runScriptImm(t *testing.T, name string) {
	t.Helper()
	script := filepath.Join(testdataDir(t), name)
	runner := devloretest.NewRunner(script)
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

// --- Planned action tests — file provider gaps ---

func TestFileUnlink(t *testing.T) {
	runScript(t, "test_file_unlink.star")
}

func TestFileGlob(t *testing.T) {
	runScript(t, "test_file_glob.star")
}

func TestFileJoin(t *testing.T) {
	runScript(t, "test_file_join.star")
}

func TestFileName(t *testing.T) {
	runScript(t, "test_file_name.star")
}

func TestFileParent(t *testing.T) {
	runScript(t, "test_file_parent.star")
}

// --- WalkTree callable tests ---

func TestWalkTreePlanned(t *testing.T) {
	runScript(t, "test_walk_tree_planned.star")
}

// --- Planned action tests — template provider ---

func TestTemplateRender(t *testing.T) {
	runScript(t, "test_template_render.star")
}

// --- Planned action tests — dry-run providers ---

func TestArchiveExtract(t *testing.T) {
	runScriptDryRun(t, "test_archive.star")
}

func TestEncryptionDecryptSopsFile(t *testing.T) {
	runScriptDryRun(t, "test_encryption.star")
}

func TestGitActions(t *testing.T) {
	runScriptDryRun(t, "test_git.star")
}

func TestNetDownload(t *testing.T) {
	runScriptDryRun(t, "test_net.star")
}

func TestPkgActions(t *testing.T) {
	runScriptDryRun(t, "test_pkg.star")
}

func TestServiceActions(t *testing.T) {
	runScriptDryRun(t, "test_service.star")
}

func TestJsonActions(t *testing.T) {
	runScriptDryRun(t, "test_json.star")
}

func TestYamlActions(t *testing.T) {
	runScriptDryRun(t, "test_yaml.star")
}

func TestRegexpActions(t *testing.T) {
	runScriptDryRun(t, "test_regexp.star")
}

// --- Immediate action tests ---

func TestImmJSON(t *testing.T) {
	runScriptImm(t, "test_imm_json.star")
}

func TestImmYAML(t *testing.T) {
	runScriptImm(t, "test_imm_yaml.star")
}

func TestImmRegexp(t *testing.T) {
	runScriptImm(t, "test_imm_regexp.star")
}

func TestImmTemplate(t *testing.T) {
	runScriptImm(t, "test_imm_template.star")
}

func TestImmUI(t *testing.T) {
	runScriptImm(t, "test_imm_ui.star")
}

func TestImmStaranalysis(t *testing.T) {
	runScriptImm(t, "test_imm_staranalysis.star")
}

func TestImmStarcode(t *testing.T) {
	runScriptImm(t, "test_imm_starcode.star")
}

func TestImmStarcomplexity(t *testing.T) {
	runScriptImm(t, "test_imm_starcomplexity.star")
}

func TestImmStarindex(t *testing.T) {
	runScriptImm(t, "test_imm_starindex.star")
}

func TestImmStarstats(t *testing.T) {
	runScriptImm(t, "test_imm_starstats.star")
}

// --- Terminal flow control tests ---

func TestFlowComplete(t *testing.T) {
	runScriptDryRun(t, "test_flow_complete.star")
}

func TestFlowDegraded(t *testing.T) {
	runScript(t, "test_flow_degraded.star")
}

func TestFlowFatal(t *testing.T) {
	runScript(t, "test_flow_fatal.star")
}
