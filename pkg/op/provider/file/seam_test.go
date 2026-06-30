// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// CompensateFileMutation dispatch arms not reached by the migrated per-action tests (which cover create/update/move):
// the default arm (an unrecognized kind) and the directory-delete arm (compensateRemoveDir, which has no forward
// producer after decision 8 dropped RemoveDir, so this is its only coverage).

// TestCompensateFileMutation_UnknownKind_Errors verifies the default dispatch arm: a receipt whose MutationKind is not
// one of the recognized values is a programming error, not a silent no-op.
func TestCompensateFileMutation_UnknownKind_Errors(t *testing.T) {

	dir := t.TempDir()
	p := testProvider(t, dir)
	resource := testFileResource(t, []byte("x"))

	receipt := NewReceipt(NewReceiptSpec(resource, MutationKind("bogus")))

	err := p.CompensateFileMutation(receipt)
	if err == nil {
		t.Fatal("CompensateFileMutation(unknown kind) = nil; want an error")
	}
	if !strings.Contains(err.Error(), "unknown kind") {
		t.Errorf("error = %q; want it to mention \"unknown kind\"", err)
	}
}

// TestCompensateFileMutation_DeleteDir_RecreatesDir verifies the MutationDeleteDir arm: compensating a recorded
// directory deletion recreates the directory.
func TestCompensateFileMutation_DeleteDir_RecreatesDir(t *testing.T) {

	dir := t.TempDir()
	p := testProvider(t, dir)
	runtimeEnvironment := p.RuntimeEnvironment()

	target := filepath.Join(dir, "sub")
	if err := os.Mkdir(target, 0o750); err != nil {
		t.Fatalf("seed dir: %v", err)
	}

	resource, err := DiscoverResource(runtimeEnvironment, target)
	if err != nil {
		t.Fatalf("DiscoverResource: %v", err)
	}

	if err := os.Remove(target); err != nil {
		t.Fatalf("remove dir: %v", err)
	}

	receipt := NewReceipt(NewReceiptSpec(resource, MutationDeleteDir))
	if err := p.CompensateFileMutation(receipt); err != nil {
		t.Fatalf("CompensateFileMutation: %v", err)
	}

	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat recreated dir: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("recreated path is not a directory")
	}
}

// WriteFile (slice 4) -- the exported streaming-write method -- paired with CompensateFileMutation as its undo.

// TestWriteFile_Create_RemovedOnCompensate writes a new file via WriteFile, then asserts compensation removes it.
func TestWriteFile_Create_RemovedOnCompensate(t *testing.T) {

	dir := t.TempDir()
	p := testProvider(t, dir)

	target, err := NewResource(p.RuntimeEnvironment(), nil, filepath.Join(dir, "new.txt"))
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	_, receipt, err := p.WriteFile(target, strings.NewReader("hello"), 0o644)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if got, err := os.ReadFile(target.SourcePath.Abs()); err != nil || string(got) != "hello" {
		t.Fatalf("written content = %q (err %v); want %q", got, err, "hello")
	}

	if err := p.CompensateFileMutation(receipt); err != nil {
		t.Fatalf("CompensateFileMutation: %v", err)
	}

	if _, err := os.Lstat(target.SourcePath.Abs()); !os.IsNotExist(err) {
		t.Errorf("created file still present after compensate (stat err = %v); want removed", err)
	}
}

// TestWriteFile_Update_RestoredOnCompensate overwrites an existing file via WriteFile, then asserts compensation
// restores the prior content from recovery.
func TestWriteFile_Update_RestoredOnCompensate(t *testing.T) {

	dir := t.TempDir()
	p := testProvider(t, dir)

	path := filepath.Join(dir, "exists.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	target, err := NewResource(p.RuntimeEnvironment(), nil, path)
	if err != nil {
		t.Fatalf("NewResource: %v", err)
	}

	_, receipt, err := p.WriteFile(target, strings.NewReader("new"), 0o644)
	if err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if got, _ := os.ReadFile(path); string(got) != "new" {
		t.Fatalf("written content = %q; want %q", got, "new")
	}

	if err := p.CompensateFileMutation(receipt); err != nil {
		t.Fatalf("CompensateFileMutation: %v", err)
	}

	if got, _ := os.ReadFile(path); string(got) != "old" {
		t.Errorf("content after compensate = %q; want %q (restored)", got, "old")
	}
}
