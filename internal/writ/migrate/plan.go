// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/model"
	"github.com/NobleFactor/devlore-cli/internal/lorepackage"
)

// Options controls migration behavior.
type Options struct {
	SourceRoot string
	TargetRoot string // empty = rename in place
	Execute    bool
	Verbose    bool
	Format     string // "json" (default), "yaml", "text"
	Provider   model.Provider
	RegClient  *lorepackage.Registry
}

// BuildMigration performs detection, inventory, and analysis, returning an
// execution Graph and MigrationAnalysis. This is the primary API that separates
// executable operations from non-executable understanding.
//
// The Graph contains rename operations for directory structure changes.
// The Analysis contains observations, warnings, and recommendations.
func BuildMigration(ctx context.Context, opts Options) (*execution.Graph, *MigrationAnalysis, error) {
	root := opts.SourceRoot

	// Detect source system (signature-based if registry available, fallback to heuristics)
	system, confidence, err := detectSourceSystem(root, opts.RegClient)
	if err != nil {
		return nil, nil, fmt.Errorf("detection failed: %w", err)
	}
	if system == SystemUnknown {
		return nil, nil, fmt.Errorf("could not detect source system in %s; specify with --system", root)
	}

	// Check for prior migration
	if exists(root + "/.writ-migrated") {
		return nil, nil, fmt.Errorf("already migrated (found .writ-migrated); remove it to re-run")
	}

	// Inventory
	entries, err := Inventory(root)
	if err != nil {
		return nil, nil, fmt.Errorf("inventory failed: %w", err)
	}

	// Build mappings (structural, no AI needed)
	mappings, err := BuildMappings(root)
	if err != nil {
		return nil, nil, fmt.Errorf("mapping failed: %w", err)
	}

	// Detect encryption systems (structural)
	encSystems := DetectEncryption(root)

	// Load signature index for package resolution (from registry if available)
	var sigIdx SignatureIndex
	if opts.RegClient != nil {
		sigIdx = opts.RegClient.SignatureIndex()
	}
	if sigIdx == nil {
		sigIdx = make(SignatureIndex)
	}

	// Build execution graph
	graph := BuildMigrationGraph(root, mappings)

	// Build analysis (with AI enhancement if available)
	analysis := BuildMigrationAnalysis(root, system, confidence, entries, mappings, encSystems, sigIdx)

	// Enhance analysis with AI if provider is available
	if opts.Provider != nil && opts.RegClient != nil {
		enhanceAnalysisWithAI(ctx, opts, analysis, entries)
	}

	return graph, analysis, nil
}

// detectSourceSystem detects the source system, using signature-based detection
// if a registry client is available, falling back to heuristics otherwise.
func detectSourceSystem(root string, regClient *lorepackage.Registry) (SourceSystem, float64, error) {
	// Try signature-based detection if registry is available
	if regClient != nil {
		signatures, err := LoadSignatures(regClient)
		if err == nil && len(signatures) > 0 {
			results := DetectWithSignatures(root, signatures)
			if len(results) > 0 && results[0].Confidence > 0.5 {
				return results[0].System, results[0].Confidence, nil
			}
		}
	}

	// Fall back to heuristic detection
	system, err := Detect(root)
	return system, 0, err
}

// enhanceAnalysisWithAI uses AI to improve the analysis with better
// classifications, secret detection, and observations.
func enhanceAnalysisWithAI(ctx context.Context, opts Options, analysis *MigrationAnalysis, entries []InventoryEntry) {
	knowledge := opts.RegClient.Knowledge("migration")

	// Load index for discovery
	index, err := knowledge.Index()
	if err != nil {
		if opts.Verbose {
			cli.Error("migration index load failed: %v", err)
		}
		return
	}

	// Discover and load prompt by purpose
	promptName := index.PromptByPurpose("writ-migration")
	if promptName == "" {
		if opts.Verbose {
			cli.Error("no prompt with purpose 'writ-migration' in migration index")
		}
		return
	}
	prompt, err := knowledge.Prompt(promptName)
	if err != nil {
		if opts.Verbose {
			cli.Error("AI prompt load failed: %v", err)
		}
		return
	}

	// Discover and load transform by source system
	var guide []byte
	transformName := index.TransformBySourceSystem(string(analysis.System))
	if transformName != "" {
		guide, _ = knowledge.Transform(transformName)
	}

	// Build summarized inventory for AI
	fileList := buildAIInventory(entries)

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
		analysis.System, string(guide), fileList)

	resp, err := opts.Provider.Chat(ctx, model.ChatRequest{
		SystemPrompt: prompt,
		Messages: []model.Message{
			{Role: model.RoleUser, Content: userMessage},
		},
		Temperature: 0,
		JSONMode:    true,
	})
	if err != nil {
		if opts.Verbose {
			cli.Error("AI chat failed: %v", err)
		}
		return
	}

	// Parse and merge AI response into analysis
	mergeAIResponseIntoAnalysis(resp.Content, analysis)
}

