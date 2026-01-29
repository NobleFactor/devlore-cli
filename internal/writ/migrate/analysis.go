// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

// MigrationAnalysis holds non-executable understanding of a source repository.
// This is the "why" and "what to watch out for" — separate from the execution
// Graph which specifies "what to do".
//
// The execution Graph and MigrationAnalysis are produced together by BuildMigration().
// FormatMigrationPlan() renders both as human-readable output.
type MigrationAnalysis struct {
	// SourceRoot is the absolute path to the source repository.
	SourceRoot string `json:"source_root" yaml:"source_root"`

	// System is the detected source system (tuckr, stow, chezmoi, etc.).
	System SourceSystem `json:"system" yaml:"system"`

	// SystemConfidence is the detection confidence (0.0-1.0) when using
	// signature-based detection. Zero for heuristic detection.
	SystemConfidence float64 `json:"system_confidence,omitempty" yaml:"system_confidence,omitempty"`

	// RepoLayer indicates the precedence layer (base, team, personal).
	RepoLayer RepoLayer `json:"repo_layer" yaml:"repo_layer"`

	// EncryptionSystems lists detected encryption tools (git-crypt, sops, etc.).
	EncryptionSystems []EncryptionSystem `json:"encryption_systems" yaml:"encryption_systems"`

	// Projects lists unique project names found in the repository.
	Projects []string `json:"projects" yaml:"projects"`

	// Platforms lists unique platform names found in the repository.
	Platforms []string `json:"platforms" yaml:"platforms"`

	// Scripts contains analysis of lifecycle scripts.
	Scripts []ScriptAnalysis `json:"scripts,omitempty" yaml:"scripts,omitempty"`

	// SecretFindings contains detected secrets with explanations.
	SecretFindings []SecretFinding `json:"secret_findings,omitempty" yaml:"secret_findings,omitempty"`

	// Observations are insights about the repository structure.
	Observations []string `json:"observations,omitempty" yaml:"observations,omitempty"`

	// Warnings are concerns that may need attention before/after migration.
	Warnings []string `json:"warnings,omitempty" yaml:"warnings,omitempty"`

	// Recommendations are suggested actions after migration.
	Recommendations []string `json:"recommendations,omitempty" yaml:"recommendations,omitempty"`

	// Stats provides summary counts derived from the inventory.
	Stats MigrationStats `json:"stats" yaml:"stats"`
}

// MigrationStats summarizes the migration numerically.
type MigrationStats struct {
	TotalFiles       int `json:"total_files" yaml:"total_files"`
	StaticConfigs    int `json:"static_configs" yaml:"static_configs"`
	Scripts          int `json:"scripts" yaml:"scripts"`
	LifecycleScripts int `json:"lifecycle_scripts" yaml:"lifecycle_scripts"`
	Secrets          int `json:"secrets" yaml:"secrets"`
	Fonts            int `json:"fonts" yaml:"fonts"`
	Templates        int `json:"templates" yaml:"templates"`
	Completions      int `json:"completions" yaml:"completions"`
	ManPages         int `json:"man_pages" yaml:"man_pages"`
	Binaries         int `json:"binaries" yaml:"binaries"`
	Projects         int `json:"projects" yaml:"projects"`
	Platforms        int `json:"platforms" yaml:"platforms"`
	Renames          int `json:"renames" yaml:"renames"`
}

// RepoLayer indicates the precedence layer of a repository.
// Precedence: base (lowest) → team → personal (highest).
type RepoLayer string

const (
	LayerBase     RepoLayer = "base"
	LayerTeam     RepoLayer = "team"
	LayerPersonal RepoLayer = "personal"
)
