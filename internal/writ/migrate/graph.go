// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"path/filepath"

	"github.com/NobleFactor/devlore-cli/internal/execution"
)

// BuildMigrationGraph constructs an execution graph from directory mappings.
// The graph contains rename operations for directories that need to change
// from legacy naming (project-Platform) to writ naming (project.Platform).
//
// The graph is the executable artifact; MigrationAnalysis provides the
// non-executable understanding. Together they replace the legacy MigrationPlan.
func BuildMigrationGraph(sourceRoot string, mappings []DirectoryMapping) *execution.Graph {
	plan := execution.NewPlan("migrate")

	// Create rename nodes for each directory mapping.
	// These use git mv when available to preserve history.
	var prevNode *execution.Node
	for _, m := range mappings {
		source := filepath.Join(sourceRoot, m.SourceDir)
		target := filepath.Join(sourceRoot, m.TargetDir)

		node := plan.Rename(source, target)

		// Chain renames sequentially to avoid conflicts
		// (e.g., renaming parent before child)
		if prevNode != nil {
			plan.DependsOn(prevNode, node)
		}
		prevNode = node
	}

	return plan.Graph()
}

// BuildMigrationAnalysis constructs the analysis artifact from detection results
// and inventory data. This is the non-executable understanding of the source repo.
func BuildMigrationAnalysis(
	sourceRoot string,
	system SourceSystem,
	systemConfidence float64,
	entries []InventoryEntry,
	mappings []DirectoryMapping,
	encSystems []EncryptionSystem,
) *MigrationAnalysis {
	// Classify entries
	Classify(entries)

	// Analyze lifecycle scripts
	scripts := AnalyzeScripts(entries)

	// Detect encrypted secrets
	secretFindings := detectEncryptedSecrets(entries)

	// Compute stats
	stats := computeMigrationStats(entries, mappings)

	// Build observations
	observations := []string{}
	if systemConfidence > 0 {
		observations = append(observations, "Detected source system via registry signatures")
	}
	if len(mappings) > 0 {
		observations = append(observations, "Directory renames required for writ naming convention")
	}

	// Build warnings
	var warnings []string
	for _, enc := range encSystems {
		if enc != EncryptNone && enc != EncryptSOPS {
			warnings = append(warnings, string(enc)+" detected — writ uses SOPS for secrets")
		}
	}

	// Build recommendations
	recommendations := []string{}
	projects := UniqueProjects(entries)
	if len(projects) > 0 {
		recommendations = append(recommendations, "Run: writ deploy "+joinStrings(projects, " "))
	}
	hasUnencryptedSecrets := false
	for _, s := range secretFindings {
		if s.Encryption == EncryptNone {
			hasUnencryptedSecrets = true
			break
		}
	}
	if hasUnencryptedSecrets {
		recommendations = append(recommendations, "Create .sops.yaml and encrypt secrets")
	}
	if stats.LifecycleScripts > 0 {
		recommendations = append(recommendations, "Evaluate Install-*/Initialize-* scripts for lore package conversion")
	}
	recommendations = append(recommendations, "Consider packages.manifest for common tool installations")

	return &MigrationAnalysis{
		SourceRoot:        sourceRoot,
		System:            system,
		SystemConfidence:  systemConfidence,
		RepoLayer:         LayerPersonal, // Default; AI can refine this
		EncryptionSystems: encSystems,
		Projects:          projects,
		Platforms:         UniquePlatforms(entries),
		Scripts:           scripts,
		SecretFindings:    secretFindings,
		Observations:      observations,
		Warnings:          warnings,
		Recommendations:   recommendations,
		Stats:             stats,
	}
}

// detectEncryptedSecrets finds files with encryption signatures.
func detectEncryptedSecrets(entries []InventoryEntry) []SecretFinding {
	var findings []SecretFinding
	for _, e := range entries {
		enc := DetectEncryptedFile(e.AbsPath)
		if enc != EncryptNone {
			findings = append(findings, SecretFinding{
				RelPath:    e.RelPath,
				Encryption: enc,
				Reason:     "Encrypted with " + string(enc),
			})
		}
	}
	return findings
}

// computeMigrationStats computes summary statistics from entries and mappings.
func computeMigrationStats(entries []InventoryEntry, mappings []DirectoryMapping) MigrationStats {
	s := MigrationStats{
		TotalFiles: len(entries),
		Renames:    len(mappings),
		Projects:   len(UniqueProjects(entries)),
		Platforms:  len(UniquePlatforms(entries)),
	}
	for _, e := range entries {
		switch e.Class {
		case ClassStaticConfig:
			s.StaticConfigs++
		case ClassScript:
			s.Scripts++
		case ClassLifecycleScript:
			s.LifecycleScripts++
		case ClassSecret:
			s.Secrets++
		case ClassFont:
			s.Fonts++
		case ClassTemplate:
			s.Templates++
		case ClassCompletion:
			s.Completions++
		case ClassManPage:
			s.ManPages++
		case ClassBinary:
			s.Binaries++
		}
	}
	return s
}

// joinStrings joins strings with a separator.
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