// mergeAIResponseIntoAnalysis parses AI response and updates analysis fields.
func mergeAIResponseIntoAnalysis(content string, analysis *MigrationAnalysis) {
	var response struct {
		RepoLayer    string          `json:"repo_layer"`
		Secrets      json.RawMessage `json:"secrets"`
		Observations json.RawMessage `json:"observations"`
		Warnings     json.RawMessage `json:"warnings"`
	}

	if err := json.Unmarshal([]byte(content), &response); err != nil {
		return
	}

	// Update repo layer
	switch response.RepoLayer {
	case "base":
		analysis.RepoLayer = LayerBase
	case "team":
		analysis.RepoLayer = LayerTeam
	case "personal":
		analysis.RepoLayer = LayerPersonal
	}

	// Merge observations
	aiObs := parseFlexibleStrings(response.Observations)
	analysis.Observations = append(analysis.Observations, aiObs...)

	// Merge warnings
	aiWarnings := parseFlexibleStrings(response.Warnings)
	analysis.Warnings = append(analysis.Warnings, aiWarnings...)

	// Merge secret findings
	aiSecrets := parseAISecretFindings(response.Secrets)
	analysis.SecretFindings = append(analysis.SecretFindings, aiSecrets...)
}

// parseAISecretFindings parses AI secret detection response.
func parseAISecretFindings(raw json.RawMessage) []SecretFinding {
	if len(raw) == 0 {
		return nil
	}

	var arr []map[string]interface{}
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil
	}

	var findings []SecretFinding
	for _, obj := range arr {
		path := extractString(obj, "path", "file", "file_path", "filepath", "name")
		reason := extractString(obj, "reason", "description", "message", "why", "signal")

		if path == "" {
			continue
		}

		findings = append(findings, SecretFinding{
			RelPath:    path,
			Encryption: EncryptNone,
			Reason:     reason,
		})
	}
	return findings
}

// extractString tries multiple keys to extract a string value from a map.
func extractString(obj map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if v, ok := obj[key]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

// parseFlexibleStrings parses a JSON field that could be []string or []object.
// If objects, it tries to extract a "text", "message", or "description" field.
func parseFlexibleStrings(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}

	// First try as []string
	var strs []string
	if err := json.Unmarshal(raw, &strs); err == nil {
		return strs
	}

	// Try as []object with text fields
	var objects []map[string]interface{}
	if err := json.Unmarshal(raw, &objects); err == nil {
		var result []string
		for _, obj := range objects {
			// Try common text field names
			for _, key := range []string{"text", "message", "description", "content", "summary"} {
				if v, ok := obj[key]; ok {
					if s, ok := v.(string); ok && s != "" {
						result = append(result, s)
						break
					}
				}
			}
		}
		return result
	}

	return nil
}

// buildAIInventory creates a summarized file inventory for AI analysis.
// Instead of listing every file (which can exceed token limits), it provides:
// - Directory structure overview
// - Executable/lifecycle scripts (explicitly listed)
// - Potential secret paths (explicitly listed)
// - File count summary by extension
func buildAIInventory(entries []InventoryEntry) string {
	var sb strings.Builder

	// Group by top-level directory
	dirFiles := make(map[string]int)
	var executables []string
	var potentialSecrets []string

	for _, e := range entries {
		// Count by top-level dir
		parts := strings.SplitN(e.RelPath, "/", 2)
		if len(parts) > 0 {
			dirFiles[parts[0]]++
		}

		// Track executables (lifecycle scripts)
		if e.IsExecutable {
			executables = append(executables, e.RelPath)
		}

		// Track potential secrets (by path patterns)
		lowerPath := strings.ToLower(e.RelPath)
		if strings.Contains(lowerPath, "secret") ||
			strings.Contains(lowerPath, "private") ||
			strings.Contains(lowerPath, ".ssh/") ||
			strings.Contains(lowerPath, ".gnupg/") ||
			strings.Contains(lowerPath, "credential") ||
			strings.Contains(lowerPath, "token") ||
			strings.HasSuffix(lowerPath, ".key") ||
			strings.HasSuffix(lowerPath, ".pem") {
			potentialSecrets = append(potentialSecrets, e.RelPath)
		}
	}

	// Structure overview
	sb.WriteString("Directory structure:\n")
	for dir, count := range dirFiles {
		sb.WriteString(fmt.Sprintf("  %s/ (%d files)\n", dir, count))
	}

	// Executables (important for lifecycle script detection)
	if len(executables) > 0 {
		sb.WriteString("\nExecutable scripts:\n")
		for _, path := range executables {
			sb.WriteString(fmt.Sprintf("  %s\n", path))
		}
	}

	// Potential secrets (important for security analysis)
	if len(potentialSecrets) > 0 {
		sb.WriteString("\nPotential secret paths:\n")
		for _, path := range potentialSecrets {
			sb.WriteString(fmt.Sprintf("  %s\n", path))
		}
	}

	// File type summary
	sb.WriteString(fmt.Sprintf("\nTotal files: %d\n", len(entries)))

	return sb.String()
}
