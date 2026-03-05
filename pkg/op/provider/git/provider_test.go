// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"errors"
	"os"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
)

func TestCloneViaHook(t *testing.T) {
	var gotURL, gotPath string

	p := &Provider{
		ProviderBase: op.NewProviderBase(op.Context{}),
		cloneFn: func(url, path string) error {
			gotURL = url
			gotPath = path
			return nil
		},
	}

	result, state, err := p.Clone("https://example.com/repo.git", "/tmp/clone-dest")
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}

	if gotURL != "https://example.com/repo.git" {
		t.Errorf("cloneFn url = %q, want %q", gotURL, "https://example.com/repo.git")
	}
	if gotPath != "/tmp/clone-dest" {
		t.Errorf("cloneFn path = %q, want %q", gotPath, "/tmp/clone-dest")
	}
	if result != "/tmp/clone-dest" {
		t.Errorf("result = %q, want %q", result, "/tmp/clone-dest")
	}

	if state.ClonedPath != "/tmp/clone-dest" {
		t.Errorf("state.ClonedPath = %q, want %q", state.ClonedPath, "/tmp/clone-dest")
	}
}

func TestCloneHookError(t *testing.T) {
	hookErr := errors.New("clone failed")
	p := &Provider{
		ProviderBase: op.NewProviderBase(op.Context{}),
		cloneFn: func(_, _ string) error {
			return hookErr
		},
	}

	result, state, err := p.Clone("https://example.com/repo.git", "/tmp/dest")
	if !errors.Is(err, hookErr) {
		t.Fatalf("Clone error = %v, want %v", err, hookErr)
	}
	if result != "" {
		t.Errorf("result = %q, want empty", result)
	}
	if state.ClonedPath != "" {
		t.Errorf("state.ClonedPath = %q, want empty", state.ClonedPath)
	}
}

func TestCompensateClone(t *testing.T) {
	tmp := t.TempDir()
	dir := tmp + "/to-remove"
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	p := &Provider{ProviderBase: op.NewProviderBase(op.Context{})}
	if err := p.CompensateClone(Tombstone{ClonedPath: dir}); err != nil {
		t.Fatalf("CompensateClone: %v", err)
	}

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("directory %q still exists after compensation", dir)
	}
}

func TestCompensateCloneEmptyPath(t *testing.T) {
	p := &Provider{ProviderBase: op.NewProviderBase(op.Context{})}
	if err := p.CompensateClone(Tombstone{}); err != nil {
		t.Fatalf("CompensateClone(empty) = %v, want nil", err)
	}
}
