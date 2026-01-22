// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

// Package receipt provides deployment receipt tracking for writ.
package receipt

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/internal/writ/tree"
)

// Receipt records a writ deployment operation.
type Receipt struct {
	// Version is the receipt format version.
	Version string `json:"version" yaml:"version"`

	// Timestamp is when the deployment occurred.
	Timestamp time.Time `json:"timestamp" yaml:"timestamp"`

	// SourceRoot is the dotfiles repository path.
	SourceRoot string `json:"source_root" yaml:"source_root"`

	// TargetRoot is the deployment target (e.g., $HOME).
	TargetRoot string `json:"target_root" yaml:"target_root"`

	// Projects deployed.
	Projects []string `json:"projects" yaml:"projects"`

	// Segments used for matching.
	Segments map[string]string `json:"segments" yaml:"segments"`

	// Entries are the individual deployed items.
	Entries []Entry `json:"entries" yaml:"entries"`

	// Backups created during deployment.
	Backups []Backup `json:"backups,omitempty" yaml:"backups,omitempty"`

	// Skipped files (due to conflicts with --skip).
	Skipped []string `json:"skipped,omitempty" yaml:"skipped,omitempty"`

	// Delegated manifests (passed to lore).
	Delegated []string `json:"delegated,omitempty" yaml:"delegated,omitempty"`

	// Summary statistics.
	Summary Summary `json:"summary" yaml:"summary"`

	// Signature contains the cryptographic signature (v3+).
	Signature *Signature `json:"signature,omitempty" yaml:"signature,omitempty"`
}

// Entry records a single deployed file.
type Entry struct {
	// Source is the absolute path in the dotfiles repo.
	Source string `json:"source" yaml:"source"`

	// Target is the absolute path in the target location.
	Target string `json:"target" yaml:"target"`

	// RelTarget is the relative path from target root.
	RelTarget string `json:"rel_target" yaml:"rel_target"`

	// Operation performed: link, copy, expand, decrypt.
	Operations []string `json:"operations" yaml:"operations"`

	// Project this file belongs to.
	Project string `json:"project" yaml:"project"`

	// AlreadyDeployed indicates the symlink already existed correctly.
	AlreadyDeployed bool `json:"already_deployed,omitempty" yaml:"already_deployed,omitempty"`

	// SourceChecksum is the SHA256 of the source file at deploy time.
	// Only set for copied files (expand, decrypt, copy operations).
	// Format: "sha256:<hex>"
	SourceChecksum string `json:"source_checksum,omitempty" yaml:"source_checksum,omitempty"`

	// TargetChecksum is the SHA256 of the target file after deployment.
	// Only set for copied files (expand, decrypt, copy operations).
	// Format: "sha256:<hex>"
	TargetChecksum string `json:"target_checksum,omitempty" yaml:"target_checksum,omitempty"`
}

// Backup records a backed-up file.
type Backup struct {
	// Original is the path that was backed up.
	Original string `json:"original" yaml:"original"`

	// BackupPath is where it was moved to.
	BackupPath string `json:"backup_path" yaml:"backup_path"`
}

// Summary contains deployment statistics.
type Summary struct {
	TotalFiles      int `json:"total_files" yaml:"total_files"`
	Links           int `json:"links" yaml:"links"`
	Copies          int `json:"copies" yaml:"copies"`
	Templates       int `json:"templates" yaml:"templates"`
	Secrets         int `json:"secrets" yaml:"secrets"`
	AlreadyDeployed int `json:"already_deployed" yaml:"already_deployed"`
	Skipped         int `json:"skipped" yaml:"skipped"`
	BackedUp        int `json:"backed_up" yaml:"backed_up"`
	Delegated       int `json:"delegated" yaml:"delegated"`
}

// CurrentVersion is the receipt format version.
// v1: Initial format
// v2: Added SourceChecksum, TargetChecksum for copied files
const CurrentVersion = "2"

// New creates a new receipt with the given configuration.
func New(sourceRoot, targetRoot string, projects []string, segments map[string]string) *Receipt {
	return &Receipt{
		Version:    CurrentVersion,
		Timestamp:  time.Now(),
		SourceRoot: sourceRoot,
		TargetRoot: targetRoot,
		Projects:   projects,
		Segments:   segments,
		Entries:    make([]Entry, 0),
	}
}

// AddEntry adds a deployed file entry to the receipt.
func (r *Receipt) AddEntry(node *tree.Node, alreadyDeployed bool) {
	r.Entries = append(r.Entries, Entry{
		Source:          node.Source,
		Target:          node.Target,
		RelTarget:       node.RelTarget,
		Operations:      node.Operations.Strings(),
		Project:         node.Project,
		AlreadyDeployed: alreadyDeployed,
	})
}

