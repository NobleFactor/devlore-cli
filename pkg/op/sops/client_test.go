// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package sops

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// writeFixture creates a minimal .sops.yaml fixture file.
func writeFixture(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("creation_rules:\n  - path_regex: .*\n    age: age1abc\n"), 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", path, err)
	}
}

// --- NewClient ---

func TestNewClient_ConfigFound(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, filepath.Join(dir, ".sops.yaml"))

	client, err := NewClient(dir)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("NewClient() returned nil client")
	}
}

func TestNewClient_ConfigNotFound(t *testing.T) {
	dir := t.TempDir()

	_, err := NewClient(dir)
	if err == nil {
		t.Fatal("NewClient() should return error when no .sops.yaml exists")
	}
}

func TestNewClient_ConfigInParent(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, filepath.Join(root, ".sops.yaml"))
	child := filepath.Join(root, "sub", "deep")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	client, err := NewClient(child)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if client == nil {
		t.Fatal("NewClient() returned nil client")
	}
}

// --- findConfig ---

func TestFindConfig(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(root string) string // returns search start dir
		wantFound bool
	}{
		{
			name: "found at root",
			setup: func(root string) string {
				writeFixture(t, filepath.Join(root, ".sops.yaml"))
				return root
			},
			wantFound: true,
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
		},
		{
			name: "not found",
			setup: func(root string) string {
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
			got := findConfig(startDir)
			if tt.wantFound {
				want := filepath.Join(root, ".sops.yaml")
				if got != want {
					t.Errorf("findConfig() = %q, want %q", got, want)
				}
			} else if got != "" {
				t.Errorf("findConfig() = %q, want empty", got)
			}
		})
	}
}

// --- Decryptor ---

func TestDecryptor_PlaintextPassthrough(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, filepath.Join(dir, ".sops.yaml"))

	client, err := NewClient(dir)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	dec := client.Decryptor()
	if dec == nil {
		t.Fatal("Decryptor() returned nil")
	}

	plaintext := []byte("hello world\n")
	got, err := dec("test.txt", plaintext)
	if err != nil {
		t.Fatalf("Decryptor() error on plaintext = %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("Decryptor() = %q, want %q", got, plaintext)
	}
}

// --- Decrypt ---

func TestDecrypt_PlaintextPassthrough(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, filepath.Join(dir, ".sops.yaml"))

	client, err := NewClient(dir)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	plaintext := []byte("name: sshd\nport: 22\n")
	got, err := client.Decrypt(plaintext, "config.yaml")
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("Decrypt() = %q, want %q", got, plaintext)
	}
}

// --- Sign ---

func TestSign_NoBackends(t *testing.T) {
	dir := t.TempDir()
	// Create config with only age (no signing backends)
	if err := os.WriteFile(filepath.Join(dir, ".sops.yaml"), []byte("creation_rules:\n  - path_regex: .*\n    age: age1abc\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	client, err := NewClient(dir)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	sig, err := client.Sign([]byte("test data"))
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	if sig != nil {
		t.Error("Sign() should return nil when no signing backends are configured")
	}
}

// --- Verify ---

func TestVerify_UnknownMethod(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, filepath.Join(dir, ".sops.yaml"))

	client, err := NewClient(dir)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	err = client.Verify([]byte("data"), &Signature{Method: "unknown"})
	if err == nil {
		t.Error("Verify() should return error for unknown method")
	}
}
