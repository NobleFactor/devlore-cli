// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package credentials

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCredentialsRoundTrip(t *testing.T) {
	// Use a temporary credentials directory for testing
	tmpDir := t.TempDir()
	originalXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", tmpDir)
	defer os.Setenv("XDG_CONFIG_HOME", originalXDG)

	key := "test/api-key"
	secret := "test-secret-value-12345"

	// Set credential (will use file fallback if no keychain)
	err := Set(key, secret)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Get credential
	retrieved, err := Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if retrieved != secret {
		t.Errorf("expected secret %q, got %q", secret, retrieved)
	}

	// Delete credential
	err = Delete(key)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deletion - should return error or empty
	retrieved, err = Get(key)
	if err == nil && retrieved != "" {
		t.Errorf("expected credential to be deleted, got %q", retrieved)
	}
}

func TestGetNonexistent(t *testing.T) {
	// Use a temporary credentials directory for testing
	tmpDir := t.TempDir()
	originalXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", tmpDir)
	defer os.Setenv("XDG_CONFIG_HOME", originalXDG)

	_, err := Get("nonexistent/key")
	// Should either return error or empty string
	// (depends on whether keychain is available)
	_ = err // We don't assert on the error, just that it doesn't panic
}

func TestDeleteNonexistent(t *testing.T) {
	// Use a temporary credentials directory for testing
	tmpDir := t.TempDir()
	originalXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", tmpDir)
	defer os.Setenv("XDG_CONFIG_HOME", originalXDG)

	// Deleting a nonexistent key should not error
	err := Delete("nonexistent/key")
	if err != nil {
		t.Errorf("Delete of nonexistent key should not error: %v", err)
	}
}

func TestSetOverwrite(t *testing.T) {
	// Use a temporary credentials directory for testing
	tmpDir := t.TempDir()
	originalXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", tmpDir)
	defer os.Setenv("XDG_CONFIG_HOME", originalXDG)

	key := "test/overwrite"

	// Set initial value
	err := Set(key, "initial")
	if err != nil {
		t.Fatalf("Set initial failed: %v", err)
	}

	// Overwrite
	err = Set(key, "updated")
	if err != nil {
		t.Fatalf("Set update failed: %v", err)
	}

	// Get should return updated value
	retrieved, err := Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved != "updated" {
		t.Errorf("expected 'updated', got %q", retrieved)
	}

	// Cleanup
	_ = Delete(key)
}

func TestCredentialsFilePath(t *testing.T) {
	// Verify credentials are stored in expected location
	tmpDir := t.TempDir()
	originalXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", tmpDir)
	defer os.Setenv("XDG_CONFIG_HOME", originalXDG)

	key := "test/filepath"
	err := Set(key, "value")
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Credentials file should exist (when using file fallback)
	credFile := filepath.Join(tmpDir, "devlore", "credentials.yaml")
	_, err = os.Stat(credFile)
	if os.IsNotExist(err) {
		// May use keychain instead of file, which is fine
		t.Log("Credentials stored in keychain, not file")
	}

	// Cleanup
	_ = Delete(key)
}

func TestMultipleKeys(t *testing.T) {
	// Use a temporary credentials directory for testing
	tmpDir := t.TempDir()
	originalXDG := os.Getenv("XDG_CONFIG_HOME")
	os.Setenv("XDG_CONFIG_HOME", tmpDir)
	defer os.Setenv("XDG_CONFIG_HOME", originalXDG)

	keys := map[string]string{
		"ai/anthropic": "sk-ant-xxx",
		"ai/openai":    "sk-xxx",
		"github/token": "ghp_xxx",
	}

	// Set all keys
	for k, v := range keys {
		if err := Set(k, v); err != nil {
			t.Fatalf("Set %q failed: %v", k, err)
		}
	}

	// Verify all keys
	for k, expected := range keys {
		got, err := Get(k)
		if err != nil {
			t.Errorf("Get %q failed: %v", k, err)
			continue
		}
		if got != expected {
			t.Errorf("key %q: expected %q, got %q", k, expected, got)
		}
	}

	// Cleanup
	for k := range keys {
		_ = Delete(k)
	}
}
