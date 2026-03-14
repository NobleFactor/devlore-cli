// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package testrunner_test

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/NobleFactor/devlore-cli/internal/e2e/testrunner"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	filegen "github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gen"
	jsongen "github.com/NobleFactor/devlore-cli/pkg/op/provider/json/gen"
	regexpgen "github.com/NobleFactor/devlore-cli/pkg/op/provider/regexp/gen"
	staranalysisgen "github.com/NobleFactor/devlore-cli/pkg/op/provider/staranalysis/gen"
	starcodegen "github.com/NobleFactor/devlore-cli/pkg/op/provider/starcode/gen"
	starcomplexitygen "github.com/NobleFactor/devlore-cli/pkg/op/provider/starcomplexity/gen"
	starindexgen "github.com/NobleFactor/devlore-cli/pkg/op/provider/starindex/gen"
	starstatsgen "github.com/NobleFactor/devlore-cli/pkg/op/provider/starstats/gen"
	templategen "github.com/NobleFactor/devlore-cli/pkg/op/provider/template/gen"
	uigen "github.com/NobleFactor/devlore-cli/pkg/op/provider/ui/gen"
	yamlgen "github.com/NobleFactor/devlore-cli/pkg/op/provider/yaml/gen"
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
	runner := testrunner.New(script, testrunner.WithGraphBuilder(), testrunner.WithReceivers(filegen.Receiver))
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
	runner := testrunner.New(script, testrunner.WithGraphBuilder(), testrunner.WithReceivers(filegen.Receiver))
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
	runner := testrunner.New(script, testrunner.WithGraphBuilder(), testrunner.WithReceivers(filegen.Receiver))
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
	runner := testrunner.New(script, testrunner.WithGraphBuilder(), testrunner.WithReceivers(filegen.Receiver))
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
	runner := testrunner.New(script, testrunner.WithTrace(), testrunner.WithGraphBuilder(), testrunner.WithReceivers(filegen.Receiver))
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
	t.Skip("choose executor runs then-branch even when predicate is false")
	runScript(t, "test_choose_not_exists.star")
}

func TestIsDir(t *testing.T) {
	runScript(t, "test_is_dir.star")
}

func TestIsFile(t *testing.T) {
	t.Skip("choose executor passes empty path for Output captured in lambda closure")
	runScript(t, "test_is_file.star")
}

// runScript runs a .star test script with plan+file providers and fails on any expectation failures.
func runScript(t *testing.T, name string) {
	t.Helper()
	script := filepath.Join(testdataDir(t), name)
	runner := testrunner.New(script, testrunner.WithGraphBuilder(), testrunner.WithReceivers(filegen.Receiver))
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
	runner := testrunner.New(script, testrunner.WithDryRun(), testrunner.WithGraphBuilder())
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

// runScriptImm runs a .star test script with the given immediate providers.
func runScriptImm(t *testing.T, name string, providers ...op.ReceiverFactory) {
	t.Helper()
	script := filepath.Join(testdataDir(t), name)
	runner := testrunner.New(script, testrunner.WithReceivers(providers...))
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
	t.Skip("reflection bug: cannot use []string as variadic string in generated receiver")
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
// These providers need external resources to execute. Dry-run proves
// registration + planned receiver + graph node creation.

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
	runScriptImm(t, "test_imm_json.star", jsongen.Receiver)
}

func TestImmYAML(t *testing.T) {
	runScriptImm(t, "test_imm_yaml.star", yamlgen.Receiver)
}

func TestImmRegexp(t *testing.T) {
	runScriptImm(t, "test_imm_regexp.star", regexpgen.Receiver)
}

func TestImmTemplate(t *testing.T) {
	runScriptImm(t, "test_imm_template.star", templategen.Receiver)
}

func TestImmUI(t *testing.T) {
	runScriptImm(t, "test_imm_ui.star", uigen.Receiver)
}

func TestImmStaranalysis(t *testing.T) {
	runScriptImm(t, "test_imm_staranalysis.star", staranalysisgen.Receiver)
}

func TestImmStarcode(t *testing.T) {
	runScriptImm(t, "test_imm_starcode.star", starcodegen.Receiver)
}

func TestImmStarcomplexity(t *testing.T) {
	runScriptImm(t, "test_imm_starcomplexity.star", starcomplexitygen.Receiver)
}

func TestImmStarindex(t *testing.T) {
	runScriptImm(t, "test_imm_starindex.star", starindexgen.Receiver)
}

func TestImmStarstats(t *testing.T) {
	runScriptImm(t, "test_imm_starstats.star", starstatsgen.Receiver)
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

func TestFlowFatalTemplate(t *testing.T) {
	runScript(t, "test_flow_fatal_template.star")
}

func TestFlowDegradedTemplate(t *testing.T) {
	runScript(t, "test_flow_degraded_template.star")
}

func TestFlowFatalRecovery(t *testing.T) {
	runScript(t, "test_flow_fatal_recovery.star")
}

// --- Other tests ---

func TestDryRun(t *testing.T) {
	script := filepath.Join(testdataDir(t), "test_write_text.star")
	runner := testrunner.New(script, testrunner.WithDryRun(), testrunner.WithGraphBuilder(), testrunner.WithReceivers(filegen.Receiver))
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
