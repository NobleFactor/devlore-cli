// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package sops

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
)

func TestEncrypter_RoundTrip(t *testing.T) {

	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("SOPS_AGE_KEY", identity.String())

	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // isolate: no XDG fallback

	sopsYAML := "creation_rules:\n  - path_regex: .*\n    age: " + identity.Recipient().String() + "\n"
	if err := os.WriteFile(filepath.Join(root, ".sops.yaml"), []byte(sopsYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	plaintext := []byte("greeting: hello\nname: world\n")
	sourcePath := filepath.Join(root, "secret.yaml")

	ciphertext, err := NewEncrypter().Encrypt(plaintext, sourcePath, root)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if !IsEncrypted(ciphertext) {
		t.Fatalf("output is not SOPS-encrypted:\n%s", ciphertext)
	}

	// Round-trip: the config-free Client decrypts it back to plaintext using the ambient age key.
	got, err := (&Client{}).Decrypt(ciphertext, sourcePath)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Contains(got, []byte("hello")) || !bytes.Contains(got, []byte("world")) {
		t.Errorf("decrypted = %q, want to contain hello + world", got)
	}
}

func TestEncrypter_NoConfig(t *testing.T) {

	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // no in-tree or XDG config

	_, err := NewEncrypter().Encrypt([]byte("x: y\n"), filepath.Join(root, "f.yaml"), root)
	if err == nil {
		t.Fatal("expected error when no .sops.yaml governs the file")
	}
}
