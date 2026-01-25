// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package migrate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/internal/ai"
	"github.com/NobleFactor/devlore-cli/internal/registry"
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
	SourceRoot        string             `json:"source_root" yaml:"source_root"`
	System            SourceSystem       `json:"system" yaml:"system"`
	EncryptionSystems []EncryptionSystem `json:"encryption_systems" yaml:"encryption_systems"`
	Entries           []InventoryEntry   `json:"entries" yaml:"entries"`
	Mappings          []DirectoryMapping `json:"mappings" yaml:"mappings"`
	Scripts           []ScriptAnalysis   `json:"scripts" yaml:"scripts"`
	SecretFindings    []SecretFinding    `json:"secret_findings" yaml:"secret_findings"`
	Stats             Stats              `json:"stats" yaml:"stats"`
	Observations      []string           `json:"observations" yaml:"observations"`
	Warnings          []string           `json:"warnings" yaml:"warnings"`
}

// Options controls migration behavior.
type Options struct {
	SourceRoot string
	TargetRoot string // empty = rename in place
	Execute    bool
	Verbose    bool
	Format     string // "text", "yaml", "json"
	AIProvider ai.Provider
	RegClient  *registry.Client
}

// BuildPlan performs detection, inventory, classification, analysis, and
// assembles a complete migration plan.
func BuildPlan(ctx context.Context, opts Options) (*MigrationPlan, error) {
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

	// Build mappings (structural, no AI needed)
	mappings, err := BuildMappings(root)
	if err != nil {
		return nil, fmt.Errorf("mapping failed: %w", err)
	}

	// Detect encryption systems (structural)
	encSystems := DetectEncryption(root)

	// Use AI-assisted analysis if available
	if opts.AIProvider != nil && opts.RegClient != nil {
		return buildPlanWithAI(ctx, opts, root, system, entries, mappings, encSystems)
	}

	// Fall back to basic structural analysis
	return buildPlanBasic(root, system, entries, mappings, encSystems)
}

// buildPlanWithAI uses AI for classification, secret detection, and recommendations.
func buildPlanWithAI(ctx context.Context, opts Options, root string, system SourceSystem, entries []InventoryEntry, mappings []DirectoryMapping, encSystems []EncryptionSystem) (*MigrationPlan, error) {
	// Load migration guide for this source system
	guide, err := opts.RegClient.MigrationGuide(string(system))
	if err != nil {
		// Fall back to basic if no guide
		return buildPlanBasic(root, system, entries, mappings, encSystems)
	}

	// Load AI prompt
	prompt, err := opts.RegClient.AIPrompt("migrate-to-writ.txt")
	if err != nil {
		return buildPlanBasic(root, system, entries, mappings, encSystems)
	}

	// Build file inventory for AI
	var fileList strings.Builder
	for _, e := range entries {
		fileList.WriteString(e.RelPath)
		if e.IsExecutable {
			fileList.WriteString(" [executable]")
		}
		fileList.WriteString("\n")
	}

	// Call AI for analysis
	userMessage := fmt.Sprintf(`Source system: %s

Migration guide:
%s

File inventory:
%s

Analyze this environment repository and provide:
1. Classification of each file (config, script, secret, etc.)
2. Detection of sensitive files needing encryption
3. Observations about the structure
4. Warnings about potential issues
5. Post-migration recommendations

Respond in JSON format per the migration-plan schema.`,
		system, string(guide), fileList.String())

	resp, err := opts.AIProvider.Chat(ctx, ai.ChatRequest{
		SystemPrompt: prompt,
		Messages: []ai.Message{
			{Role: ai.RoleUser, Content: userMessage},
		},
		Temperature: 0, // Deterministic
		JSONMode:    true,
	})
	if err != nil {
		// AI failed, fall back to basic
		return buildPlanBasic(root, system, entries, mappings, encSystems)
	}

	// Parse AI response
	aiPlan, err := parseAIResponse(resp.Content, entries)
	if err != nil {
		// Parse failed, fall back to basic
		return buildPlanBasic(root, system, entries, mappings, encSystems)
	}

	// Merge AI analysis with structural data
	return &MigrationPlan{
		SourceRoot:        root,
		System:            system,
		EncryptionSystems: encSystems,
		Entries:           aiPlan.Entries,
		Mappings:          mappings,
		Scripts:           aiPlan.Scripts,
		SecretFindings:    aiPlan.SecretFindings,
		Stats:             computeStats(aiPlan.Entries, mappings),
		Observations:      aiPlan.Observations,
		Warnings:          aiPlan.Warnings,
	}, nil
}

