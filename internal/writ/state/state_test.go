// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package state

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"filippo.io/age"

	"github.com/NobleFactor/devlore-cli/internal/writ/receipt"
)

func TestNewState(t *testing.T) {
	s := New("/home/user/dotfiles", "/home/user")

	if s.Version != CurrentVersion {
		t.Errorf("expected version %q, got %q", CurrentVersion, s.Version)
	}
	if s.SourceRoot != "/home/user/dotfiles" {
		t.Errorf("expected source root %q, got %q", "/home/user/dotfiles", s.SourceRoot)
	}
	if s.TargetRoot != "/home/user" {
		t.Errorf("expected target root %q, got %q", "/home/user", s.TargetRoot)
	}
	if s.Files == nil {
		t.Error("expected Files map to be initialized")
	}
}

func TestStateAddRemoveEntry(t *testing.T) {
	s := New("/home/user/dotfiles", "/home/user")

	entry := &FileEntry{
		Source:     "/home/user/dotfiles/all/.bashrc",
		Project:    "all",
		Operations: []string{"link"},
		DeployedAt: time.Now(),
		Receipt:    "2026-01-21T10-30-00.yaml",
	}

	s.AddEntry(".bashrc", entry)

	if got := s.GetEntry(".bashrc"); got != entry {
		t.Error("expected to get entry back")
	}

	s.RemoveEntry(".bashrc")

	if got := s.GetEntry(".bashrc"); got != nil {
		t.Error("expected entry to be removed")
	}
}

func TestStateRemoveProject(t *testing.T) {
	s := New("/home/user/dotfiles", "/home/user")

	// Add entries for two projects
	s.AddEntry(".bashrc", &FileEntry{Project: "all", Operations: []string{"link"}})
	s.AddEntry(".zshrc", &FileEntry{Project: "all", Operations: []string{"link"}})
	s.AddEntry(".gitconfig", &FileEntry{Project: "noblefactor", Operations: []string{"expand", "copy"}})

	removed := s.RemoveProject("all")

	if removed != 2 {
		t.Errorf("expected 2 entries removed, got %d", removed)
	}

	if len(s.Files) != 1 {
		t.Errorf("expected 1 entry remaining, got %d", len(s.Files))
	}

	if s.GetEntry(".gitconfig") == nil {
		t.Error("expected .gitconfig to remain")
	}
}

func TestStateCopiedFiles(t *testing.T) {
	s := New("/home/user/dotfiles", "/home/user")

	s.AddEntry(".bashrc", &FileEntry{Operations: []string{"link"}})
	s.AddEntry(".gitconfig", &FileEntry{Operations: []string{"expand", "copy"}})
	s.AddEntry(".npmrc", &FileEntry{Operations: []string{"decrypt", "copy"}})

	copied := s.CopiedFiles()

	if len(copied) != 2 {
		t.Errorf("expected 2 copied files, got %d", len(copied))
	}

	if _, ok := copied[".bashrc"]; ok {
		t.Error("expected .bashrc (link) to not be in copied files")
	}
}

func TestStateProjects(t *testing.T) {
	s := New("/home/user/dotfiles", "/home/user")

	s.AddEntry(".bashrc", &FileEntry{Project: "all"})
	s.AddEntry(".zshrc", &FileEntry{Project: "all"})
	s.AddEntry(".gitconfig", &FileEntry{Project: "noblefactor"})

	projects := s.Projects()

	if len(projects) != 2 {
		t.Errorf("expected 2 projects, got %d", len(projects))
	}
}

func TestStateSummary(t *testing.T) {
	s := New("/home/user/dotfiles", "/home/user")

	s.AddEntry(".bashrc", &FileEntry{Operations: []string{"link"}})
	s.AddEntry(".zshrc", &FileEntry{Operations: []string{"link"}})
	s.AddEntry(".gitconfig", &FileEntry{Operations: []string{"expand", "copy"}})

	links, copied := s.Summary()

	if links != 2 {
		t.Errorf("expected 2 links, got %d", links)
	}
	if copied != 1 {
		t.Errorf("expected 1 copied, got %d", copied)
	}
}

func TestStateWriteAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "state.yaml")

	s := New("/home/user/dotfiles", "/home/user")
	s.AddEntry(".bashrc", &FileEntry{
		Source:     "/home/user/dotfiles/all/.bashrc",
		Project:    "all",
		Operations: []string{"link"},
		DeployedAt: time.Now(),
		Receipt:    "2026-01-21T10-30-00.yaml",
	})

	if err := s.WriteTo(statePath); err != nil {
		t.Fatalf("write state: %v", err)
	}

	loaded, err := LoadFrom(statePath)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}

	if loaded.Version != CurrentVersion {
		t.Errorf("expected version %q, got %q", CurrentVersion, loaded.Version)
	}
	if loaded.SourceRoot != "/home/user/dotfiles" {
		t.Errorf("expected source root %q, got %q", "/home/user/dotfiles", loaded.SourceRoot)
	}
	if len(loaded.Files) != 1 {
		t.Errorf("expected 1 file, got %d", len(loaded.Files))
	}
	if loaded.GetEntry(".bashrc") == nil {
		t.Error("expected .bashrc entry")
	}
}

