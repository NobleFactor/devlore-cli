// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package inventory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	"github.com/google/uuid"
)

// The file-mutation compensation seam (phase-8, file-mutation-receipts slice 3). Every other file compensation test
// calls Provider.CompensateFileMutation directly, and compensator_test.go only checks CompensatorByName resolves.
// These exercise the real path end-to-end: a *file.Receipt pushed onto a provider-agnostic op.RecoveryStack unwinds by
// resolving its constructor-stamped compensatingAction through the registry's compensator-name index to
// file.CompensateFileMutation. The test lives in inventory because that is where the gen blank-imports populate the
// registry the index reads.

// seamEnv builds a runtime environment rooted at dir with the catalog and recovery site a file compensation needs.
func seamEnv(t *testing.T, dir string) *op.RuntimeEnvironment {
	t.Helper()

	runtimeEnvironment := &op.RuntimeEnvironment{
		Root:            fsroot.OpenWritableUnconfined(dir),
		ResourceCatalog: op.NewResourceCatalog(),
	}
	runtimeEnvironment.RecoverySite = op.NewRecoverySite(runtimeEnvironment)
	return runtimeEnvironment
}

// TestRecoveryStackUnwind_FileReceiptCreate_RemovesViaIndex proves the seam for a create mutation: a *file.Receipt with
// no dispatching forwardAction unwinds via the compensator-name index to file.CompensateFileMutation, which removes the
// created file. The empty forwardAction is the point — compensation follows the constructor-stamped compensatingAction,
// not the dispatcher, which is exactly why an archive.extract-built receipt compensates as a file mutation.
func TestRecoveryStackUnwind_FileReceiptCreate_RemovesViaIndex(t *testing.T) {

	dir := t.TempDir()
	runtimeEnvironment := seamEnv(t, dir)

	target := filepath.Join(dir, "created.txt")
	if err := os.WriteFile(target, []byte("new content"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	resource, err := file.DiscoverResource(runtimeEnvironment, target)
	if err != nil {
		t.Fatalf("DiscoverResource: %v", err)
	}

	receipt := file.NewReceipt(file.NewReceiptSpec(resource, file.MutationCreateFile))
	if err := receipt.Commit(nil, nil, receipt, nil); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// The receipt names its undo via compensatingAction (constructor-stamped), decoupled from forwardAction (empty here
	// — definitively not a file action).
	if got := receipt.CompensatingAction(); got != "file.compensate_file_mutation" {
		t.Fatalf("CompensatingAction() = %q; want file.compensate_file_mutation", got)
	}
	if got := receipt.ForwardAction(); got != "" {
		t.Fatalf("ForwardAction() = %q; want empty (no file dispatcher)", got)
	}

	stack := op.NewRecoveryStack()
	if err := stack.Push(receipt, runtimeEnvironment); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if err := stack.Unwind(); err != nil {
		t.Fatalf("Unwind: %v", err)
	}

	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		t.Errorf("created file still present after Unwind (stat err = %v); want it removed", err)
	}
}

// TestRecoveryStackUnwind_FileReceiptUpdate_RestoresViaIndex proves the seam for an update mutation: an overwrite whose
// prior content was archived to recovery unwinds, through the same index, to a restore of that prior content. This is
// the displaced-content path archive.extract relies on for files it overwrites.
func TestRecoveryStackUnwind_FileReceiptUpdate_RestoresViaIndex(t *testing.T) {

	dir := t.TempDir()
	runtimeEnvironment := seamEnv(t, dir)

	target := filepath.Join(dir, "updated.txt")
	if err := os.WriteFile(target, []byte("new content"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	// Simulate the update having displaced the prior content to the recovery store.
	recoveryID := uuid.Must(uuid.NewV7()).String()
	recoveryPath := filepath.Join(dir, ".devlore", "recovery", recoveryID)
	if err := os.MkdirAll(filepath.Dir(recoveryPath), 0o700); err != nil {
		t.Fatalf("seed recovery dir: %v", err)
	}
	if err := os.WriteFile(recoveryPath, []byte("old content"), 0o644); err != nil {
		t.Fatalf("seed recovery file: %v", err)
	}

	resource, err := file.DiscoverResource(runtimeEnvironment, target)
	if err != nil {
		t.Fatalf("DiscoverResource: %v", err)
	}

	receipt := file.NewReceipt(file.NewReceiptSpec(resource, file.MutationUpdateFile).WithRecovery(recoveryID, op.Digest{}))
	if err := receipt.Commit(nil, nil, receipt, nil); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	stack := op.NewRecoveryStack()
	if err := stack.Push(receipt, runtimeEnvironment); err != nil {
		t.Fatalf("Push: %v", err)
	}
	if err := stack.Unwind(); err != nil {
		t.Fatalf("Unwind: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read restored file: %v", err)
	}
	if string(got) != "old content" {
		t.Errorf("restored content = %q; want %q", got, "old content")
	}
}
