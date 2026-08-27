// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

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

// testdataDir returns the absolute path to the data/ directory next to this test file.
//
// Parameters:
//   - `t`: the test handle; fatal-fails the test if runtime.Caller cannot locate the source file.
//
// Returns:
//   - `string`: the absolute path to the testdata directory.
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

func TestGatherBasic(t *testing.T) {
	runScript(t, "test_gather_basic.star")
}

func TestGatherConcurrency(t *testing.T) {
	runScript(t, "test_gather_concurrency.star")
}

func TestGatherAdvanced(t *testing.T) {
	runScript(t, "test_gather_advanced.star")
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

// TestChoose_UnchosenInvocationBranchDoesNotRun is the step-10 goal proof: a side-effecting when or then on an
// unchosen or after-the-match branch must not execute — the first-truthy short-circuit is the graph topology itself.
func TestChoose_UnchosenInvocationBranchDoesNotRun(t *testing.T) {
	runScript(t, "test_choose_unchosen_branch.star")
}

// --- plan.wait_until (step 12: predicate-container subgraph re-evaluated each poll) ---

func TestWaitUntil(t *testing.T) {
	runScript(t, "test_wait_until.star")
}

func TestWaitUntilTimeout(t *testing.T) {
	runScript(t, "test_wait_until_timeout.star")
}

func TestChooseLiterals(t *testing.T) {
	runScript(t, "test_choose_literals.star")
}

func TestChoosePredicates(t *testing.T) {
	runScript(t, "test_choose_predicates.star")
}

// runScript runs a .star test script with all providers and fails on any expectation failures.
// runScript runs the named .star fixture under a graph-builder runner and reports failures via t.Errorf.
//
// Parameters:
//   - `t`: the test handle.
//   - `name`: the .star fixture filename under testdataDir, e.g. "test_hello.star".
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

// runScriptDryRun runs the named .star fixture in dry-run mode with the graph builder enabled.
//
// Parameters:
//   - `t`: the test handle.
//   - `name`: the .star fixture filename under testdataDir.
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

// runScriptImm runs the named .star fixture in immediate mode (no graph builder).
//
// Parameters:
//   - `t`: the test handle.
//   - `name`: the .star fixture filename under testdataDir.
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

func TestFileRemoveLink(t *testing.T) {
	runScript(t, "test_file_remove_link.star")
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

func TestWalkTree_Planned(t *testing.T) {
	runScript(t, "test_walk_tree_planned.star")
}

// --- function provider ---

// TestFunctionCall_OverWalkTree is function.Provider's only end-to-end coverage.
//
// `function.call` is the provider's sole method, and until this fixture nothing exercised it. The function
// resource appeared only as a callback other providers accept — walk_tree's reducer, choose's lambdas — never
// as the thing being dispatched.
func TestFunctionCall_OverWalkTree(t *testing.T) {
	runScript(t, "test_function_call_walk_tree.star")
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
	runScriptDryRun(t, "test_regex.star")
}

// --- Immediate action tests ---

func TestImmediateFile(t *testing.T) {
	runScriptImm(t, "test_imm_file.star")
}

func TestImmediateJSON(t *testing.T) {
	runScriptImm(t, "test_imm_json.star")
}

func TestImmYAML(t *testing.T) {
	runScriptImm(t, "test_imm_yaml.star")
}

func TestImmRegexp(t *testing.T) {
	runScriptImm(t, "test_imm_regex.star")
}

func TestImmTemplate(t *testing.T) {
	runScriptImm(t, "test_imm_template.star")
}

func TestImmUI(t *testing.T) {
	runScriptImm(t, "test_imm_ui.star")
}

func TestImmUI_PrintReplacesBuiltin(t *testing.T) {
	runScriptImm(t, "test_imm_ui_print_replaces_builtin.star")
}

func TestImmUI_OneString(t *testing.T) {
	runScriptImm(t, "test_imm_ui_one_string.star")
}

func TestImmUI_Fail(t *testing.T) {
	runScriptImm(t, "test_imm_ui_fail.star")
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

func TestOrphanUnattached(t *testing.T) {
	runScript(t, "test_orphan_unattached.star")
}

func TestGatherProjection(t *testing.T) {
	runScript(t, "test_gather_projection.star")
}

func TestGatherProjectionMissingField(t *testing.T) {
	runScript(t, "test_gather_projection_missing_field.star")
}

func TestChooseInGather(t *testing.T) {
	runScript(t, "test_choose_in_gather.star")
}

func TestPromiseTypeMismatch(t *testing.T) {
	runScript(t, "test_promise_type_mismatch.star")
}

// --- writ adopt integration tests — wired Phase 6.A for baseline capture ---

func TestWritAdoptHappyPath(t *testing.T) {
	runScript(t, "test_writ_adopt.star")
}

func TestWritAdoptMissingRequired(t *testing.T) {
	runScript(t, "test_writ_adopt_missing_required.star")
}

func TestWritAdoptOriginFull(t *testing.T) {
	runScript(t, "test_writ_adopt_origin_full.star")
}

func TestWritAdoptOriginNamespace(t *testing.T) {
	runScript(t, "test_writ_adopt_origin_namespace.star")
}

func TestWritAdoptPrecedence(t *testing.T) {
	runScript(t, "test_writ_adopt_precedence.star")
}

func TestWritAdoptSubgraph(t *testing.T) {
	runScript(t, "test_writ_adopt_subgraph.star")
}

func TestWritAdoptTypeMismatch(t *testing.T) {
	runScript(t, "test_writ_adopt_type_mismatch.star")
}

// --- Judgment scenarios (docs/plans/resource-construction.md) ---
//
// Predictions authored before implementation; the implementation is correct when the harness observes
// exactly the prediction. These run in CI as the standing evidence the rulings require.

func TestGraphCatalogContract(t *testing.T) {
	runScript(t, "test_graph_catalog_contract.star")
}

func TestJudgmentScenario1_DeleteThenCopy(t *testing.T) {
	runScript(t, "test_judgment_1_delete_then_copy.star")
}

func TestJudgmentPreflightFailFast(t *testing.T) {
	runScript(t, "test_judgment_preflight_fail_fast.star")
}

func TestJudgmentScopedClaims(t *testing.T) {
	runScript(t, "test_judgment_scoped_claims.star")
}

func TestJudgmentGoneTolerance(t *testing.T) {
	runScript(t, "test_judgment_gone_tolerance.star")
}

func TestJudgmentPromiseOrdering(t *testing.T) {
	runScript(t, "test_judgment_promise_ordering.star")
}

func TestJudgmentDiscoverAfterExec(t *testing.T) {
	runScript(t, "test_judgment_discover_after_exec.star")
}

func TestJudgmentReloadDispatch(t *testing.T) {
	runScript(t, "test_judgment_reload_dispatch.star")
}

func TestJudgmentDoctoredChecksum(t *testing.T) {
	runScript(t, "test_judgment_doctored_checksum.star")
}

func TestJudgmentDispatchMiss(t *testing.T) {
	runScript(t, "test_judgment_dispatch_miss.star")
}

func TestJudgmentStringPromiseRefusal(t *testing.T) {
	runScript(t, "test_judgment_string_promise_refusal.star")
}

func TestJudgmentDiscoverKindVerdict(t *testing.T) {
	runScript(t, "test_judgment_discover_kind_verdict.star")
}

func TestJudgmentEntryDefaultConsumerMismatch(t *testing.T) {
	runScript(t, "test_judgment_entry_default_consumer_mismatch.star")
}

func TestJudgmentLstatStatPair(t *testing.T) {
	runScript(t, "test_judgment_lstat_stat_pair.star")
}

func TestJudgmentResolveDangling(t *testing.T) {
	runScript(t, "test_judgment_resolve_dangling.star")
}

func TestJudgmentResolveEscape(t *testing.T) {
	runScript(t, "test_judgment_resolve_escape.star")
}

func TestJudgmentRuntimeEscapeRefusal(t *testing.T) {
	runScript(t, "test_judgment_runtime_escape_refusal.star")
}

func TestJudgmentClaimedAndDiscovered(t *testing.T) {
	runScript(t, "test_judgment_claimed_and_discovered.star")
}

func TestJudgmentDiscoveredThenDestroyed(t *testing.T) {
	runScript(t, "test_judgment_discovered_then_destroyed.star")
}

func TestJudgmentKindHonestActivation(t *testing.T) {
	runScript(t, "test_judgment_kind_honest_activation.star")
}

func TestJudgmentInterfaceSlotMints(t *testing.T) {
	runScript(t, "test_judgment_interface_slot_mints.star")
}

func TestJudgmentMoveAnyKind(t *testing.T) {
	runScript(t, "test_judgment_move_any_kind.star")
}

func TestJudgmentMoveMissingSource(t *testing.T) {
	runScript(t, "test_judgment_move_missing_source.star")
}

func TestJudgmentRemoveAnyKind(t *testing.T) {
	runScript(t, "test_judgment_remove_any_kind.star")
}
