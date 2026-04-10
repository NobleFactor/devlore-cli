// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package archive_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"testing"

	"reflect"

	"github.com/NobleFactor/devlore-cli/pkg/op/bind"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/archive"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/archive/gen"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	_ "github.com/NobleFactor/devlore-cli/pkg/op/provider/file/gen" // register file.Resource constructor
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func testCtx(t *testing.T) (*op.ExecutionContext, string) {
	t.Helper()
	dir := t.TempDir()
	root := op.NewRootReaderWriter(dir)
	ctx := &op.ExecutionContext{
		Context:  context.Background(),
		Writer:   &bytes.Buffer{},
		Root:     root,
		Registry: op.NewReceiverRegistry(),
	}
	ctx.RecoverySite = op.NewRecoverySite(ctx)
	return ctx, dir
}

func receiverType(t *testing.T) op.ProviderReceiverType {
	t.Helper()
	reg := op.NewReceiverRegistry()
	rt, ok := reg.TypeByReflection(reflect.TypeFor[archive.Provider]())
	if !ok {
		t.Fatal("archive provider type not registered")
	}
	return rt.(op.ProviderReceiverType)
}

// createTestTarGz creates a tar.gz with a single file inside.
func createTestTarGz(t *testing.T, dir string) string {
	t.Helper()
	archivePath := filepath.Join(dir, "test.tar.gz")

	f, err := os.Create(archivePath)
	if err != nil {
		t.Fatalf("create archive: %v", err)
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	content := []byte("extracted content")
	hdr := &tar.Header{
		Name: "sample.txt",
		Mode: 0o644,
		Size: int64(len(content)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("write content: %v", err)
	}

	return archivePath
}

// region Starlark integration

func TestStarlark(t *testing.T) {
	ctx, dir := testCtx(t)
	archivePath := createTestTarGz(t, dir)
	destDir := filepath.Join(dir, "extracted")

	p := archive.NewProvider(ctx)
	receiver := bind.NewProvider(receiverType(t), p)

	globals := starlark.StringDict{
		"archive":      receiver,
		"test_archive": starlark.String(archivePath),
		"test_dest":    starlark.String(destDir),
	}

	thread := &starlark.Thread{
		Name:  "archive-integration",
		Print: func(_ *starlark.Thread, msg string) { t.Logf("[star] %s", msg) },
	}

	data, err := os.ReadFile("testdata/integration.star")
	if err != nil {
		t.Fatalf("reading script: %v", err)
	}

	opts := &syntax.FileOptions{Set: true, GlobalReassign: true, TopLevelControl: true}
	result, err := starlark.ExecFileOptions(opts, thread, "testdata/integration.star", data, globals)
	if err != nil {
		t.Fatalf("exec script: %v", err)
	}

	assertBool(t, result, "result_done")

	// Verify the file was actually extracted.
	extracted := filepath.Join(destDir, "sample.txt")
	content, err := os.ReadFile(extracted)
	if err != nil {
		t.Fatalf("reading extracted file: %v", err)
	}
	if string(content) != "extracted content" {
		t.Errorf("extracted content = %q, want 'extracted content'", string(content))
	}
}

// endregion

// region Action dispatch

func TestActions_Extract(t *testing.T) {
	ctx, dir := testCtx(t)
	archivePath := createTestTarGz(t, dir)
	destDir := filepath.Join(dir, "action_extracted")

	a, err := ctx.ActionByName("archive.extract")
	if err != nil {
		t.Fatalf("action archive.extract not registered: %v", err)
	}

	sourceRes, err := file.NewResource(ctx, archivePath)
	if err != nil {
		t.Fatalf("NewResource(source): %v", err)
	}
	prefixRes, err := file.NewResource(ctx, destDir)
	if err != nil {
		t.Fatalf("NewResource(prefix): %v", err)
	}
	result, complement, err := a.Do(ctx, map[string]any{
		"source": sourceRes,
		"prefix": prefixRes,
	})
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}

	// Result should be a *file.Resource for the dest dir.
	res, ok := result.(*file.Resource)
	if !ok {
		t.Fatalf("result type = %T, want *file.Resource", result)
	}
	if res.SourcePath.Abs() != destDir {
		t.Errorf("result path = %q, want %q", res.SourcePath.Abs(), destDir)
	}

	// Complement should be non-nil (compensable action).
	if complement == nil {
		t.Error("complement = nil, want non-nil tombstone")
	}

	// Verify extraction.
	extracted := filepath.Join(destDir, "sample.txt")
	content, err := os.ReadFile(extracted)
	if err != nil {
		t.Fatalf("reading extracted file: %v", err)
	}
	if string(content) != "extracted content" {
		t.Errorf("extracted content = %q, want 'extracted content'", string(content))
	}
}

// endregion

// region Assertions

func assertBool(t *testing.T, globals starlark.StringDict, key string) {
	t.Helper()
	v, ok := globals[key]
	if !ok {
		t.Errorf("missing global %q", key)
		return
	}
	if v != starlark.True {
		t.Errorf("%s = %v, want true", key, v)
	}
}

// endregion
