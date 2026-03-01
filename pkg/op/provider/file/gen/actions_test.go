// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package file_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	provider "github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	filegen "github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gen"
)

func newCtx(t *testing.T) *op.Context {
	t.Helper()
	return &op.Context{
		Context: context.Background(),
		Writer:  &bytes.Buffer{},
	}
}

func dryRunCtx(t *testing.T) *op.Context {
	t.Helper()
	return &op.Context{
		Context: context.Background(),
		DryRun:  true,
		Writer:  &bytes.Buffer{},
	}
}

func makeRegistry(t *testing.T, p *provider.Provider) *op.ActionRegistry {
	t.Helper()
	reg := op.NewActionRegistry()
	op.RegisterReflectedActions(reg, "file", p, filegen.Params)
	return reg
}

func getAction(t *testing.T, reg *op.ActionRegistry, name string) op.Action {
	t.Helper()
	a, ok := reg.Get(name)
	if !ok {
		t.Fatalf("action %q not registered", name)
	}
	return a
}

func getCompensable(t *testing.T, reg *op.ActionRegistry, name string) op.CompensableAction {
	t.Helper()
	a := getAction(t, reg, name)
	ca, ok := a.(op.CompensableAction)
	if !ok {
		t.Fatalf("action %q does not implement CompensableAction", name)
	}
	return ca
}

// ── Name ────────────────────────────────────────────────────────────────────────

func TestActionNames(t *testing.T) {
	reg := makeRegistry(t, &provider.Provider{})
	tests := []string{
		"file.backup", "file.copy", "file.glob", "file.link",
		"file.mkdir", "file.move", "file.read", "file.remove",
		"file.remove_all", "file.unlink", "file.write_bytes", "file.write_text",
	}
	for _, name := range tests {
		a := getAction(t, reg, name)
		if got := a.Name(); got != name {
			t.Errorf("Name() = %q, want %q", got, name)
		}
	}
}

// ── Register ────────────────────────────────────────────────────────────────────

func TestRegister(t *testing.T) {
	reg := makeRegistry(t, &provider.Provider{})

	expected := []string{
		"file.backup", "file.copy", "file.glob",
		"file.link", "file.mkdir", "file.move",
		"file.read", "file.remove", "file.remove_all",
		"file.unlink", "file.write_bytes", "file.write_text",
	}
	for _, name := range expected {
		if _, ok := reg.Get(name); !ok {
			t.Errorf("action %q not registered", name)
		}
	}
}

// ── WriteText ───────────────────────────────────────────────────────────────────

func TestWriteTextAction_Do(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "action-write.txt")

	reg := makeRegistry(t, &provider.Provider{})
	action := getAction(t, reg, "file.write_text")
	ctx := newCtx(t)
	slots := map[string]any{
		"destination": path,
		"content":     "hello from action",
		"mode":        os.FileMode(0o644),
	}

	result, undo, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	fileResource, ok := result.(provider.Resource)
	if !ok {
		t.Fatalf("result type = %T, want provider.Resource", result)
	}
	if fileResource.SourcePath != path {
		t.Errorf("result.SourcePath = %q, want %q", fileResource.SourcePath, path)
	}
	if undo == nil {
		t.Fatal("undo is nil, want non-nil")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "hello from action" {
		t.Errorf("file content = %q, want %q", got, "hello from action")
	}
}

func TestWriteTextAction_Undo(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "action-undo.txt")

	reg := makeRegistry(t, &provider.Provider{})
	action := getCompensable(t, reg, "file.write_text")
	ctx := newCtx(t)
	slots := map[string]any{
		"destination": path,
		"content":     "to be undone",
		"mode":        os.FileMode(0o644),
	}

	_, undo, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}

	if err := action.Undo(ctx, undo); err != nil {
		t.Fatalf("Undo() error = %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file still exists after Undo")
	}
}

func TestWriteTextAction_UndoNil(t *testing.T) {
	reg := makeRegistry(t, &provider.Provider{})
	action := getCompensable(t, reg, "file.write_text")
	if err := action.Undo(newCtx(t), nil); err != nil {
		t.Errorf("Undo(nil) = %v, want nil", err)
	}
}

func TestWriteTextAction_DryRun(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "dryrun.txt")

	reg := makeRegistry(t, &provider.Provider{})
	action := getAction(t, reg, "file.write_text")
	ctx := dryRunCtx(t)
	slots := map[string]any{
		"destination": path,
		"content":     "dry",
		"mode":        os.FileMode(0o644),
	}

	result, undo, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != nil {
		t.Errorf("dry-run result = %v, want nil", result)
	}
	if undo != nil {
		t.Errorf("dry-run undo = %v, want nil", undo)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file created during dry-run")
	}

	output := ctx.Writer.(*bytes.Buffer).String()
	if !strings.Contains(output, "[dry-run] file.write_text") {
		t.Errorf("dry-run output = %q, want to contain [dry-run] file.write_text", output)
	}
}

