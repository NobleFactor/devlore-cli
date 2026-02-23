// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package secrets

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestNewManager(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	if m == nil {
		t.Fatal("NewManager() returned nil")
	}
}

func TestHasConfig(t *testing.T) {
	tests := []struct {
		name       string
		createSops bool
		want       bool
	}{
		{"with .sops.yaml", true, true},
		{"without .sops.yaml", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.createSops {
				if err := os.WriteFile(filepath.Join(dir, ".sops.yaml"), []byte("# sops config\n"), 0o644); err != nil {
					t.Fatalf("create .sops.yaml: %v", err)
				}
			}
			m, err := NewManager(dir)
			if err != nil {
				t.Fatalf("NewManager() error = %v", err)
			}
			if got := m.HasConfig(); got != tt.want {
				t.Errorf("HasConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigPath(t *testing.T) {
	tests := []struct {
		name       string
		createSops bool
		wantEmpty  bool
	}{
		{"config exists", true, false},
		{"config missing", false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.createSops {
				if err := os.WriteFile(filepath.Join(dir, ".sops.yaml"), []byte("# sops config\n"), 0o644); err != nil {
					t.Fatalf("create .sops.yaml: %v", err)
				}
			}
			m, err := NewManager(dir)
			if err != nil {
				t.Fatalf("NewManager() error = %v", err)
			}
			got := m.ConfigPath()
			if tt.wantEmpty && got != "" {
				t.Errorf("ConfigPath() = %q, want empty", got)
			}
			if !tt.wantEmpty {
				want := filepath.Join(dir, ".sops.yaml")
				if got != want {
					t.Errorf("ConfigPath() = %q, want %q", got, want)
				}
			}
		})
	}
}

func TestFindSopsConfig(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(root string) string // returns search start dir
		wantFound bool
		wantDir   string // relative to root; which dir holds .sops.yaml
	}{
		{
			name: "found at root",
			setup: func(root string) string {
				writeFixture(t, filepath.Join(root, ".sops.yaml"))
				return root
			},
			wantFound: true,
			wantDir:   "",
		},
		{
			name: "found at parent",
			setup: func(root string) string {
				writeFixture(t, filepath.Join(root, ".sops.yaml"))
				child := filepath.Join(root, "sub", "deep")
				if err := os.MkdirAll(child, 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				return child
			},
			wantFound: true,
			wantDir:   "",
		},
		{
			name: "not found",
			setup: func(root string) string {
				// no .sops.yaml anywhere under root
				child := filepath.Join(root, "sub")
				if err := os.MkdirAll(child, 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				return child
			},
			wantFound: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			startDir := tt.setup(root)
			got := findSopsConfig(startDir)
			if tt.wantFound {
				want := filepath.Join(root, tt.wantDir, ".sops.yaml")
				if got != want {
					t.Errorf("findSopsConfig() = %q, want %q", got, want)
				}
			} else if got != "" {
				t.Errorf("findSopsConfig() = %q, want empty", got)
			}
		})
	}
}

func TestDecryptor(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(dir)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	dec := m.Decryptor()
	if dec == nil {
		t.Fatal("Decryptor() returned nil")
	}

	// Plaintext data (not encrypted) should pass through unchanged.
	plaintext := []byte("hello world\n")
	got, err := dec("test.txt", plaintext)
	if err != nil {
		t.Fatalf("Decryptor() error on plaintext = %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("Decryptor() = %q, want %q", got, plaintext)
	}
}

// writeFixture creates a minimal .sops.yaml fixture file.
func writeFixture(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("# sops config\n"), 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", path, err)
	}
}
