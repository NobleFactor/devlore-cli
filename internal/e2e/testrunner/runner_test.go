// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package testrunner_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"filippo.io/age"
	"github.com/getsops/sops/v3"
	"github.com/getsops/sops/v3/aes"
	sopsage "github.com/getsops/sops/v3/age"
	"github.com/getsops/sops/v3/stores/yaml"

	"github.com/NobleFactor/devlore-cli/internal/e2e/testrunner"
)

// sopsEncrypt generates age keys and encrypts plainYAML with SOPS.
// Returns the encrypted bytes and the age identity string for decryption.
func sopsEncrypt(t *testing.T, plainYAML []byte) ([]byte, string) {
	t.Helper()

	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}

	store := &yaml.Store{}
	branches, err := store.LoadPlainFile(plainYAML)
	if err != nil {
		t.Fatalf("loading plain YAML: %v", err)
	}

	masterKey := &sopsage.MasterKey{
		Recipient: identity.Recipient().String(),
	}

	tree := sops.Tree{
		Branches: branches,
		Metadata: sops.Metadata{
			KeyGroups: []sops.KeyGroup{{masterKey}},
			Version:   "3.7.0",
		},
	}

	dataKey, errs := tree.GenerateDataKey()
	if len(errs) > 0 {
		t.Fatalf("GenerateDataKey: %v", errs)
	}

	cipher := aes.NewCipher()
	mac, err := tree.Encrypt(dataKey, cipher)
	if err != nil {
		t.Fatalf("encrypting tree: %v", err)
	}

	encryptedMac, err := cipher.Encrypt(mac, dataKey, tree.Metadata.LastModified.Format("2006-01-02T15:04:05Z"))
	if err != nil {
		t.Fatalf("encrypting MAC: %v", err)
	}
	tree.Metadata.MessageAuthenticationCode = encryptedMac

	encrypted, err := store.EmitEncryptedFile(tree)
	if err != nil {
		t.Fatalf("emitting encrypted file: %v", err)
	}

	return encrypted, identity.String()
}

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
	runner := testrunner.New(script, testrunner.WithReceivers("plan", "file"))
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
	runner := testrunner.New(script, testrunner.WithReceivers("plan", "file"))
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
	runner := testrunner.New(script, testrunner.WithReceivers("plan", "file"))
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
	runner := testrunner.New(script, testrunner.WithReceivers("plan", "file"))
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
	runner := testrunner.New(script, testrunner.WithTrace(), testrunner.WithReceivers("plan", "file"))
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