// ── WriteBytes ──────────────────────────────────────────────────────────────────

func TestWriteBytesAction_Do(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "action-bytes.bin")

	reg := makeRegistry(t, &provider.Provider{})
	action := getAction(t, reg, "file.write_bytes")
	ctx := newCtx(t)
	slots := map[string]any{
		"destination": path,
		"content":     "binary data",
		"mode":        os.FileMode(0o600),
	}

	result, undo, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	fileResource, ok := result.(provider.Resource)
	if !ok {
		t.Fatalf("result type = %T, want provider.Resource", result)
	}
	if fileResource.SourcePath != path {
		t.Errorf("result.SourcePath = %q, want %q", fileResource.SourcePath, path)
	}
	if undo == nil {
		t.Fatal("undo is nil")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "binary data" {
		t.Errorf("content = %q, want %q", got, "binary data")
	}
}

func TestWriteBytesAction_Undo(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "action-bytes-undo.bin")

	reg := makeRegistry(t, &provider.Provider{})
	action := getCompensable(t, reg, "file.write_bytes")
	ctx := newCtx(t)
	slots := map[string]any{
		"destination": path,
		"content":     "to undo",
		"mode":        os.FileMode(0o600),
	}

	_, undo, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}

	if err := action.Undo(ctx, undo); err != nil {
		t.Fatalf("Undo() error = %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file still exists after Undo")
	}
}

func TestWriteBytesAction_DryRun(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "dryrun.bin")

	reg := makeRegistry(t, &provider.Provider{})
	action := getAction(t, reg, "file.write_bytes")
	ctx := dryRunCtx(t)
	slots := map[string]any{
		"destination": path,
		"content":     "dry",
		"mode":        os.FileMode(0o600),
	}

	result, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != nil {
		t.Errorf("dry-run result = %v, want nil", result)
	}

	output := ctx.Writer.(*bytes.Buffer).String()
	if !strings.Contains(output, "[dry-run] file.write_bytes") {
		t.Errorf("dry-run output = %q, want to contain [dry-run] file.write_bytes", output)
	}
}

// ── Copy ────────────────────────────────────────────────────────────────────────

