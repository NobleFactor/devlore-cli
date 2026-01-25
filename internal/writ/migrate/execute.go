// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package migrate

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// MigratedMarker records what was done during execution.
type MigratedMarker struct {
	Timestamp string             `yaml:"timestamp"`
	System    SourceSystem       `yaml:"system"`
	Mappings  []DirectoryMapping `yaml:"mappings"`
}

// Execute performs the directory renames specified in the migration plan.
// It writes progress to the given writer (typically os.Stderr).
func Execute(w io.Writer, plan *MigrationPlan) error {
	if len(plan.Mappings) == 0 {
		fmt.Fprintf(w, "No renames needed.\n")
		return nil
	}

	fmt.Fprintf(w, "Migrating: %s → writ (%d directory renames)\n", plan.System, len(plan.Mappings))

	// Verify no target conflicts before starting
	for _, m := range plan.Mappings {
		targetPath := filepath.Join(plan.SourceRoot, m.TargetDir)
		if exists(targetPath) {
			return fmt.Errorf("target directory %q already exists; aborting", m.TargetDir)
		}
	}

	// Perform renames
	maxLen := 0
	for _, m := range plan.Mappings {
		if len(m.SourceDir) > maxLen {
			maxLen = len(m.SourceDir)
		}
	}

	for _, m := range plan.Mappings {
		srcPath := filepath.Join(plan.SourceRoot, m.SourceDir)
		dstPath := filepath.Join(plan.SourceRoot, m.TargetDir)

		if err := os.Rename(srcPath, dstPath); err != nil {
			fmt.Fprintf(w, "  %-*s  →  %-*s  ✗\n", maxLen, m.SourceDir, maxLen, m.TargetDir)
			return fmt.Errorf("rename %s → %s: %w", m.SourceDir, m.TargetDir, err)
		}
		fmt.Fprintf(w, "  %-*s  →  %-*s  ✓\n", maxLen, m.SourceDir, maxLen, m.TargetDir)
	}

	// Write marker file
	marker := MigratedMarker{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		System:    plan.System,
		Mappings:  plan.Mappings,
	}
	markerPath := filepath.Join(plan.SourceRoot, ".writ-migrated")
	data, err := yaml.Marshal(&marker)
	if err != nil {
		return fmt.Errorf("marshal marker: %w", err)
	}
	if err := os.WriteFile(markerPath, data, 0644); err != nil {
		return fmt.Errorf("write marker: %w", err)
	}

	fmt.Fprintf(w, "\nWrote .writ-migrated marker.\n")
	fmt.Fprintf(w, "Migration complete. Next steps:\n")

	projects := UniqueProjects(plan.Entries)
	fmt.Fprintf(w, "  git add -A && git commit -m \"Migrate to writ naming conventions\"\n")
	fmt.Fprintf(w, "  writ add %s\n", joinWords(projects))

	return nil
}

func joinWords(words []string) string {
	result := ""
	for i, w := range words {
		if i > 0 {
			result += " "
		}
		result += w
	}
	return result
}