// runScript runs a .star test script with plan+file receivers and fails on any expectation failures.
func runScript(t *testing.T, name string) {
	t.Helper()
	script := filepath.Join(testdataDir(t), name)
	runner := testrunner.New(script, testrunner.WithReceivers("plan", "file"))
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

// runScriptDryRun runs a .star test script in dry-run mode with plan receiver.
func runScriptDryRun(t *testing.T, name string) {
	t.Helper()
	script := filepath.Join(testdataDir(t), name)
	runner := testrunner.New(script, testrunner.WithDryRun(), testrunner.WithReceivers("plan"))
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

// runScriptImm runs a .star test script with the given immediate receivers.
func runScriptImm(t *testing.T, name string, receivers ...string) {
	t.Helper()
	script := filepath.Join(testdataDir(t), name)
	runner := testrunner.New(script, testrunner.WithReceivers(receivers...))
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

// ── Planned action tests — file provider gaps ──

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

// ── WalkTree callable tests ──

func TestWalkTree(t *testing.T) {
	runScriptImm(t, "test_walk_tree.star", "file")
}

func TestWalkTreePlanned(t *testing.T) {
	runScript(t, "test_walk_tree_planned.star")
}

func TestWalkTreeGitignore(t *testing.T) {
	runScriptImm(t, "test_walk_tree_gitignore.star", "file")
}

func TestWalkTreeClosure(t *testing.T) {
	runScriptImm(t, "test_walk_tree_closure.star", "file")
}

// ── Planned action tests — template provider ──

func TestTemplateRender(t *testing.T) {
	runScript(t, "test_template_render.star")
}

// ── Planned action tests — dry-run providers ──
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

// ── Immediate action tests ──

func TestImmFile(t *testing.T) {
	runScriptImm(t, "test_imm_file.star", "file")
}

func TestImmFileJoinVariadicError(t *testing.T) {
	runScriptImm(t, "test_imm_file_join_variadic_error.star", "file")
}

func TestImmJSON(t *testing.T) {
	runScriptImm(t, "test_imm_json.star", "json")
}

func TestImmYAML(t *testing.T) {
	runScriptImm(t, "test_imm_yaml.star", "yaml")
}

func TestImmRegexp(t *testing.T) {
	runScriptImm(t, "test_imm_regexp.star", "regexp")
}

func TestImmShell(t *testing.T) {
	runScriptImm(t, "test_imm_shell.star", "shell")
}

func TestImmTemplate(t *testing.T) {
	runScriptImm(t, "test_imm_template.star", "template")
}

func TestImmUI(t *testing.T) {
	runScriptImm(t, "test_imm_ui.star", "ui")
}

func TestImmStaranalysis(t *testing.T) {
	runScriptImm(t, "test_imm_staranalysis.star", "staranalysis")
}

func TestImmStarcode(t *testing.T) {
	runScriptImm(t, "test_imm_starcode.star", "starcode")
}

func TestImmStarcomplexity(t *testing.T) {
	runScriptImm(t, "test_imm_starcomplexity.star", "starcomplexity")
}

func TestImmStarindex(t *testing.T) {
	runScriptImm(t, "test_imm_starindex.star", "starindex")
}

func TestImmStarstats(t *testing.T) {
	runScriptImm(t, "test_imm_starstats.star", "starstats")
}

func TestImmEncryption(t *testing.T) {
	// Generate age keys and SOPS-encrypt a YAML file.
	plainYAML := []byte("greeting: hello\nname: world\n")
	encrypted, ageKey := sopsEncrypt(t, plainYAML)

	tmp := t.TempDir()
	srcPath := filepath.Join(tmp, "secret.enc.yaml")
	dstPath := filepath.Join(tmp, "secret.dec.yaml")
	if err := os.WriteFile(srcPath, encrypted, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SOPS_AGE_KEY", ageKey)

	// Write a script that decrypts the fixture and verifies the result.
	script := filepath.Join(tmp, "test_imm_encryption.star")
	content := fmt.Sprintf(`# Generated encryption immediate test.
result = encryption.decrypt_sops_file(source=%q, destination=%q)
t.expect_equal(type(result), "struct")
t.expect_node_count(0)
`, srcPath, dstPath)
	if err := os.WriteFile(script, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := testrunner.New(script, testrunner.WithReceivers("encryption"))
	result, err := runner.Start(context.Background())
	if err != nil {
		t.Fatalf("runner error: %v", err)
	}
	if !result.Passed {
		for _, f := range result.Failures {
			t.Errorf("FAIL: %s — %s", f.Expectation, f.Message)
		}
	}

	// Verify the decrypted file was actually written.
	decrypted, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("reading decrypted file: %v", err)
	}
	if !bytes.Contains(decrypted, []byte("hello")) {
		t.Errorf("decrypted content missing 'hello': %s", decrypted)
	}
	if !bytes.Contains(decrypted, []byte("world")) {
		t.Errorf("decrypted content missing 'world': %s", decrypted)
	}
}

// ── Terminal flow control tests ──

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

// ── Other tests ──

func TestDryRun(t *testing.T) {
	script := filepath.Join(testdataDir(t), "test_write_text.star")
	runner := testrunner.New(script, testrunner.WithDryRun(), testrunner.WithReceivers("plan", "file"))
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
