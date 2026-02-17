// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestCloneAndCompensate(t *testing.T) {
	dir := t.TempDir()
	clonePath := filepath.Join(dir, "repo")

	p := &Provider{
		cloneFn: func(url, path string, output io.Writer) error {
			return os.MkdirAll(path, 0755)
		},
	}

	state, err := p.Clone("https://example.com/repo.git", clonePath, io.Discard)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}

	path, _ := state["path"].(string)
	if path != clonePath {
		t.Errorf("expected path=%q, got %q", clonePath, path)
	}

	// Verify directory exists
	if _, err := os.Stat(clonePath); err != nil {
		t.Fatalf("cloned directory should exist: %v", err)
	}

	// Compensate: removes directory
	if err := p.CompensateClone(state); err != nil {
		t.Fatalf("CompensateClone: %v", err)
	}

	if _, err := os.Stat(clonePath); !os.IsNotExist(err) {
		t.Error("cloned directory should have been removed")
	}
}

func TestCompensateCloneEmptyPath(t *testing.T) {
	p := &Provider{}
	if err := p.CompensateClone(map[string]any{"path": ""}); err != nil {
		t.Errorf("CompensateClone(empty path): %v", err)
	}
}

func TestCompensateCloneNilState(t *testing.T) {
	p := &Provider{}
	if err := p.CompensateClone(nil); err != nil {
		t.Errorf("CompensateClone(nil): %v", err)
	}
}
