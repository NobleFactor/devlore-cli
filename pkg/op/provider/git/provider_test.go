// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package git

import (
	"errors"
	"os"
	"testing"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	netprov "github.com/NobleFactor/devlore-cli/pkg/op/provider/net"
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

	url := mustNetResource(t, "https://example.com/repo.git")
	dest := mustFileResource(t, "/tmp/clone-dest")

	result, state, err := p.Clone(url, dest)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}

	if gotURL != "https://example.com/repo.git" {
		t.Errorf("cloneFn url = %q, want %q", gotURL, "https://example.com/repo.git")
	}
	if gotPath != "/tmp/clone-dest" {
		t.Errorf("cloneFn path = %q, want %q", gotPath, "/tmp/clone-dest")
	}
	if result.ClonePath != "/tmp/clone-dest" {
		t.Errorf("result.ClonePath = %q, want %q", result.ClonePath, "/tmp/clone-dest")
	}
	if result.URL != "https://example.com/repo.git" {
		t.Errorf("result.URL = %q, want %q", result.URL, "https://example.com/repo.git")
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

	url := mustNetResource(t, "https://example.com/repo.git")
	dest := mustFileResource(t, "/tmp/dest")

	result, state, err := p.Clone(url, dest)
	if !errors.Is(err, hookErr) {
		t.Fatalf("Clone error = %v, want %v", err, hookErr)
	}
	if result.ClonePath != "" {
		t.Errorf("result.ClonePath = %q, want empty", result.ClonePath)
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

func mustNetResource(t *testing.T, raw string) netprov.Resource {
	t.Helper()
	r, err := op.Construct[netprov.Resource](raw)
	if err != nil {
		t.Fatalf("Construct net.Resource(%q): %v", raw, err)
	}
	return r
}

func mustFileResource(t *testing.T, path string) file.Resource {
	t.Helper()
	r, err := op.Construct[file.Resource](path)
	if err != nil {
		t.Fatalf("Construct file.Resource(%q): %v", path, err)
	}
	return r
}
