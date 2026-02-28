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

// ── Name ────────────────────────────────────────────────────────────────────────

func TestActionNames(t *testing.T) {
	p := &provider.Provider{}
	tests := []struct {
		action op.Action
		want   string
	}{
		{&filegen.Backup{Impl: p}, "file.backup"},
		{&filegen.Copy{Impl: p}, "file.copy"},
		{&filegen.Exists{Impl: p}, "file.exists"},
		{&filegen.Link{Impl: p}, "file.link"},
		{&filegen.Move{Impl: p}, "file.move"},
		{&filegen.Remove{Impl: p}, "file.remove"},
		{&filegen.RemoveAll{Impl: p}, "file.remove_all"},
		{&filegen.Unlink{Impl: p}, "file.unlink"},
		{&filegen.WriteBytes{Impl: p}, "file.write_bytes"},
		{&filegen.WriteText{Impl: p}, "file.write_text"},
		{&filegen.Glob{Impl: p}, "file.glob"},
		{&filegen.IsDir{Impl: p}, "file.is_dir"},
		{&filegen.IsFile{Impl: p}, "file.is_file"},
		{&filegen.Join{Impl: p}, "file.join"},
		{&filegen.Mkdir{Impl: p}, "file.mkdir"},
		{&filegen.NameAction{Impl: p}, "file.name"},
		{&filegen.Parent{Impl: p}, "file.parent"},
		{&filegen.Read{Impl: p}, "file.read"},
	}
	for _, tt := range tests {
		if got := tt.action.Name(); got != tt.want {
			t.Errorf("%T.Name() = %q, want %q", tt.action, got, tt.want)
		}
	}
}

// ── Register ────────────────────────────────────────────────────────────────────