// buildPlanBasic uses structural analysis only (no AI).
func buildPlanBasic(root string, system SourceSystem, entries []InventoryEntry, mappings []DirectoryMapping, encSystems []EncryptionSystem) (*MigrationPlan, error) {
	// Basic classification based on file attributes
	Classify(entries)

	// Analyze lifecycle scripts (pattern-based)
	scripts := AnalyzeScripts(entries)

	// Basic secret detection (encrypted files only, no content scanning)
	secretFindings := detectEncryptedSecrets(entries)

	// Compute stats
	stats := computeStats(entries, mappings)

	// Basic observations
	observations := []string{
		fmt.Sprintf("Detected source system: %s", system),
	}
	if len(mappings) > 0 {
		observations = append(observations, fmt.Sprintf("%d directories will be renamed", len(mappings)))
	}
	observations = append(observations, "Run with AI enabled for detailed analysis: remove --no-ai flag")

	// Basic warnings
	var warnings []string
	for _, enc := range encSystems {
		if enc != EncryptNone && enc != EncryptSOPS {
			warnings = append(warnings, fmt.Sprintf("%s detected — writ uses SOPS for secrets", enc))
		}
	}

	return &MigrationPlan{
		SourceRoot:        root,
		System:            system,
		EncryptionSystems: encSystems,
		Entries:           entries,
		Mappings:          mappings,
		Scripts:           scripts,
		SecretFindings:    secretFindings,
		Stats:             stats,
		Observations:      observations,
		Warnings:          warnings,
	}, nil
}

// detectEncryptedSecrets finds files with encryption signatures (no content scanning).
func detectEncryptedSecrets(entries []InventoryEntry) []SecretFinding {
	var findings []SecretFinding
	for _, e := range entries {
		enc := DetectEncryptedFile(e.AbsPath)
		if enc != EncryptNone {
			findings = append(findings, SecretFinding{
				RelPath:    e.RelPath,
				Encryption: enc,
				Reason:     fmt.Sprintf("Encrypted with %s", enc),
			})
		}
	}
	return findings
}

// aiAnalysisResult holds parsed AI response.
type aiAnalysisResult struct {
	Entries        []InventoryEntry
	Scripts        []ScriptAnalysis
	SecretFindings []SecretFinding
	Observations   []string
	Warnings       []string
}

