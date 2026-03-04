// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"
)

func TestCloneViaHook(t *testing.T) {
	var gotURL, gotPath string
	var gotOutput io.Writer

	p := &Provider{
		cloneFn: func(url, path string, output io.Writer) error {
			gotURL = url
			gotPath = path
			gotOutput = output
			return nil
		},
	}

	buf := &bytes.Buffer{}
	result, state, err := p.Clone("https://example.com/repo.git", "/tmp/clone-dest", buf)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}

	if gotURL != "https://example.com/repo.git" {
		t.Errorf("cloneFn url = %q, want %q", gotURL, "https://example.com/repo.git")
	}
	if gotPath != "/tmp/clone-dest" {
		t.Errorf("cloneFn path = %q, want %q", gotPath, "/tmp/clone-dest")
	}
	if gotOutput != buf {
		t.Error("cloneFn output writer does not match")
	}
	if result != "/tmp/clone-dest" {
		t.Errorf("result = %q, want %q", result, "/tmp/clone-dest")
	}

	if state == nil {
		t.Fatal("state is nil")
	}
	if path, _ := state["path"].(string); path != "/tmp/clone-dest" {
		t.Errorf("state path = %q, want %q", path, "/tmp/clone-dest")
	}
}

func TestCloneHookError(t *testing.T) {
	hookErr := errors.New("clone failed")
	p := &Provider{
		cloneFn: func(_, _ string, _ io.Writer) error {
			return hookErr
		},
	}

	result, state, err := p.Clone("https://example.com/repo.git", "/tmp/dest", &bytes.Buffer{})
	if !errors.Is(err, hookErr) {
		t.Fatalf("Clone error = %v, want %v", err, hookErr)
	}
	if result != "" {
		t.Errorf("result = %q, want empty", result)
	}
	if state != nil {
		t.Errorf("state = %v, want nil", state)
	}
}

func TestCompensateClone(t *testing.T) {
	tmp := t.TempDir()
	dir := tmp + "/to-remove"
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	state := map[string]any{"path": dir}
	p := &Provider{}
	if err := p.CompensateClone(state); err != nil {
		t.Fatalf("CompensateClone: %v", err)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("directory %q still exists after compensation", dir)
	}
}

func TestCompensateCloneNilState(t *testing.T) {
	p := &Provider{}
	if err := p.CompensateClone(nil); err != nil {
		t.Fatalf("CompensateClone(nil) = %v, want nil", err)
	}
}

func TestCompensateCloneEmptyPath(t *testing.T) {
	state := map[string]any{"path": ""}
	p := &Provider{}
	if err := p.CompensateClone(state); err != nil {
		t.Fatalf("CompensateClone(empty path) = %v, want nil", err)
	}
}
