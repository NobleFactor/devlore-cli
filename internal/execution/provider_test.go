// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build ignore
// +build ignore

package execution_test

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/appnet"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/archive"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/git"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/pkg"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/service"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/shell"
)

// mockServiceManager implements op.ServiceManager for tests.
type mockServiceManager struct {
	startFail bool
}

func (m *mockServiceManager) Exists(_ string) bool    { return false }
func (m *mockServiceManager) IsRunning(_ string) bool { return false }
func (m *mockServiceManager) IsEnabled(_ string) bool { return false }
func (m *mockServiceManager) Status(_ string) string  { return "stopped" }
func (m *mockServiceManager) NeedsSudo() bool         { return false }

func (m *mockServiceManager) Start(_ string) op.PlatformResult {
	if m.startFail {
		return op.PlatformResult{OK: false, Stderr: "mock start failure"}
	}
	return op.PlatformResult{OK: true}
}

func (m *mockServiceManager) Stop(_ string) op.PlatformResult {
	return op.PlatformResult{OK: true}
}

func (m *mockServiceManager) Enable(_ string) op.PlatformResult {
	return op.PlatformResult{OK: true}
}

func (m *mockServiceManager) Disable(_ string) op.PlatformResult {
	return op.PlatformResult{OK: true}
}

// --- pkg action dry-run tests ---

func TestPkgInstallDryRun(t *testing.T) {
	var buf bytes.Buffer
	ctx := &op.Context{ContextBase: op.ContextBase{Context: context.Background(), DryRun: true, Writer: &buf}}
	action := &pkg.Install{}
	slots := map[string]any{
		"packages": []string{"vim", "tmux"},
		"manager":  "brew",
		"cask":     false,
	}

	_, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("pkg.install dry-run: %v", err)
	}
	if !strings.Contains(buf.String(), "[dry-run] pkg.install") {
		t.Errorf("expected dry-run pkg.install log, got %q", buf.String())
	}
}

func TestPkgUpgradeDryRun(t *testing.T) {
	var buf bytes.Buffer
	ctx := &op.Context{ContextBase: op.ContextBase{Context: context.Background(), DryRun: true, Writer: &buf}}
	action := &pkg.Upgrade{}
	slots := map[string]any{
		"packages": []string{"vim"},
		"manager":  "brew",
		"cask":     false,
	}

	_, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("pkg.upgrade dry-run: %v", err)
	}
	if !strings.Contains(buf.String(), "pkg.upgrade") {
		t.Errorf("expected 'pkg.upgrade' in log, got %q", buf.String())
	}
}

func TestPkgRemoveDryRun(t *testing.T) {
	var buf bytes.Buffer
	ctx := &op.Context{ContextBase: op.ContextBase{Context: context.Background(), DryRun: true, Writer: &buf}}
	action := &pkg.Remove{}
	slots := map[string]any{
		"packages": []string{"unused-pkg"},
		"manager":  "brew",
		"cask":     false,
	}

	_, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("pkg.remove dry-run: %v", err)
	}
	if !strings.Contains(buf.String(), "pkg.remove") {
		t.Errorf("expected 'pkg.remove' in log, got %q", buf.String())
	}
}

func TestPkgUpdateDryRun(t *testing.T) {
	var buf bytes.Buffer
	ctx := &op.Context{ContextBase: op.ContextBase{Context: context.Background(), DryRun: true, Writer: &buf}}
	action := &pkg.Update{}
	slots := map[string]any{
		"manager": "brew",
	}

	_, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("pkg.update dry-run: %v", err)
	}
	if !strings.Contains(buf.String(), "pkg.update") {
		t.Errorf("expected 'pkg.update' in log, got %q", buf.String())
	}
}

// --- shell action dry-run tests ---

