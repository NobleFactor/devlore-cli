// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package sops

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
)

// TestEncrypter_RoundTrip encrypts every file in testdata/roundtrip under its own name, decrypts the result under that
// name plus .sops, as deploy decrypts it, and compares what comes back with the file, byte for byte (#977).
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

	entries, err := os.ReadDir(filepath.Join("testdata", "roundtrip"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("testdata/roundtrip holds no files")
	}

	for _, entry := range entries {
		t.Run(entry.Name(), func(t *testing.T) {

			cleartext, err := os.ReadFile(filepath.Join("testdata", "roundtrip", entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			sourcePath := filepath.Join(root, entry.Name())

			ciphertext, err := NewEncrypter().Encrypt(cleartext, sourcePath, root)
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}
			if !IsEncrypted(ciphertext) {
				t.Fatalf("output is not SOPS-encrypted:\n%s", ciphertext)
			}

			// The config-free Client decrypts with the ambient age key, under the name deploy reads.
			got, err := (&Client{}).Decrypt(ciphertext, sourcePath+".sops")
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}
			if !bytes.Equal(got, cleartext) {
				t.Errorf("decrypted = %q, want %q", got, cleartext)
			}
		})
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