func TestRegister(t *testing.T) {
	reg := op.NewActionRegistry()
	filegen.Register(reg)

	expected := []string{
		"file.backup", "file.copy", "file.exists", "file.glob",
		"file.is_dir", "file.is_file", "file.join", "file.link",
		"file.mkdir", "file.move", "file.name", "file.parent",
		"file.read", "file.remove", "file.remove_all", "file.unlink",
		"file.write_bytes", "file.write_text",
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

	action := &filegen.WriteText{Impl: &provider.Provider{}}
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
	blob, ok := result.(provider.Blob)
	if !ok {
		t.Fatalf("result type = %T, want provider.Blob", result)
	}
	if blob.SourcePath != path {
		t.Errorf("result.SourcePath = %q, want %q", blob.SourcePath, path)
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

	action := &filegen.WriteText{Impl: &provider.Provider{}}
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
	action := &filegen.WriteText{Impl: &provider.Provider{}}
	if err := action.Undo(newCtx(t), nil); err != nil {
		t.Errorf("Undo(nil) = %v, want nil", err)
	}
}

func TestWriteTextAction_DryRun(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "dryrun.txt")

	action := &filegen.WriteText{Impl: &provider.Provider{}}
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

	action := &filegen.WriteBytes{Impl: &provider.Provider{}}
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
	blob, ok := result.(provider.Blob)
	if !ok {
		t.Fatalf("result type = %T, want provider.Blob", result)
	}
	if blob.SourcePath != path {
		t.Errorf("result.SourcePath = %q, want %q", blob.SourcePath, path)
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

	action := &filegen.WriteBytes{Impl: &provider.Provider{}}
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

	action := &filegen.WriteBytes{Impl: &provider.Provider{}}
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

	action := &filegen.Copy{Impl: &provider.Provider{}}
	ctx := newCtx(t)
	slots := map[string]any{
		"destination": dest,
		"source":      source,
		"mode":        os.FileMode(0o644),
	}

	result, undo, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	blob, ok := result.(provider.Blob)
	if !ok {
		t.Fatalf("result type = %T, want provider.Blob", result)
	}
	if blob.SourcePath != dest {
		t.Errorf("result.SourcePath = %q, want %q", blob.SourcePath, dest)
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

	action := &filegen.Copy{Impl: &provider.Provider{}}
	ctx := newCtx(t)
	slots := map[string]any{
		"destination": dest,
		"source":      source,
		"mode":        os.FileMode(0o644),
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
	action := &filegen.Copy{Impl: &provider.Provider{}}
	ctx := dryRunCtx(t)
	slots := map[string]any{
		"destination": "/tmp/copy-dst.txt",
		"source":      "/tmp/copy-src.txt",
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

	action := &filegen.Link{Impl: &provider.Provider{}}
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

	action := &filegen.Link{Impl: &provider.Provider{}}
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
	action := &filegen.Link{Impl: &provider.Provider{}}
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

	action := &filegen.Move{Impl: &provider.Provider{}}
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

	action := &filegen.Move{Impl: &provider.Provider{}}
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
	action := &filegen.Move{Impl: &provider.Provider{}}
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

	action := &filegen.Backup{Impl: &provider.Provider{}}
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

	action := &filegen.Backup{Impl: &provider.Provider{}}
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
	action := &filegen.Backup{Impl: &provider.Provider{}}
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
	action := &filegen.Remove{Impl: &provider.Provider{}}
	ctx := dryRunCtx(t)
	slots := map[string]any{
		"path":            "/tmp/remove.txt",
		"prune":           false,
		"prune_boundary":  "",
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
	action := &filegen.RemoveAll{Impl: &provider.Provider{}}
	ctx := dryRunCtx(t)
	slots := map[string]any{
		"path":            "/tmp/remove-all",
		"prune":           false,
		"prune_boundary":  "",
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
	action := &filegen.Unlink{Impl: &provider.Provider{}}
	ctx := dryRunCtx(t)
	slots := map[string]any{
		"path":            "/tmp/unlink.txt",
		"prune":           false,
		"prune_boundary":  "",
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

	action := &filegen.Glob{Impl: &provider.Provider{Root: tmp}}
	ctx := newCtx(t)
	slots := map[string]any{
		"pattern":          filepath.Join(tmp, "*.txt"),
		"honor_gitignore":  false,
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
	action := &filegen.Glob{Impl: &provider.Provider{}}
	ctx := dryRunCtx(t)
	slots := map[string]any{
		"pattern":          "/tmp/*.txt",
		"honor_gitignore":  false,
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

	action := &filegen.Mkdir{Impl: &provider.Provider{}}
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
	action := &filegen.Mkdir{Impl: &provider.Provider{}}
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

	action := &filegen.Read{Impl: &provider.Provider{}}
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
		t.Error("result is nil, want Blob")
	}
}

func TestReadAction_DryRun(t *testing.T) {
	action := &filegen.Read{Impl: &provider.Provider{}}
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

// ── Exists ───────────────────────────────────────────────────────────────────────

func TestExistsAction_Do(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "action-exists.txt")
	if err := os.WriteFile(path, []byte("exists"), 0o644); err != nil {
		t.Fatal(err)
	}

	action := &filegen.Exists{Impl: &provider.Provider{}}
	ctx := newCtx(t)
	slots := map[string]any{
		"path": path,
	}

	result, undo, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if undo != nil {
		t.Errorf("undo = %v, want nil (Exists is not compensable)", undo)
	}
	if result != true {
		t.Errorf("result = %v, want true", result)
	}
}

func TestExistsAction_DoMissing(t *testing.T) {
	action := &filegen.Exists{Impl: &provider.Provider{}}
	ctx := newCtx(t)
	slots := map[string]any{
		"path": "/nonexistent/file.txt",
	}

	result, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != false {
		t.Errorf("result = %v, want false for non-existent file", result)
	}
}

func TestExistsAction_DryRun(t *testing.T) {
	action := &filegen.Exists{Impl: &provider.Provider{}}
	ctx := dryRunCtx(t)
	slots := map[string]any{
		"path": "/tmp/dryrun-exists.txt",
	}

	result, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != nil {
		t.Errorf("dry-run result = %v, want nil", result)
	}

	output := ctx.Writer.(*bytes.Buffer).String()
	if !strings.Contains(output, "[dry-run] file.exists") {
		t.Errorf("dry-run output = %q, want to contain [dry-run] file.exists", output)
	}
}

// ── IsDir ────────────────────────────────────────────────────────────────────────

func TestIsDirAction_Do(t *testing.T) {
	tmp := t.TempDir()

	action := &filegen.IsDir{Impl: &provider.Provider{}}
	ctx := newCtx(t)
	slots := map[string]any{
		"path": tmp,
	}

	result, undo, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if undo != nil {
		t.Errorf("undo = %v, want nil (IsDir is not compensable)", undo)
	}
	if result != true {
		t.Errorf("result = %v, want true for directory", result)
	}
}

func TestIsDirAction_DoFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "file.txt")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	action := &filegen.IsDir{Impl: &provider.Provider{}}
	ctx := newCtx(t)
	slots := map[string]any{
		"path": path,
	}

	result, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != false {
		t.Errorf("result = %v, want false for regular file", result)
	}
}

func TestIsDirAction_DryRun(t *testing.T) {
	action := &filegen.IsDir{Impl: &provider.Provider{}}
	ctx := dryRunCtx(t)
	slots := map[string]any{
		"path": "/tmp",
	}

	result, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != nil {
		t.Errorf("dry-run result = %v, want nil", result)
	}

	output := ctx.Writer.(*bytes.Buffer).String()
	if !strings.Contains(output, "[dry-run] file.is_dir") {
		t.Errorf("dry-run output = %q, want to contain [dry-run] file.is_dir", output)
	}
}

// ── IsFile ───────────────────────────────────────────────────────────────────────

func TestIsFileAction_Do(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "regular.txt")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	action := &filegen.IsFile{Impl: &provider.Provider{}}
	ctx := newCtx(t)
	slots := map[string]any{
		"path": path,
	}

	result, undo, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if undo != nil {
		t.Errorf("undo = %v, want nil (IsFile is not compensable)", undo)
	}
	if result != true {
		t.Errorf("result = %v, want true for regular file", result)
	}
}

func TestIsFileAction_DoDir(t *testing.T) {
	tmp := t.TempDir()

	action := &filegen.IsFile{Impl: &provider.Provider{}}
	ctx := newCtx(t)
	slots := map[string]any{
		"path": tmp,
	}

	result, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != false {
		t.Errorf("result = %v, want false for directory", result)
	}
}

func TestIsFileAction_DryRun(t *testing.T) {
	action := &filegen.IsFile{Impl: &provider.Provider{}}
	ctx := dryRunCtx(t)
	slots := map[string]any{
		"path": "/tmp/dryrun-isfile.txt",
	}

	result, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != nil {
		t.Errorf("dry-run result = %v, want nil", result)
	}

	output := ctx.Writer.(*bytes.Buffer).String()
	if !strings.Contains(output, "[dry-run] file.is_file") {
		t.Errorf("dry-run output = %q, want to contain [dry-run] file.is_file", output)
	}
}

// ── Join ─────────────────────────────────────────────────────────────────────────

func TestJoinAction_Do(t *testing.T) {
	action := &filegen.Join{Impl: &provider.Provider{}}
	ctx := newCtx(t)
	slots := map[string]any{
		"parts": []string{"a", "b", "c"},
	}

	result, undo, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if undo != nil {
		t.Errorf("undo = %v, want nil (Join is not compensable)", undo)
	}

	want := filepath.Join("a", "b", "c")
	if result != want {
		t.Errorf("result = %v, want %q", result, want)
	}
}

func TestJoinAction_DryRun(t *testing.T) {
	action := &filegen.Join{Impl: &provider.Provider{}}
	ctx := dryRunCtx(t)
	slots := map[string]any{
		"parts": []string{"a", "b"},
	}

	result, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != nil {
		t.Errorf("dry-run result = %v, want nil", result)
	}

	output := ctx.Writer.(*bytes.Buffer).String()
	if !strings.Contains(output, "[dry-run] file.join") {
		t.Errorf("dry-run output = %q, want to contain [dry-run] file.join", output)
	}
}

// ── Name ─────────────────────────────────────────────────────────────────────────

func TestNameAction_Do(t *testing.T) {
	action := &filegen.NameAction{Impl: &provider.Provider{}}
	ctx := newCtx(t)
	slots := map[string]any{
		"path": "/foo/bar/baz.txt",
	}

	result, undo, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if undo != nil {
		t.Errorf("undo = %v, want nil (Name is not compensable)", undo)
	}
	if result != "baz.txt" {
		t.Errorf("result = %v, want %q", result, "baz.txt")
	}
}

func TestNameAction_DryRun(t *testing.T) {
	action := &filegen.NameAction{Impl: &provider.Provider{}}
	ctx := dryRunCtx(t)
	slots := map[string]any{
		"path": "/foo/bar.txt",
	}

	result, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != nil {
		t.Errorf("dry-run result = %v, want nil", result)
	}

	output := ctx.Writer.(*bytes.Buffer).String()
	if !strings.Contains(output, "[dry-run] file.name") {
		t.Errorf("dry-run output = %q, want to contain [dry-run] file.name", output)
	}
}

// ── Parent ───────────────────────────────────────────────────────────────────────

func TestParentAction_Do(t *testing.T) {
	action := &filegen.Parent{Impl: &provider.Provider{}}
	ctx := newCtx(t)
	slots := map[string]any{
		"path": "/foo/bar/baz.txt",
	}

	result, undo, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if undo != nil {
		t.Errorf("undo = %v, want nil (Parent is not compensable)", undo)
	}
	if result != "/foo/bar" {
		t.Errorf("result = %v, want %q", result, "/foo/bar")
	}
}

func TestParentAction_DryRun(t *testing.T) {
	action := &filegen.Parent{Impl: &provider.Provider{}}
	ctx := dryRunCtx(t)
	slots := map[string]any{
		"path": "/foo/bar.txt",
	}

	result, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	if result != nil {
		t.Errorf("dry-run result = %v, want nil", result)
	}

	output := ctx.Writer.(*bytes.Buffer).String()
	if !strings.Contains(output, "[dry-run] file.parent") {
		t.Errorf("dry-run output = %q, want to contain [dry-run] file.parent", output)
	}
}

// ── Nil Undo for all compensable actions ────────────────────────────────────────

func TestCompensableActions_UndoNil(t *testing.T) {
	p := &provider.Provider{}
	ctx := newCtx(t)

	compensable := []struct {
		name   string
		action interface {
			Undo(*op.Context, op.UndoState) error
		}
	}{
		{"file.backup", &filegen.Backup{Impl: p}},
		{"file.copy", &filegen.Copy{Impl: p}},
		{"file.link", &filegen.Link{Impl: p}},
		{"file.move", &filegen.Move{Impl: p}},
		{"file.remove", &filegen.Remove{Impl: p}},
		{"file.remove_all", &filegen.RemoveAll{Impl: p}},
		{"file.unlink", &filegen.Unlink{Impl: p}},
		{"file.write_bytes", &filegen.WriteBytes{Impl: p}},
		{"file.write_text", &filegen.WriteText{Impl: p}},
	}

	for _, tc := range compensable {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.action.Undo(ctx, nil); err != nil {
				t.Errorf("Undo(nil) = %v, want nil", err)
			}
		})
	}
}