func TestShellDryRun(t *testing.T) {
	var buf bytes.Buffer
	ctx := &op.Context{ContextBase: op.ContextBase{Context: context.Background(), DryRun: true, Writer: &buf}}
	action := &shell.Exec{Impl: &shell.Provider{}}
	slots := map[string]any{
		"command": "echo hello",
	}

	_, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("shell.shell dry-run: %v", err)
	}
	if !strings.Contains(buf.String(), "[dry-run] shell") {
		t.Errorf("expected dry-run shell log, got %q", buf.String())
	}
	if !strings.Contains(buf.String(), "echo hello") {
		t.Errorf("expected command in log, got %q", buf.String())
	}
}

func TestShellEmptyCommand(t *testing.T) {
	p := &shell.Provider{}
	_, err := p.Exec("", os.Stdout)
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestPowerShellDryRun(t *testing.T) {
	var buf bytes.Buffer
	ctx := &op.Context{ContextBase: op.ContextBase{Context: context.Background(), DryRun: true, Writer: &buf}}
	action := &shell.PowerShell{Impl: &shell.Provider{}}
	slots := map[string]any{
		"command": "Get-Process",
	}

	_, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("shell.powershell dry-run: %v", err)
	}
	if !strings.Contains(buf.String(), "[dry-run] shell.power_shell") {
		t.Errorf("expected dry-run power_shell log, got %q", buf.String())
	}
}

// --- service action dry-run tests ---

func TestServiceStartDryRun(t *testing.T) {
	var buf bytes.Buffer
	ctx := &op.Context{ContextBase: op.ContextBase{Context: context.Background(), DryRun: true, Writer: &buf}}
	action := &service.Start{}
	slots := map[string]any{"name": "nginx"}

	_, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("service.start dry-run: %v", err)
	}
	if !strings.Contains(buf.String(), "service.start") {
		t.Errorf("expected 'service.start' in log, got %q", buf.String())
	}
	if !strings.Contains(buf.String(), "nginx") {
		t.Errorf("expected service name in log, got %q", buf.String())
	}
}

func TestServiceStopDryRun(t *testing.T) {
	var buf bytes.Buffer
	ctx := &op.Context{ContextBase: op.ContextBase{Context: context.Background(), DryRun: true, Writer: &buf}}
	action := &service.Stop{}
	slots := map[string]any{"name": "nginx"}

	_, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("service.stop dry-run: %v", err)
	}
	if !strings.Contains(buf.String(), "service.stop") {
		t.Errorf("expected 'service.stop' in log, got %q", buf.String())
	}
}

func TestServiceRestartDryRun(t *testing.T) {
	var buf bytes.Buffer
	ctx := &op.Context{ContextBase: op.ContextBase{Context: context.Background(), DryRun: true, Writer: &buf}}
	action := &service.Restart{}
	slots := map[string]any{"name": "nginx"}

	_, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("service.restart dry-run: %v", err)
	}
	if !strings.Contains(buf.String(), "service.restart") {
		t.Errorf("expected 'service.restart' in log, got %q", buf.String())
	}
}

func TestServiceEnableDryRun(t *testing.T) {
	var buf bytes.Buffer
	ctx := &op.Context{ContextBase: op.ContextBase{Context: context.Background(), DryRun: true, Writer: &buf}}
	action := &service.Enable{}
	slots := map[string]any{"name": "nginx"}

	_, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("service.enable dry-run: %v", err)
	}
	if !strings.Contains(buf.String(), "service.enable") {
		t.Errorf("expected 'service.enable' in log, got %q", buf.String())
	}
}

func TestServiceDisableDryRun(t *testing.T) {
	var buf bytes.Buffer
	ctx := &op.Context{ContextBase: op.ContextBase{Context: context.Background(), DryRun: true, Writer: &buf}}
	action := &service.Disable{}
	slots := map[string]any{"name": "nginx"}

	_, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("service.disable dry-run: %v", err)
	}
	if !strings.Contains(buf.String(), "service.disable") {
		t.Errorf("expected 'service.disable' in log, got %q", buf.String())
	}
}