func TestStateUpdateFromReceipt(t *testing.T) {
	s := New("/home/user/dotfiles", "/home/user")

	rcpt := &receipt.Receipt{
		Timestamp:  time.Now(),
		SourceRoot: "/home/user/dotfiles",
		TargetRoot: "/home/user",
		Projects:   []string{"all", "noblefactor"},
		Entries: []receipt.Entry{
			{
				Source:     "/home/user/dotfiles/all/.bashrc",
				Target:     "/home/user/.bashrc",
				RelTarget:  ".bashrc",
				Operations: []string{"link"},
				Project:    "all",
			},
			{
				Source:         "/home/user/dotfiles/noblefactor/.gitconfig.template",
				Target:         "/home/user/.gitconfig",
				RelTarget:      ".gitconfig",
				Operations:     []string{"expand", "copy"},
				Project:        "noblefactor",
				SourceChecksum: "sha256:abc123",
				TargetChecksum: "sha256:def456",
			},
		},
	}

	s.UpdateFromReceipt(rcpt, "2026-01-21T10-30-00.yaml")

	if len(s.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(s.Files))
	}

	bashrc := s.GetEntry(".bashrc")
	if bashrc == nil {
		t.Fatal("expected .bashrc entry")
	}
	if bashrc.Project != "all" {
		t.Errorf("expected project 'all', got %q", bashrc.Project)
	}

	gitconfig := s.GetEntry(".gitconfig")
	if gitconfig == nil {
		t.Fatal("expected .gitconfig entry")
	}
	if gitconfig.SourceChecksum != "sha256:abc123" {
		t.Errorf("expected source checksum 'sha256:abc123', got %q", gitconfig.SourceChecksum)
	}
}

func TestFileEntryIsCopied(t *testing.T) {
	tests := []struct {
		ops      []string
		isCopied bool
	}{
		{[]string{"link"}, false},
		{[]string{"expand", "copy"}, true},
		{[]string{"decrypt", "copy"}, true},
		{[]string{"decrypt", "expand", "copy"}, true},
		{[]string{"delegate"}, false},
	}

	for _, tt := range tests {
		entry := &FileEntry{Operations: tt.ops}
		if got := entry.IsCopied(); got != tt.isCopied {
			t.Errorf("IsCopied(%v) = %v, want %v", tt.ops, got, tt.isCopied)
		}
	}
}

func TestStateSignAndVerify(t *testing.T) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}

	s := New("/home/user/dotfiles", "/home/user")
	s.AddEntry(".bashrc", &FileEntry{
		Source:     "/home/user/dotfiles/all/.bashrc",
		Project:    "all",
		Operations: []string{"link"},
		DeployedAt: time.Now(),
	})

	if err := s.Sign(identity); err != nil {
		t.Fatalf("sign state: %v", err)
	}

	if !s.IsSigned() {
		t.Error("expected state to be signed")
	}

	if err := s.Verify([]age.Identity{identity}); err != nil {
		t.Fatalf("verify state: %v", err)
	}
}

func TestStateVerifyTampered(t *testing.T) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}

	s := New("/home/user/dotfiles", "/home/user")
	s.AddEntry(".bashrc", &FileEntry{
		Source:  "/home/user/dotfiles/all/.bashrc",
		Project: "all",
	})

	if err := s.Sign(identity); err != nil {
		t.Fatalf("sign state: %v", err)
	}

	// Tamper with state
	s.AddEntry(".tampered", &FileEntry{Project: "tampered"})

	if err := s.Verify([]age.Identity{identity}); err == nil {
		t.Error("expected verification to fail on tampered state")
	}
}

func TestLoadOrCreate(t *testing.T) {
	// Test with non-existent file
	tmpDir := t.TempDir()

	// Temporarily override StateDir
	origStateDir := os.Getenv("XDG_STATE_HOME")
	os.Setenv("XDG_STATE_HOME", tmpDir)
	defer os.Setenv("XDG_STATE_HOME", origStateDir)

	s, err := LoadOrCreate("/home/user/dotfiles", "/home/user")
	if err != nil {
		t.Fatalf("LoadOrCreate: %v", err)
	}

	if s.SourceRoot != "/home/user/dotfiles" {
		t.Errorf("expected source root %q, got %q", "/home/user/dotfiles", s.SourceRoot)
	}
}
