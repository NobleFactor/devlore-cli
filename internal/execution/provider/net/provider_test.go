// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package net

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompensateDownloadRemovesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	p := &Provider{}
	state := map[string]any{"path": path}
	if err := p.CompensateDownload(state); err != nil {
		t.Fatalf("CompensateDownload: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should have been removed")
	}
}

func TestCompensateDownloadEmptyPath(t *testing.T) {
	p := &Provider{}
	if err := p.CompensateDownload(map[string]any{"path": ""}); err != nil {
		t.Errorf("CompensateDownload(empty path): %v", err)
	}
}

func TestCompensateDownloadNilState(t *testing.T) {
	p := &Provider{}
	if err := p.CompensateDownload(nil); err != nil {
		t.Errorf("CompensateDownload(nil): %v", err)
	}
}