func TestServiceEmptyName(t *testing.T) {
	platform := &op.Platform{
		ServiceManager: &mockServiceManager{startFail: true},
	}
	p := &service.Provider{Platform: platform}
	_, _, err := p.Start("", io.Discard)
	if err == nil {
		t.Fatal("expected error for empty service name")
	}
}

// --- git action dry-run tests ---

func TestGitCloneDryRun(t *testing.T) {
	var buf bytes.Buffer
	ctx := &op.Context{ContextBase: op.ContextBase{Context: context.Background(), DryRun: true, Writer: &buf}}
	action := &git.Clone{Impl: &git.Provider{}}
	slots := map[string]any{
		"url":  "https://github.com/example/repo.git",
		"path": "/tmp/repo",
	}

	_, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("git.clone dry-run: %v", err)
	}
	if !strings.Contains(buf.String(), "git.clone") {
		t.Errorf("expected 'git.clone' in log, got %q", buf.String())
	}
	if !strings.Contains(buf.String(), "https://github.com/example/repo.git") {
		t.Errorf("expected URL in log, got %q", buf.String())
	}
}

func TestGitCloneEmptyURL(t *testing.T) {
	p := &git.Provider{}
	_, _, err := p.Clone("", "/tmp/repo", os.Stdout)
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestGitCheckoutDryRun(t *testing.T) {
	var buf bytes.Buffer
	ctx := &op.Context{ContextBase: op.ContextBase{Context: context.Background(), DryRun: true, Writer: &buf}}
	action := &git.Checkout{Impl: &git.Provider{}}
	slots := map[string]any{
		"repo": "/tmp/repo",
		"ref":  "main",
	}

	_, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("git.checkout dry-run: %v", err)
	}
	if !strings.Contains(buf.String(), "git.checkout") {
		t.Errorf("expected 'git.checkout' in log, got %q", buf.String())
	}
}

func TestGitPullDryRun(t *testing.T) {
	var buf bytes.Buffer
	ctx := &op.Context{ContextBase: op.ContextBase{Context: context.Background(), DryRun: true, Writer: &buf}}
	action := &git.Pull{Impl: &git.Provider{}}
	slots := map[string]any{
		"repo": "/tmp/repo",
	}

	_, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("git.pull dry-run: %v", err)
	}
	if !strings.Contains(buf.String(), "git.pull") {
		t.Errorf("expected 'git.pull' in log, got %q", buf.String())
	}
}

// --- appnet action tests ---

func TestAppnetDownload(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "downloaded content")
	}))
	defer ts.Close()

	ctx := &op.Context{ContextBase: op.ContextBase{Context: context.Background()}}
	action := &appnet.Download{Impl: &appnet.Provider{}}
	slots := map[string]any{
		"url": ts.URL,
	}

	result, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("appnet.download: %v", err)
	}

	data, ok := result.([]byte)
	if !ok {
		t.Fatalf("expected []byte result, got %T", result)
	}
	if string(data) != "downloaded content" {
		t.Errorf("expected 'downloaded content', got %q", string(data))
	}
}

func TestAppnetDownloadReturnsContent(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "file content")
	}))
	defer ts.Close()

	ctx := &op.Context{ContextBase: op.ContextBase{Context: context.Background()}}
	action := &appnet.Download{Impl: &appnet.Provider{}}
	slots := map[string]any{
		"url": ts.URL,
	}

	result, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("appnet.download: %v", err)
	}

	data, ok := result.([]byte)
	if !ok {
		t.Fatalf("expected []byte result, got %T", result)
	}
	if string(data) != "file content" {
		t.Errorf("expected 'file content', got %q", string(data))
	}
}

func TestAppnetDownloadDryRun(t *testing.T) {
	var buf bytes.Buffer
	ctx := &op.Context{ContextBase: op.ContextBase{Context: context.Background(), DryRun: true, Writer: &buf}}
	action := &appnet.Download{Impl: &appnet.Provider{}}
	slots := map[string]any{
		"url": "https://example.com/test.tar.gz",
	}

	_, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("appnet.download dry-run: %v", err)
	}
	if !strings.Contains(buf.String(), "[dry-run] appnet.download") {
		t.Errorf("expected dry-run download log, got %q", buf.String())
	}
}

