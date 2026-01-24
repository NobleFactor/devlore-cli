// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package migrate

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// Stats summarizes the migration plan numerically.
type Stats struct {
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

// MigrationPlan represents the complete analysis of a migration.
type MigrationPlan struct {
	SourceRoot   string             `json:"source_root" yaml:"source_root"`
	System       SourceSystem       `json:"system" yaml:"system"`
	Entries      []InventoryEntry   `json:"entries" yaml:"entries"`
	Mappings     []DirectoryMapping `json:"mappings" yaml:"mappings"`
	Scripts      []ScriptAnalysis   `json:"scripts" yaml:"scripts"`
	Stats        Stats              `json:"stats" yaml:"stats"`
	Observations []string           `json:"observations" yaml:"observations"`
	Warnings     []string           `json:"warnings" yaml:"warnings"`
}

// Options controls migration behavior.
type Options struct {
	SourceRoot string
	TargetRoot string // empty = rename in place
	Execute    bool
	Verbose    bool
	Format     string // "text", "yaml", "json"
}

// BuildPlan performs detection, inventory, classification, analysis, and
// assembles a complete migration plan.
func BuildPlan(opts Options) (*MigrationPlan, error) {
	root := opts.SourceRoot

	// Detect source system
	system, err := Detect(root)
	if err != nil {
		return nil, fmt.Errorf("detection failed: %w", err)
	}
	if system == SystemUnknown {
		return nil, fmt.Errorf("could not detect source system in %s; specify with --system", root)
	}

	// Check for prior migration
	if exists(root + "/.writ-migrated") {
		return nil, fmt.Errorf("already migrated (found .writ-migrated); remove it to re-run")
	}

	// Inventory
	entries, err := Inventory(root)
	if err != nil {
		return nil, fmt.Errorf("inventory failed: %w", err)
	}

	// Classify
	Classify(entries)

	// Build mappings
	mappings, err := BuildMappings(root)
	if err != nil {
		return nil, fmt.Errorf("mapping failed: %w", err)
	}

	// Analyze lifecycle scripts
	scripts := AnalyzeScripts(entries)

	// Compute stats
	stats := computeStats(entries, mappings)

	// Generate observations and warnings
	observations := generateObservations(entries, mappings, scripts)
	warnings := generateWarnings(entries, scripts)

	plan := &MigrationPlan{
		SourceRoot:   root,
		System:       system,
		Entries:      entries,
		Mappings:     mappings,
		Scripts:      scripts,
		Stats:        stats,
		Observations: observations,
		Warnings:     warnings,
	}

	return plan, nil
}

func computeStats(entries []InventoryEntry, mappings []DirectoryMapping) Stats {
	s := Stats{
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

func generateObservations(entries []InventoryEntry, mappings []DirectoryMapping, scripts []ScriptAnalysis) []string {
	var obs []string

	// Check for tuckr install scripts (obsolete after migration)
	for _, s := range scripts {
		if strings.Contains(strings.ToLower(s.Name), "tuckr") {
			obs = append(obs, s.Name+" can be removed post-migration (writ replaces tuckr)")
			break
		}
	}

	// Check for secrets directories
	secretDirs := 0
	for _, e := range entries {
		if e.Class == ClassSecret && containsSecretsDir(e.RelPath) {
			secretDirs++
		}
	}
	if secretDirs > 0 {
		obs = append(obs, fmt.Sprintf("%d secret files in secrets directories — recommend .age encryption", secretDirs))
	}

	// Font files
	fontCount := 0
	for _, e := range entries {
		if e.Class == ClassFont {
			fontCount++
		}
	}
	if fontCount > 0 {
		obs = append(obs, fmt.Sprintf("%d font files — these symlink normally", fontCount))
	}

	return obs
}

func generateWarnings(entries []InventoryEntry, scripts []ScriptAnalysis) []string {
	var warnings []string

	// Unencrypted secrets
	hasUnencryptedSecrets := false
	for _, e := range entries {
		if e.Class == ClassSecret {
			ext := strings.ToLower(strings.TrimSpace(e.RelPath))
			if !strings.HasSuffix(ext, ".age") && !strings.HasSuffix(ext, ".sops") {
				hasUnencryptedSecrets = true
				break
			}
		}
	}
	if hasUnencryptedSecrets {
		warnings = append(warnings, "Secret files stored without encryption; writ expects .age/.sops")
	}

	// Tuckr scripts obsolete
	for _, s := range scripts {
		if strings.Contains(strings.ToLower(s.Name), "tuckr") {
			warnings = append(warnings, s.Name+" becomes obsolete after migration")
			break
		}
	}

	return warnings
}

// FormatPlan writes the migration plan in the specified format.
func FormatPlan(w io.Writer, plan *MigrationPlan, format string) error {
	switch format {
	case "yaml":
		return formatYAML(w, plan)
	case "json":
		return formatJSON(w, plan)
	default:
		return formatText(w, plan)
	}
}

func formatYAML(w io.Writer, plan *MigrationPlan) error {
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	return enc.Encode(plan)
}

func formatJSON(w io.Writer, plan *MigrationPlan) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(plan)
}

func formatText(w io.Writer, plan *MigrationPlan) error {
	fmt.Fprintf(w, "Migration Plan\n")
	fmt.Fprintf(w, "Source: %s\n", plan.SourceRoot)
	fmt.Fprintf(w, "System: %s\n", plan.System)
	fmt.Fprintln(w)

	// Summary
	fmt.Fprintf(w, "Summary:\n")
	fmt.Fprintf(w, "  Files: %d | Projects: %d | Platforms: %d\n",
		plan.Stats.TotalFiles, plan.Stats.Projects, plan.Stats.Platforms)
	fmt.Fprintf(w, "  Configs: %d | Scripts: %d | Lifecycle: %d\n",
		plan.Stats.StaticConfigs, plan.Stats.Scripts, plan.Stats.LifecycleScripts)

	extras := []string{}
	if plan.Stats.Secrets > 0 {
		extras = append(extras, fmt.Sprintf("Secrets: %d", plan.Stats.Secrets))
	}
	if plan.Stats.Fonts > 0 {
		extras = append(extras, fmt.Sprintf("Fonts: %d", plan.Stats.Fonts))
	}
	if plan.Stats.Completions > 0 {
		extras = append(extras, fmt.Sprintf("Completions: %d", plan.Stats.Completions))
	}
	if plan.Stats.Templates > 0 {
		extras = append(extras, fmt.Sprintf("Templates: %d", plan.Stats.Templates))
	}
	if len(extras) > 0 {
		fmt.Fprintf(w, "  %s\n", strings.Join(extras, " | "))
	}
	fmt.Fprintln(w)

	// Directory renames
	if len(plan.Mappings) > 0 {
		fmt.Fprintf(w, "Directory renames (%d):\n", len(plan.Mappings))
		maxLen := 0
		for _, m := range plan.Mappings {
			if len(m.SourceDir) > maxLen {
				maxLen = len(m.SourceDir)
			}
		}
		for _, m := range plan.Mappings {
			fmt.Fprintf(w, "  %-*s  →  %s\n", maxLen, m.SourceDir, m.TargetDir)
		}
		fmt.Fprintln(w)
	}

	// Lifecycle scripts
	if len(plan.Scripts) > 0 {
		fmt.Fprintf(w, "Lifecycle scripts (%d):\n", len(plan.Scripts))
		for _, s := range plan.Scripts {
			// Show path with dot notation (post-migration name)
			displayPath := applyMappingToPath(s.RelPath, plan.Mappings)
			fmt.Fprintf(w, "  %s\n", displayPath)

			details := []string{s.Phase}
			if s.PackageManager != "" {
				details = append(details, "manager: "+s.PackageManager)
			}
			if len(s.PackageNames) > 0 {
				if len(s.PackageNames) <= 3 {
					details = append(details, "packages: ["+strings.Join(s.PackageNames, ", ")+"]")
				} else {
					details = append(details, fmt.Sprintf("packages: [%s, ...] (%d total)",
						strings.Join(s.PackageNames[:3], ", "), len(s.PackageNames)))
				}
			}
			details = append(details, fmt.Sprintf("%d lines", s.LineCount))
			fmt.Fprintf(w, "    %s\n", strings.Join(details, " | "))

			for _, obs := range s.Observations {
				fmt.Fprintf(w, "    %s\n", obs)
			}
		}
		fmt.Fprintln(w)
	}

	// Observations
	if len(plan.Observations) > 0 {
		fmt.Fprintf(w, "Observations:\n")
		for _, obs := range plan.Observations {
			fmt.Fprintf(w, "  - %s\n", obs)
		}
		fmt.Fprintln(w)
	}

	// Warnings
	if len(plan.Warnings) > 0 {
		fmt.Fprintf(w, "Warnings:\n")
		for _, warn := range plan.Warnings {
			fmt.Fprintf(w, "  - %s\n", warn)
		}
		fmt.Fprintln(w)
	}

	// TODOs
	projects := UniqueProjects(plan.Entries)
	fmt.Fprintf(w, "TODOs after migration:\n")
	fmt.Fprintf(w, "  1. Run: writ add %s\n", strings.Join(projects, " "))
	if plan.Stats.Secrets > 0 {
		fmt.Fprintf(w, "  2. Encrypt secrets: age -R recipients.txt -o file.age file\n")
	}
	if plan.Stats.LifecycleScripts > 0 {
		fmt.Fprintf(w, "  3. Evaluate Install-*/Initialize-* scripts for lore package conversion\n")
	}
	fmt.Fprintf(w, "  4. Consider packages.manifest for common tool installations\n")

	return nil
}

// applyMappingToPath replaces the first directory component if it matches
// a mapping source, showing the post-migration path.
func applyMappingToPath(relPath string, mappings []DirectoryMapping) string {
	parts := strings.SplitN(relPath, string('/'), 2)
	if len(parts) == 0 {
		return relPath
	}
	for _, m := range mappings {
		if parts[0] == m.SourceDir {
			if len(parts) == 2 {
				return m.TargetDir + "/" + parts[1]
			}
			return m.TargetDir
		}
	}
	return relPath
}