func TestCopyAction_Do(t *testing.T) {
	tmp := t.TempDir()
	source := filepath.Join(tmp, "copy-source.txt")
	if err := os.WriteFile(source, []byte("copy me"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(tmp, "copy-dest.txt")

	reg := makeRegistry(t, &provider.Provider{})
	action := getAction(t, reg, "file.copy")
	ctx := newCtx(t)
	slots := map[string]any{
		"source_file":           source,
		"destination_filename":  dest,
		"destination_file_mode": os.FileMode(0o644),
	}

	result, undo, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	fileResource, ok := result.(provider.Resource)
	if !ok {
		t.Fatalf("result type = %T, want provider.Resource", result)
	}
	if fileResource.SourcePath != dest {
		t.Errorf("result.SourcePath = %q, want %q", fileResource.SourcePath, dest)
	}
	if undo == nil {
		t.Fatal("undo is nil, want non-nil")
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "copy me" {
		t.Errorf("file content = %q, want %q", got, "copy me")
	}
}

func TestCopyAction_Undo(t *testing.T) {
	tmp := t.TempDir()
	source := filepath.Join(tmp, "copy-undo-src.txt")
	if err := os.WriteFile(source, []byte("undo copy"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(tmp, "copy-undo-dst.txt")

	reg := makeRegistry(t, &provider.Provider{})
	action := getCompensable(t, reg, "file.copy")
	ctx := newCtx(t)
	slots := map[string]any{
		"source_file":           source,
		"destination_filename":  dest,
		"destination_file_mode": os.FileMode(0o644),
	}

	_, undo, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}

	if err := action.Undo(ctx, undo); err != nil {
		t.Fatalf("Undo() error = %v", err)
	}

	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Error("file still exists after Undo")
	}
}

func TestCopyAction_DryRun(t *testing.T) {
	reg := makeRegistry(t, &provider.Provider{})
	action := getAction(t, reg, "file.copy")
	ctx := dryRunCtx(t)
	slots := map[string]any{
		"source_file":           "/tmp/copy-src.txt",
		"destination_filename":  "/tmp/copy-dst.txt",
		"destination_file_mode": os.FileMode(0o644),
	}

	result, undo, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != nil {
		t.Errorf("dry-run result = %v, want nil", result)
	}
	if undo != nil {
		t.Errorf("dry-run undo = %v, want nil", undo)
	}

	output := ctx.Writer.(*bytes.Buffer).String()
	if !strings.Contains(output, "[dry-run] file.copy") {
		t.Errorf("dry-run output = %q, want to contain [dry-run] file.copy", output)
	}
}

// ── Link ────────────────────────────────────────────────────────────────────────

func TestLinkAction_Do(t *testing.T) {
	tmp := t.TempDir()
	source := filepath.Join(tmp, "source.txt")
	if err := os.WriteFile(source, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(tmp, "link.txt")

	reg := makeRegistry(t, &provider.Provider{})
	action := getAction(t, reg, "file.link")
	ctx := newCtx(t)
	slots := map[string]any{
		"source": source,
		"path":   linkPath,
	}

	result, undo, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != linkPath {
		t.Errorf("result = %v, want %q", result, linkPath)
	}
	if undo == nil {
		t.Fatal("undo is nil")
	}

	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink() error = %v", err)
	}
	if target != source {
		t.Errorf("link target = %q, want %q", target, source)
	}
}

func TestLinkAction_Undo(t *testing.T) {
	tmp := t.TempDir()
	source := filepath.Join(tmp, "source.txt")
	if err := os.WriteFile(source, []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(tmp, "link-undo.txt")

	reg := makeRegistry(t, &provider.Provider{})
	action := getCompensable(t, reg, "file.link")
	ctx := newCtx(t)
	slots := map[string]any{
		"source": source,
		"path":   linkPath,
	}

	_, undo, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}

	if err := action.Undo(ctx, undo); err != nil {
		t.Fatalf("Undo() error = %v", err)
	}

	if _, err := os.Lstat(linkPath); !os.IsNotExist(err) {
		t.Error("symlink still exists after Undo")
	}
}

func TestLinkAction_DryRun(t *testing.T) {
	reg := makeRegistry(t, &provider.Provider{})
	action := getAction(t, reg, "file.link")
	ctx := dryRunCtx(t)
	slots := map[string]any{
		"source": "/tmp/source",
		"path":   "/tmp/link",
	}

	result, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != nil {
		t.Errorf("dry-run result = %v, want nil", result)
	}

	output := ctx.Writer.(*bytes.Buffer).String()
	if !strings.Contains(output, "[dry-run] file.link") {
		t.Errorf("dry-run output = %q, want to contain [dry-run] file.link", output)
	}
}

// ── Move ────────────────────────────────────────────────────────────────────────

func TestMoveAction_Do(t *testing.T) {
	tmp := t.TempDir()
	source := filepath.Join(tmp, "move-src.txt")
	if err := os.WriteFile(source, []byte("move me"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(tmp, "move-dst.txt")

	reg := makeRegistry(t, &provider.Provider{})
	action := getAction(t, reg, "file.move")
	ctx := newCtx(t)
	slots := map[string]any{
		"source":      source,
		"destination": dest,
	}

	result, undo, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != dest {
		t.Errorf("result = %v, want %q", result, dest)
	}
	if undo == nil {
		t.Fatal("undo is nil")
	}

	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Error("source still exists after Move")
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != "move me" {
		t.Errorf("content = %q, want %q", got, "move me")
	}
}

func TestMoveAction_Undo(t *testing.T) {
	tmp := t.TempDir()
	source := filepath.Join(tmp, "move-undo-src.txt")
	if err := os.WriteFile(source, []byte("move undo"), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(tmp, "move-undo-dst.txt")

	reg := makeRegistry(t, &provider.Provider{})
	action := getCompensable(t, reg, "file.move")
	ctx := newCtx(t)
	slots := map[string]any{
		"source":      source,
		"destination": dest,
	}

	_, undo, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}

	if err := action.Undo(ctx, undo); err != nil {
		t.Fatalf("Undo() error = %v", err)
	}

	got, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("source not restored: %v", err)
	}
	if string(got) != "move undo" {
		t.Errorf("restored content = %q, want %q", got, "move undo")
	}
}

func TestMoveAction_DryRun(t *testing.T) {
	reg := makeRegistry(t, &provider.Provider{})
	action := getAction(t, reg, "file.move")
	ctx := dryRunCtx(t)
	slots := map[string]any{
		"source":      "/tmp/src",
		"destination": "/tmp/dst",
	}

	result, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != nil {
		t.Errorf("dry-run result = %v, want nil", result)
	}

	output := ctx.Writer.(*bytes.Buffer).String()
	if !strings.Contains(output, "[dry-run] file.move") {
		t.Errorf("dry-run output = %q, want to contain [dry-run] file.move", output)
	}
}

// ── Backup ──────────────────────────────────────────────────────────────────────

func TestBackupAction_Do(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "backup-target.txt")
	if err := os.WriteFile(path, []byte("backup me"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := makeRegistry(t, &provider.Provider{})
	action := getAction(t, reg, "file.backup")
	ctx := newCtx(t)
	slots := map[string]any{
		"path":          path,
		"backup_suffix": ".bak",
	}

	result, undo, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}

	backupPath, ok := result.(string)
	if !ok || backupPath == "" {
		t.Fatalf("result = %v, want non-empty string", result)
	}
	if undo == nil {
		t.Fatal("undo is nil")
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("original still exists after Backup")
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Errorf("backup not found at %q: %v", backupPath, err)
	}
}

func TestBackupAction_Undo(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "backup-undo.txt")
	if err := os.WriteFile(path, []byte("restore me"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := makeRegistry(t, &provider.Provider{})
	action := getCompensable(t, reg, "file.backup")
	ctx := newCtx(t)
	slots := map[string]any{
		"path":          path,
		"backup_suffix": ".bak",
	}

	_, undo, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}

	if err := action.Undo(ctx, undo); err != nil {
		t.Fatalf("Undo() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("original not restored: %v", err)
	}
	if string(got) != "restore me" {
		t.Errorf("restored content = %q, want %q", got, "restore me")
	}
}

func TestBackupAction_DryRun(t *testing.T) {
	reg := makeRegistry(t, &provider.Provider{})
	action := getAction(t, reg, "file.backup")
	ctx := dryRunCtx(t)
	slots := map[string]any{
		"path":          "/tmp/backup.txt",
		"backup_suffix": ".bak",
	}

	result, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != nil {
		t.Errorf("dry-run result = %v, want nil", result)
	}

	output := ctx.Writer.(*bytes.Buffer).String()
	if !strings.Contains(output, "[dry-run] file.backup") {
		t.Errorf("dry-run output = %q, want to contain [dry-run] file.backup", output)
	}
}

// ── Remove ──────────────────────────────────────────────────────────────────────

func TestRemoveAction_Do(t *testing.T) {
	t.Skip("blocked: recovery site bug (#164)")
}

func TestRemoveAction_DryRun(t *testing.T) {
	reg := makeRegistry(t, &provider.Provider{})
	action := getAction(t, reg, "file.remove")
	ctx := dryRunCtx(t)
	slots := map[string]any{
		"path":           "/tmp/remove.txt",
		"prune":          false,
		"prune_boundary": "",
	}

	result, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != nil {
		t.Errorf("dry-run result = %v, want nil", result)
	}

	output := ctx.Writer.(*bytes.Buffer).String()
	if !strings.Contains(output, "[dry-run] file.remove") {
		t.Errorf("dry-run output = %q, want to contain [dry-run] file.remove", output)
	}
}

// ── RemoveAll ───────────────────────────────────────────────────────────────────

func TestRemoveAllAction_Do(t *testing.T) {
	t.Skip("blocked: recovery site bug (#164)")
}

func TestRemoveAllAction_DryRun(t *testing.T) {
	reg := makeRegistry(t, &provider.Provider{})
	action := getAction(t, reg, "file.remove_all")
	ctx := dryRunCtx(t)
	slots := map[string]any{
		"path":           "/tmp/remove-all",
		"prune":          false,
		"prune_boundary": "",
	}

	result, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != nil {
		t.Errorf("dry-run result = %v, want nil", result)
	}

	output := ctx.Writer.(*bytes.Buffer).String()
	if !strings.Contains(output, "[dry-run] file.remove_all") {
		t.Errorf("dry-run output = %q, want to contain [dry-run] file.remove_all", output)
	}
}

// ── Unlink ──────────────────────────────────────────────────────────────────────

func TestUnlinkAction_Do(t *testing.T) {
	t.Skip("blocked: recovery site bug (#164)")
}

func TestUnlinkAction_DryRun(t *testing.T) {
	reg := makeRegistry(t, &provider.Provider{})
	action := getAction(t, reg, "file.unlink")
	ctx := dryRunCtx(t)
	slots := map[string]any{
		"path":           "/tmp/unlink.txt",
		"prune":          false,
		"prune_boundary": "",
	}

	result, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != nil {
		t.Errorf("dry-run result = %v, want nil", result)
	}

	output := ctx.Writer.(*bytes.Buffer).String()
	if !strings.Contains(output, "[dry-run] file.unlink") {
		t.Errorf("dry-run output = %q, want to contain [dry-run] file.unlink", output)
	}
}

// ── Glob ────────────────────────────────────────────────────────────────────────

func TestGlobAction_Do(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "b.txt"), []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := makeRegistry(t, &provider.Provider{Root: tmp})
	action := getAction(t, reg, "file.glob")
	ctx := newCtx(t)
	slots := map[string]any{
		"pattern":         filepath.Join(tmp, "*.txt"),
		"honor_gitignore": false,
	}

	result, undo, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if undo != nil {
		t.Errorf("undo = %v, want nil (Glob is not compensable)", undo)
	}

	matches, ok := result.([]string)
	if !ok {
		t.Fatalf("result type = %T, want []string", result)
	}
	if len(matches) != 2 {
		t.Errorf("len(matches) = %d, want 2", len(matches))
	}
}

func TestGlobAction_DryRun(t *testing.T) {
	reg := makeRegistry(t, &provider.Provider{})
	action := getAction(t, reg, "file.glob")
	ctx := dryRunCtx(t)
	slots := map[string]any{
		"pattern":         "/tmp/*.txt",
		"honor_gitignore": false,
	}

	result, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != nil {
		t.Errorf("dry-run result = %v, want nil", result)
	}

	output := ctx.Writer.(*bytes.Buffer).String()
	if !strings.Contains(output, "[dry-run] file.glob") {
		t.Errorf("dry-run output = %q, want to contain [dry-run] file.glob", output)
	}
}

// ── Mkdir ───────────────────────────────────────────────────────────────────────

func TestMkdirAction_Do(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "action-mkdir")

	reg := makeRegistry(t, &provider.Provider{})
	action := getAction(t, reg, "file.mkdir")
	ctx := newCtx(t)
	slots := map[string]any{
		"path": path,
		"mode": os.FileMode(0o755),
	}

	result, undo, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if undo != nil {
		t.Errorf("undo = %v, want nil (Mkdir is not compensable)", undo)
	}

	resultStr, ok := result.(string)
	if !ok {
		t.Fatalf("result type = %T, want string", result)
	}
	if resultStr == "" {
		t.Error("result is empty")
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if !info.IsDir() {
		t.Error("path is not a directory")
	}
}

func TestMkdirAction_DryRun(t *testing.T) {
	reg := makeRegistry(t, &provider.Provider{})
	action := getAction(t, reg, "file.mkdir")
	ctx := dryRunCtx(t)
	slots := map[string]any{
		"path": "/tmp/dryrun-dir",
		"mode": os.FileMode(0o755),
	}

	result, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != nil {
		t.Errorf("dry-run result = %v, want nil", result)
	}

	output := ctx.Writer.(*bytes.Buffer).String()
	if !strings.Contains(output, "[dry-run] file.mkdir") {
		t.Errorf("dry-run output = %q, want to contain [dry-run] file.mkdir", output)
	}
}

// ── Read ────────────────────────────────────────────────────────────────────────

func TestReadAction_Do(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "action-read.txt")
	if err := os.WriteFile(path, []byte("readable"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := makeRegistry(t, &provider.Provider{})
	action := getAction(t, reg, "file.read")
	ctx := newCtx(t)
	slots := map[string]any{
		"path": path,
	}

	result, undo, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if undo != nil {
		t.Errorf("undo = %v, want nil (Read is not compensable)", undo)
	}
	if result == nil {
		t.Error("result is nil, want Resource")
	}
}

func TestReadAction_DryRun(t *testing.T) {
	reg := makeRegistry(t, &provider.Provider{})
	action := getAction(t, reg, "file.read")
	ctx := dryRunCtx(t)
	slots := map[string]any{
		"path": "/tmp/dryrun-read.txt",
	}

	result, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != nil {
		t.Errorf("dry-run result = %v, want nil", result)
	}

	output := ctx.Writer.(*bytes.Buffer).String()
	if !strings.Contains(output, "[dry-run] file.read") {
		t.Errorf("dry-run output = %q, want to contain [dry-run] file.read", output)
	}
}

// ── Nil Undo for all compensable actions ────────────────────────────────────────

func TestCompensableActions_UndoNil(t *testing.T) {
	reg := makeRegistry(t, &provider.Provider{})
	ctx := newCtx(t)

	names := []string{
		"file.backup", "file.copy", "file.link",
		"file.move", "file.remove", "file.remove_all",
		"file.unlink", "file.write_bytes", "file.write_text",
	}

	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			ca := getCompensable(t, reg, name)
			if err := ca.Undo(ctx, nil); err != nil {
				t.Errorf("Undo(nil) = %v, want nil", err)
			}
		})
	}
}