// AddEntryWithChecksums adds a deployed file entry with content checksums.
// Use this for copied files (expand, decrypt, copy operations).
func (r *Receipt) AddEntryWithChecksums(node *tree.Node, alreadyDeployed bool, sourceChecksum, targetChecksum string) {
	r.Entries = append(r.Entries, Entry{
		Source:          node.Source,
		Target:          node.Target,
		RelTarget:       node.RelTarget,
		Operations:      node.Operations.Strings(),
		Project:         node.Project,
		AlreadyDeployed: alreadyDeployed,
		SourceChecksum:  sourceChecksum,
		TargetChecksum:  targetChecksum,
	})
}

// Checksum computes a SHA256 checksum of the given content.
// Returns format "sha256:<hex>".
func Checksum(content []byte) string {
	hash := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(hash[:])
}

// ChecksumFile computes a SHA256 checksum of a file's contents.
// Returns format "sha256:<hex>" or empty string on error.
func ChecksumFile(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return Checksum(content)
}

// IsCopied returns true if this entry was copied (not symlinked).
func (e *Entry) IsCopied() bool {
	for _, op := range e.Operations {
		if op == "expand" || op == "decrypt" || op == "copy" {
			return true
		}
	}
	return false
}

// IsLinked returns true if this entry is a symlink.
func (e *Entry) IsLinked() bool {
	return len(e.Operations) == 1 && e.Operations[0] == "link"
}

// AddBackup records a backup that was created.
func (r *Receipt) AddBackup(original, backupPath string) {
	r.Backups = append(r.Backups, Backup{
		Original:   original,
		BackupPath: backupPath,
	})
}

// AddSkipped records a skipped file.
func (r *Receipt) AddSkipped(relTarget string) {
	r.Skipped = append(r.Skipped, relTarget)
}

// AddDelegated records a delegated manifest.
func (r *Receipt) AddDelegated(source string) {
	r.Delegated = append(r.Delegated, source)
}

// ComputeSummary calculates summary statistics from entries.
func (r *Receipt) ComputeSummary() {
	r.Summary = Summary{
		TotalFiles: len(r.Entries),
		BackedUp:   len(r.Backups),
		Skipped:    len(r.Skipped),
		Delegated:  len(r.Delegated),
	}

	for _, e := range r.Entries {
		if e.AlreadyDeployed {
			r.Summary.AlreadyDeployed++
			continue
		}

		for _, op := range e.Operations {
			switch op {
			case "link":
				r.Summary.Links++
			case "copy":
				r.Summary.Copies++
			case "expand":
				r.Summary.Templates++
			case "decrypt":
				r.Summary.Secrets++
			}
		}
	}
}

// StateDir returns the writ state directory.
// Default: ~/.local/state/writ
func StateDir() string {
	return filepath.Join(cli.StateHome(), "writ")
}

// ReceiptsDir returns the receipts directory.
func ReceiptsDir() string {
	return filepath.Join(StateDir(), "receipts")
}

// LatestReceiptPath returns the path to the "latest" symlink.
func LatestReceiptPath() string {
	return filepath.Join(ReceiptsDir(), "latest.yaml")
}

// Write saves the receipt to the state directory.
// Returns the path where it was written.
func (r *Receipt) Write() (string, error) {
	r.ComputeSummary()

	dir := ReceiptsDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create receipts dir: %w", err)
	}

	// Filename: YYYY-MM-DDTHH-MM-SS.yaml
	filename := r.Timestamp.Format("2006-01-02T15-04-05") + ".yaml"
	path := filepath.Join(dir, filename)

	data, err := yaml.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("marshal receipt: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("write receipt: %w", err)
	}

	// Update "latest" symlink
	latestPath := LatestReceiptPath()
	_ = os.Remove(latestPath) // Ignore error if doesn't exist
	if err := os.Symlink(filename, latestPath); err != nil {
		// Non-fatal, just log if verbose
	}

	return path, nil
}

// LoadLatest loads the most recent receipt.
func LoadLatest() (*Receipt, error) {
	return Load(LatestReceiptPath())
}

// Load reads a receipt from a file.
func Load(path string) (*Receipt, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var r Receipt
	if err := yaml.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse receipt: %w", err)
	}

	return &r, nil
}

// JSON returns the receipt as JSON.
func (r *Receipt) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// YAML returns the receipt as YAML.
func (r *Receipt) YAML() ([]byte, error) {
	return yaml.Marshal(r)
}

// String returns a human-readable summary of the receipt.
func (r *Receipt) String() string {
	r.ComputeSummary()
	return fmt.Sprintf("%d files (%d links, %d templates, %d secrets), %d already deployed, %d skipped, %d backed up",
		r.Summary.TotalFiles,
		r.Summary.Links,
		r.Summary.Templates,
		r.Summary.Secrets,
		r.Summary.AlreadyDeployed,
		r.Summary.Skipped,
		r.Summary.BackedUp,
	)
}
