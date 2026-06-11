// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package sops

import (
	"bytes"
	"testing"
)

// --- Decrypt ---

func TestDecrypt_PlaintextPassthrough(t *testing.T) {

	plaintext := []byte("name: sshd\nport: 22\n")
	got, err := (&Client{}).Decrypt(plaintext, "config.yaml")
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Errorf("Decrypt() = %q, want %q", got, plaintext)
	}
}

// --- Decryptor ---

func TestDecryptor_PlaintextPassthrough(t *testing.T) {

	dec := (&Client{}).Decryptor()
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