// parseAIResponse parses the AI JSON response and updates entries.
func parseAIResponse(content string, originalEntries []InventoryEntry) (*aiAnalysisResult, error) {
	// Try to parse as JSON
	var response struct {
		Classifications []struct {
			Path  string `json:"path"`
			Class string `json:"class"`
		} `json:"classifications"`
		Secrets []struct {
			Path   string `json:"path"`
			Reason string `json:"reason"`
		} `json:"secrets"`
		Scripts []struct {
			Path           string   `json:"path"`
			Phase          string   `json:"phase"`
			PackageManager string   `json:"package_manager"`
			Packages       []string `json:"packages"`
		} `json:"scripts"`
		Observations []string `json:"observations"`
		Warnings     []string `json:"warnings"`
	}

	if err := json.Unmarshal([]byte(content), &response); err != nil {
		return nil, err
	}

	// Build path -> classification map
	classMap := make(map[string]FileClass)
	for _, c := range response.Classifications {
		classMap[c.Path] = FileClass(c.Class)
	}

	// Update entries with AI classifications
	entries := make([]InventoryEntry, len(originalEntries))
	copy(entries, originalEntries)
	for i := range entries {
		if class, ok := classMap[entries[i].RelPath]; ok {
			entries[i].Class = class
		}
	}

	// Build secret findings
	var secretFindings []SecretFinding
	for _, s := range response.Secrets {
		secretFindings = append(secretFindings, SecretFinding{
			RelPath:    s.Path,
			Encryption: EncryptNone,
			Reason:     s.Reason,
		})
	}

	// Build script analyses
	var scripts []ScriptAnalysis
	for _, s := range response.Scripts {
		scripts = append(scripts, ScriptAnalysis{
			RelPath:        s.Path,
			Phase:          s.Phase,
			PackageManager: s.PackageManager,
			PackageNames:   s.Packages,
		})
	}

	return &aiAnalysisResult{
		Entries:        entries,
		Scripts:        scripts,
		SecretFindings: secretFindings,
		Observations:   response.Observations,
		Warnings:       response.Warnings,
	}, nil
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

func generateObservations(entries []InventoryEntry, mappings []DirectoryMapping, scripts []ScriptAnalysis, secrets []SecretFinding, encSystems []EncryptionSystem) []string {
	var obs []string

	// Check for tuckr install scripts (obsolete after migration)
	for _, s := range scripts {
		if strings.Contains(strings.ToLower(s.Name), "tuckr") {
			obs = append(obs, s.Name+" can be removed post-migration (writ replaces tuckr)")
			break
		}
	}

	// Encryption system observations
	for _, enc := range encSystems {
		switch enc {
		case EncryptSOPS:
			obs = append(obs, "SOPS already configured (.sops.yaml found) — writ will use it")
		case EncryptGitCrypt:
			obs = append(obs, "git-crypt detected — consider migrating to SOPS for unified config")
		case EncryptBlackbox:
			obs = append(obs, "Blackbox detected — consider migrating to SOPS")
		case EncryptTranscrypt:
			obs = append(obs, "transcrypt detected — consider migrating to SOPS")
		}
	}

	// Secret findings summary
	encrypted := 0
	unencrypted := 0
	for _, s := range secrets {
		if s.Encryption != EncryptNone {
			encrypted++
		} else {
			unencrypted++
		}
	}
	if encrypted > 0 {
		obs = append(obs, fmt.Sprintf("%d encrypted secret files found", encrypted))
	}
	if unencrypted > 0 {
		obs = append(obs, fmt.Sprintf("%d unencrypted sensitive files detected — encrypt with SOPS", unencrypted))
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

func generateWarnings(entries []InventoryEntry, scripts []ScriptAnalysis, secrets []SecretFinding, encSystems []EncryptionSystem) []string {
	var warnings []string

	// Check for unencrypted secrets
	unencryptedSecrets := 0
	for _, s := range secrets {
		if s.Encryption == EncryptNone {
			unencryptedSecrets++
		}
	}
	if unencryptedSecrets > 0 {
		warnings = append(warnings, fmt.Sprintf("%d sensitive files are unencrypted — encrypt with SOPS before committing", unencryptedSecrets))
	}

	// Non-SOPS encryption systems
	for _, enc := range encSystems {
		switch enc {
		case EncryptGitCrypt:
			warnings = append(warnings, "git-crypt is in use — writ requires SOPS for secret management")
		case EncryptBlackbox:
			warnings = append(warnings, "Blackbox is in use — writ requires SOPS for secret management")
		case EncryptTranscrypt:
			warnings = append(warnings, "transcrypt is in use — writ requires SOPS for secret management")
		}
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

	// Secrets section
	if len(plan.SecretFindings) > 0 {
		fmt.Fprintf(w, "Secrets detected (%d):\n", len(plan.SecretFindings))
		for _, s := range plan.SecretFindings {
			icon := "🔓" // unlocked
			if s.Encryption != EncryptNone {
				icon = "🔐" // locked
			}
			encLabel := ""
			if s.Encryption != EncryptNone {
				encLabel = fmt.Sprintf(" (%s)", s.Encryption)
			}
			fmt.Fprintf(w, "  %s %s%s\n", icon, s.RelPath, encLabel)
			fmt.Fprintf(w, "      %s\n", s.Reason)
		}
		fmt.Fprintln(w)

		// Generate .sops.yaml suggestion if there are unencrypted secrets
		hasUnencrypted := false
		for _, s := range plan.SecretFindings {
			if s.Encryption == EncryptNone {
				hasUnencrypted = true
				break
			}
		}
		if hasUnencrypted {
			formatSOPSRecommendation(w, plan.SecretFindings)
		}
	}

	// TODOs
	projects := UniqueProjects(plan.Entries)
	fmt.Fprintf(w, "TODOs after migration:\n")
	todoNum := 1
	fmt.Fprintf(w, "  %d. Run: writ add %s\n", todoNum, strings.Join(projects, " "))
	todoNum++

	// Check for unencrypted secrets
	hasUnencryptedSecrets := false
	for _, s := range plan.SecretFindings {
		if s.Encryption == EncryptNone {
			hasUnencryptedSecrets = true
			break
		}
	}
	if hasUnencryptedSecrets {
		fmt.Fprintf(w, "  %d. Create .sops.yaml and encrypt secrets (see above)\n", todoNum)
		todoNum++
	}

	if plan.Stats.LifecycleScripts > 0 {
		fmt.Fprintf(w, "  %d. Evaluate Install-*/Initialize-* scripts for lore package conversion\n", todoNum)
		todoNum++
	}
	fmt.Fprintf(w, "  %d. Consider packages.manifest for common tool installations\n", todoNum)

	return nil
}

// formatSOPSRecommendation outputs a suggested .sops.yaml configuration.
func formatSOPSRecommendation(w io.Writer, secrets []SecretFinding) {
	fmt.Fprintf(w, "SOPS Setup Recommendation:\n")
	fmt.Fprintf(w, "  1. Install SOPS: brew install sops  # or: go install github.com/getsops/sops/v3/cmd/sops@latest\n")
	fmt.Fprintf(w, "  2. Create age key: age-keygen -o ~/.config/sops/age/keys.txt\n")
	fmt.Fprintf(w, "  3. Create .sops.yaml with your public key:\n")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "     # .sops.yaml\n")
	fmt.Fprintf(w, "     creation_rules:\n")

	// Collect unique patterns
	patterns := make(map[string]bool)
	for _, s := range secrets {
		if s.Encryption == EncryptNone && s.SuggestedPattern != "" {
			patterns[s.SuggestedPattern] = true
		}
	}

	for pattern := range patterns {
		fmt.Fprintf(w, "       - path_regex: %s\n", pattern)
		fmt.Fprintf(w, "         age: \"<your-age-public-key>\"\n")
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "  4. Encrypt each secret: sops encrypt --in-place <file>\n")
	fmt.Fprintf(w, "  5. Commit .sops.yaml and encrypted files\n")
	fmt.Fprintln(w)
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
