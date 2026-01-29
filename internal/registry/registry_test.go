// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package registry

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewDefault(t *testing.T) {
	client, err := NewDefault()
	if err != nil {
		t.Fatalf("NewDefault() error: %v", err)
	}

	if client.name != "central" {
		t.Errorf("expected name 'central', got %q", client.name)
	}

	if client.provider.Name() != "git" {
		t.Errorf("expected provider 'git', got %q", client.provider.Name())
	}
}

func TestClient_FilePaths(t *testing.T) {
	client := New("test", nil, "/tmp/test-cache")

	tests := []struct {
		relPath  string
		expected string
	}{
		{"ai/prompts/migrate-to-writ.txt", "/tmp/test-cache/ai/prompts/migrate-to-writ.txt"},
		{"packages/docker/lifecycle.yaml", "/tmp/test-cache/packages/docker/lifecycle.yaml"},
		{"INDEX.yaml", "/tmp/test-cache/INDEX.yaml"},
	}

	for _, tt := range tests {
		got := client.FilePath(tt.relPath)
		if got != tt.expected {
			t.Errorf("FilePath(%q) = %q, want %q", tt.relPath, got, tt.expected)
		}
	}
}

func TestGitProvider_Name(t *testing.T) {
	provider := NewGitProvider("https://github.com/example/repo.git", "main")
	if provider.Name() != "git" {
		t.Errorf("Name() = %q, want 'git'", provider.Name())
	}
}

func TestClient_SyncIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Skip if no network
	if os.Getenv("SKIP_NETWORK_TESTS") != "" {
		t.Skip("skipping network test")
	}

	// Use temp directory for test cache
	tmpDir, err := os.MkdirTemp("", "registry-test-*")
	if err != nil {
		t.Fatalf("creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	provider := NewGitProvider(
		"https://github.com/NobleFactor/devlore-registry.git",
		"develop", // develop branch has AI assets
	)
	client := New("test", provider, filepath.Join(tmpDir, "central"))

	// First sync (clone)
	ctx := context.Background()
	result, err := client.Sync(ctx, SyncOptions{})
	if err != nil {
		t.Fatalf("first Sync() error: %v", err)
	}

	if !result.FromClone {
		t.Error("expected FromClone=true for first sync")
	}
	if !result.Updated {
		t.Error("expected Updated=true for first sync")
	}
	if result.ToRef == "" {
		t.Error("expected non-empty ToRef")
	}

	// Verify cache exists
	if !client.Exists() {
		t.Error("expected cache to exist after sync")
	}

	// Verify we can read knowledge assets (if they exist in registry)
	if client.FileExists("knowledge/migration/prompts/migrate-to-writ.txt") {
		// Read prompt via Knowledge domain API
		prompt, err := client.Knowledge("migration").Prompt("migrate-to-writ.txt")
		if err != nil {
			t.Errorf("Knowledge(migration).Prompt() error: %v", err)
		}
		if prompt == "" {
			t.Error("expected non-empty prompt")
		}
	} else {
		t.Log("knowledge/migration/prompts/migrate-to-writ.txt not yet in registry, skipping prompt check")
	}

	// Verify Knowledge().Index() can load and parse index.yaml with metadata
	if client.FileExists("knowledge/migration/index.yaml") {
		index, err := client.Knowledge("migration").Index()
		if err != nil {
			t.Errorf("Knowledge(migration).Index() error: %v", err)
		}
		if index == nil {
			t.Fatal("expected non-nil index")
		}
		if index.Domain != "migration" {
			t.Errorf("index.Domain = %q, want 'migration'", index.Domain)
		}
		// Verify index lists expected asset types
		if len(index.Prompts) == 0 {
			t.Error("expected index.Prompts to be non-empty")
		}
		if len(index.Signatures) == 0 {
			t.Error("expected index.Signatures to be non-empty")
		}
		t.Logf("migration index: %d prompts, %d schemas, %d signatures",
			len(index.Prompts), len(index.Schemas), len(index.Signatures))

		// Test discovery methods
		promptName := index.PromptByPurpose("writ-migration")
		if promptName == "" {
			t.Error("PromptByPurpose('writ-migration') returned empty string")
		} else {
			t.Logf("PromptByPurpose('writ-migration') = %q", promptName)
		}

		transformName := index.TransformBySourceSystem("stow")
		if transformName == "" {
			t.Error("TransformBySourceSystem('stow') returned empty string")
		} else {
			t.Logf("TransformBySourceSystem('stow') = %q", transformName)
		}

		sigNames := index.SignatureNames()
		if len(sigNames) == 0 {
			t.Error("SignatureNames() returned empty slice")
		} else {
			t.Logf("SignatureNames() = %v", sigNames)
		}
	} else {
		t.Log("knowledge/migration/index.yaml not yet in registry, skipping index check")
	}

	// Second sync (pull)
	result2, err := client.Sync(ctx, SyncOptions{})
	if err != nil {
		t.Fatalf("second Sync() error: %v", err)
	}

	if result2.FromClone {
		t.Error("expected FromClone=false for second sync")
	}
	if result2.FromRef != result.ToRef {
		t.Errorf("expected FromRef=%q, got %q", result.ToRef, result2.FromRef)
	}

	// Check sync info
	info, err := client.SyncInfo()
	if err != nil {
		t.Errorf("SyncInfo() error: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil SyncInfo")
	}
	if info.Provider != "git" {
		t.Errorf("expected Provider='git', got %q", info.Provider)
	}
}