// --- archive action tests ---

// createTarGz creates a tar.gz archive in dir with the given files.
func createTarGz(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	archivePath := filepath.Join(dir, "test.tar.gz")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	gw := gzip.NewWriter(f)
	defer func() { _ = gw.Close() }()
	tw := tar.NewWriter(gw)
	defer func() { _ = tw.Close() }()

	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}

	return archivePath
}

// createZip creates a zip archive in dir with the given files.
func createZip(t *testing.T, dir string, files map[string]string) string {
	t.Helper()
	archivePath := filepath.Join(dir, "test.zip")
	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	zw := zip.NewWriter(f)
	defer func() { _ = zw.Close() }()

	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}

	return archivePath
}

func TestArchiveExtractTarGz(t *testing.T) {
	tmpDir := t.TempDir()
	archiveDir := filepath.Join(tmpDir, "archives")
	extractDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		t.Fatal(err)
	}

	archivePath := createTarGz(t, archiveDir, map[string]string{
		"readme.txt":     "hello from tar.gz",
		"sub/nested.txt": "nested content",
	})

	ctx := &op.Context{ContextBase: op.ContextBase{Context: context.Background()}}
	action := &archive.Extract{Impl: &archive.Provider{}}
	slots := map[string]any{
		"source": archivePath,
		"prefix": extractDir,
	}

	_, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("archive.extract tar.gz: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(extractDir, "readme.txt"))
	if err != nil {
		t.Fatalf("read extracted: %v", err)
	}
	if string(content) != "hello from tar.gz" {
		t.Errorf("expected 'hello from tar.gz', got %q", string(content))
	}

	nested, err := os.ReadFile(filepath.Join(extractDir, "sub", "nested.txt"))
	if err != nil {
		t.Fatalf("read nested: %v", err)
	}
	if string(nested) != "nested content" {
		t.Errorf("expected 'nested content', got %q", string(nested))
	}
}

func TestArchiveExtractZip(t *testing.T) {
	tmpDir := t.TempDir()
	archiveDir := filepath.Join(tmpDir, "archives")
	extractDir := filepath.Join(tmpDir, "extracted")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(extractDir, 0o755); err != nil {
		t.Fatal(err)
	}

	archivePath := createZip(t, archiveDir, map[string]string{
		"readme.txt":     "hello from zip",
		"sub/nested.txt": "zip nested",
	})

	ctx := &op.Context{ContextBase: op.ContextBase{Context: context.Background()}}
	action := &archive.Extract{Impl: &archive.Provider{}}
	slots := map[string]any{
		"source": archivePath,
		"prefix": extractDir,
	}

	_, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("archive.extract zip: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(extractDir, "readme.txt"))
	if err != nil {
		t.Fatalf("read extracted: %v", err)
	}
	if string(content) != "hello from zip" {
		t.Errorf("expected 'hello from zip', got %q", string(content))
	}

	nested, err := os.ReadFile(filepath.Join(extractDir, "sub", "nested.txt"))
	if err != nil {
		t.Fatalf("read nested: %v", err)
	}
	if string(nested) != "zip nested" {
		t.Errorf("expected 'zip nested', got %q", string(nested))
	}
}

func TestArchiveExtractDryRun(t *testing.T) {
	var buf bytes.Buffer
	ctx := &op.Context{ContextBase: op.ContextBase{Context: context.Background(), DryRun: true, Writer: &buf}}
	action := &archive.Extract{Impl: &archive.Provider{}}
	slots := map[string]any{
		"source": "/tmp/test.tar.gz",
		"prefix": "/tmp/extracted",
	}

	_, _, err := action.Do(ctx, slots)
	if err != nil {
		t.Fatalf("archive.extract dry-run: %v", err)
	}
	if !strings.Contains(buf.String(), "[dry-run] archive.extract") {
		t.Errorf("expected dry-run archive.extract log, got %q", buf.String())
	}
}
