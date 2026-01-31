// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

// SourceSystem identifies the dotfile management approach used in the source repository.
type SourceSystem string

const (
	SystemNative      SourceSystem = "native"       // Already writ-compatible (Home/ or System/)
	SystemTuckr       SourceSystem = "tuckr"        // Tuckr dotfile manager
	SystemStow        SourceSystem = "stow"         // GNU Stow
	SystemChezmoi     SourceSystem = "chezmoi"      // chezmoi
	SystemYadm        SourceSystem = "yadm"         // yadm
	SystemBareGit     SourceSystem = "bare-git"     // Bare git repo as home
	SystemScriptBased SourceSystem = "script-based" // Custom install scripts
	SystemUnknown     SourceSystem = "unknown"
)

// EncryptionSystem identifies the secret encryption tool in use.
type EncryptionSystem string

const (
	EncryptGitCrypt     EncryptionSystem = "git-crypt"
	EncryptBlackbox     EncryptionSystem = "blackbox"
	EncryptTranscrypt   EncryptionSystem = "transcrypt"
	EncryptGPG          EncryptionSystem = "gpg"
	EncryptAge          EncryptionSystem = "age"
	EncryptAnsibleVault EncryptionSystem = "ansible-vault"
	EncryptSOPS         EncryptionSystem = "sops"
	EncryptNone         EncryptionSystem = "none"
)

// SecretFinding represents a detected secret file.
type SecretFinding struct {
	RelPath          string           `json:"rel_path" yaml:"rel_path"`
	Encryption       EncryptionSystem `json:"encryption" yaml:"encryption"`
	Reason           string           `json:"reason" yaml:"reason"`
	SuggestedPattern string           `json:"suggested_pattern,omitempty" yaml:"suggested_pattern,omitempty"`
}

// StructureInfo describes the detected repository structure.
type StructureInfo struct {
	// GroupsPath is where groups live (e.g., "Home/Configs").
	GroupsPath string `json:"groups_path" yaml:"groups_path"`

	// NamingConvention is the current naming pattern (e.g., "<group>-<Platform>").
	NamingConvention string `json:"naming_convention" yaml:"naming_convention"`

	// Groups is the list of group names found.
	Groups []string `json:"groups" yaml:"groups"`

	// Platforms is the list of platform names found.
	Platforms []string `json:"platforms" yaml:"platforms"`
}

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

	// SystemConfidence is the detection confidence (0.0-1.0).
	SystemConfidence float64 `json:"system_confidence,omitempty" yaml:"system_confidence,omitempty"`

	// InputSummary describes what the LLM saw in the inputs.
	InputSummary string `json:"input_summary,omitempty" yaml:"input_summary,omitempty"`

	// Structure describes the detected repository structure.
	Structure *StructureInfo `json:"structure,omitempty" yaml:"structure,omitempty"`

	// RepoLayer indicates the precedence layer (base, team, personal).
	RepoLayer RepoLayer `json:"repo_layer" yaml:"repo_layer"`

	// EncryptionSystems lists detected encryption tools (git-crypt, sops, etc.).
	EncryptionSystems []EncryptionSystem `json:"encryption_systems,omitempty" yaml:"encryption_systems,omitempty"`

	// Projects lists unique project names found in the repository.
	Projects []string `json:"projects,omitempty" yaml:"projects,omitempty"`

	// Platforms lists unique platform names found in the repository.
	Platforms []string `json:"platforms,omitempty" yaml:"platforms,omitempty"`

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
	Stats MigrationStats `json:"stats,omitempty" yaml:"stats,omitempty"`
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
