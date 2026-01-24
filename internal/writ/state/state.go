// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

// Package state provides merged deployment state tracking for writ.
// The state file aggregates all deployed files across multiple deployments,
// enabling complete status checks and upgrade operations.
package state

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/internal/writ/receipt"
)

// CurrentVersion is the state file format version.
const CurrentVersion = "1"

// State represents the merged deployment state.
type State struct {
	// Version is the state file format version.
	Version string `json:"version" yaml:"version"`

	// LastUpdated is when the state was last modified.
	LastUpdated time.Time `json:"last_updated" yaml:"last_updated"`

	// SourceRoot is the dotfiles repository path.
	SourceRoot string `json:"source_root" yaml:"source_root"`

	// TargetRoot is the deployment target (e.g., $HOME).
	TargetRoot string `json:"target_root" yaml:"target_root"`

	// Files maps relative target paths to their deployment info.
	Files map[string]*FileEntry `json:"files" yaml:"files"`

	// Signature contains the cryptographic signature (optional).
	Signature *receipt.Signature `json:"signature,omitempty" yaml:"signature,omitempty"`
}

// FileEntry represents a single deployed file in the state.
type FileEntry struct {
	// Source is the absolute path in the dotfiles repo.
	Source string `json:"source" yaml:"source"`

	// Project this file belongs to.
	Project string `json:"project" yaml:"project"`

	// Operations performed: link, expand, copy, decrypt.
	Operations []string `json:"operations" yaml:"operations"`

	// DeployedAt is when the file was deployed.
	DeployedAt time.Time `json:"deployed_at" yaml:"deployed_at"`

	// Receipt is the receipt filename that recorded this deployment.
	Receipt string `json:"receipt" yaml:"receipt"`

	// SourceChecksum is the SHA256 of the source file at deploy time.
	// Only set for copied files (expand, decrypt, copy operations).
	SourceChecksum string `json:"source_checksum,omitempty" yaml:"source_checksum,omitempty"`

	// TargetChecksum is the SHA256 of the target file after deployment.
	// Only set for copied files (expand, decrypt, copy operations).
	TargetChecksum string `json:"target_checksum,omitempty" yaml:"target_checksum,omitempty"`
}

// IsCopied returns true if this entry was copied (not symlinked).
func (e *FileEntry) IsCopied() bool {
	for _, op := range e.Operations {
		if op == "expand" || op == "decrypt" || op == "copy" {
			return true
		}
	}
	return false
}

// IsLinked returns true if this entry is a symlink.
func (e *FileEntry) IsLinked() bool {
	return len(e.Operations) == 1 && e.Operations[0] == "link"
}

// New creates a new empty state.
func New(sourceRoot, targetRoot string) *State {
	return &State{
		Version:     CurrentVersion,
		LastUpdated: time.Now(),
		SourceRoot:  sourceRoot,
		TargetRoot:  targetRoot,
		Files:       make(map[string]*FileEntry),
	}
}

// StateDir returns the writ state directory.
func StateDir() string {
	return filepath.Join(cli.StateHome(), "writ")
}

// StatePath returns the path to the state file.
func StatePath() string {
	return filepath.Join(StateDir(), "state.yaml")
}

// Load reads the state file from the default location.
func Load() (*State, error) {
	return LoadFrom(StatePath())
}

// LoadFrom reads a state file from a specific path.
func LoadFrom(path string) (*State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var s State
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}

	// Initialize map if nil (empty state file)
	if s.Files == nil {
		s.Files = make(map[string]*FileEntry)
	}

	return &s, nil
}

// LoadOrCreate loads the state file or creates a new one if it doesn't exist.
func LoadOrCreate(sourceRoot, targetRoot string) (*State, error) {
	s, err := Load()
	if err != nil {
		if os.IsNotExist(err) {
			return New(sourceRoot, targetRoot), nil
		}
		return nil, err
	}

	// Update roots if they've changed
	if s.SourceRoot != sourceRoot || s.TargetRoot != targetRoot {
		s.SourceRoot = sourceRoot
		s.TargetRoot = targetRoot
	}

	return s, nil
}

// Write saves the state file to the default location.
func (s *State) Write() error {
	return s.WriteTo(StatePath())
}

// WriteTo saves the state file to a specific path.
func (s *State) WriteTo(path string) error {
	s.LastUpdated = time.Now()

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	data, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write state: %w", err)
	}

	return nil
}

// AddEntry adds or updates a file entry in the state.
func (s *State) AddEntry(relTarget string, entry *FileEntry) {
	s.Files[relTarget] = entry
}

// RemoveEntry removes a file entry from the state.
func (s *State) RemoveEntry(relTarget string) {
	delete(s.Files, relTarget)
}

// RemoveProject removes all entries for a project from the state.
func (s *State) RemoveProject(project string) int {
	removed := 0
	for relTarget, entry := range s.Files {
		if entry.Project == project {
			delete(s.Files, relTarget)
			removed++
		}
	}
	return removed
}

// GetEntry returns the entry for a relative target path.
func (s *State) GetEntry(relTarget string) *FileEntry {
	return s.Files[relTarget]
}

// EntriesForProject returns all entries for a specific project.
func (s *State) EntriesForProject(project string) map[string]*FileEntry {
	result := make(map[string]*FileEntry)
	for relTarget, entry := range s.Files {
		if entry.Project == project {
			result[relTarget] = entry
		}
	}
	return result
}

// CopiedFiles returns all entries that are copied (not symlinked).
func (s *State) CopiedFiles() map[string]*FileEntry {
	result := make(map[string]*FileEntry)
	for relTarget, entry := range s.Files {
		if entry.IsCopied() {
			result[relTarget] = entry
		}
	}
	return result
}

// Projects returns a list of all projects with deployed files.
func (s *State) Projects() []string {
	seen := make(map[string]bool)
	for _, entry := range s.Files {
		seen[entry.Project] = true
	}

	projects := make([]string, 0, len(seen))
	for p := range seen {
		projects = append(projects, p)
	}
	return projects
}

// Summary returns counts of files by type.
func (s *State) Summary() (links, copied int) {
	for _, entry := range s.Files {
		if entry.IsCopied() {
			copied++
		} else {
			links++
		}
	}
	return
}

// UpdateFromReceipt merges nodes from a v4 graph-format receipt into the state.
func (s *State) UpdateFromReceipt(rcpt *receipt.Receipt, receiptFilename string) {
	for _, n := range rcpt.Nodes {
		// Skip non-file nodes
		if n.Status == "skipped" || n.Operation == "delegate" || n.Operation == "backup" {
			continue
		}

		entry := &FileEntry{
			Source:         n.Source,
			Project:        n.Project,
			Operations:     []string{n.Operation},
			DeployedAt:     rcpt.Timestamp,
			Receipt:        receiptFilename,
			SourceChecksum: n.SourceChecksum,
			TargetChecksum: n.TargetChecksum,
		}
		s.AddEntry(n.ID, entry) // ID = rel_target
	}
}

// UpdateChecksum updates the checksums for a file entry.
func (s *State) UpdateChecksum(relTarget, sourceChecksum, targetChecksum string) {
	if entry := s.Files[relTarget]; entry != nil {
		entry.SourceChecksum = sourceChecksum
		entry.TargetChecksum = targetChecksum
	}
}
