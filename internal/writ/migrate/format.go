// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/internal/engine"
)

// FormatMigrationPlan renders the execution Graph and MigrationAnalysis as
// human-readable output. This is the derived view that replaces the legacy
// MigrationPlan struct.
//
// Supported formats: "text" (default), "yaml", "json"
func FormatMigrationPlan(w io.Writer, graph *engine.Graph, analysis *MigrationAnalysis, format string) error {
	switch format {
	case "yaml":
		return formatMigrationYAML(w, graph, analysis)
	case "json":
		return formatMigrationJSON(w, graph, analysis)
	default:
		return formatMigrationText(w, graph, analysis)
	}
}

// migrationView is the combined view for YAML/JSON serialization.
type migrationView struct {
	Analysis   *MigrationAnalysis `json:"analysis" yaml:"analysis"`
	Operations []operationView    `json:"operations" yaml:"operations"`
}

// operationView represents a single operation for serialization.
type operationView struct {
	ID         string `json:"id" yaml:"id"`
	Type       string `json:"type" yaml:"type"`
	Source     string `json:"source,omitempty" yaml:"source,omitempty"`
	Target     string `json:"target,omitempty" yaml:"target,omitempty"`
	DependsOn  string `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
}

func formatMigrationYAML(w io.Writer, graph *engine.Graph, analysis *MigrationAnalysis) error {
	view := buildMigrationView(graph, analysis)
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	return enc.Encode(view)
}

func formatMigrationJSON(w io.Writer, graph *engine.Graph, analysis *MigrationAnalysis) error {
	view := buildMigrationView(graph, analysis)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(view)
}

func buildMigrationView(graph *engine.Graph, analysis *MigrationAnalysis) *migrationView {
	// Build dependency map from edges
	dependsOn := make(map[string]string)
	for _, edge := range graph.Edges {
		if edge.Relation == "depends_on" {
			dependsOn[edge.To] = edge.From
		}
	}

	var ops []operationView
	for _, node := range graph.Nodes {
		opType := "unknown"
		if len(node.Operations) > 0 {
			opType = node.Operations[0]
		}
		ops = append(ops, operationView{
			ID:        node.ID,
			Type:      opType,
			Source:    node.Source,
			Target:    node.Target,
			DependsOn: dependsOn[node.ID],
		})
	}

	return &migrationView{
		Analysis:   analysis,
		Operations: ops,
	}
}

func formatMigrationText(w io.Writer, graph *engine.Graph, analysis *MigrationAnalysis) error {
	fmt.Fprintf(w, "Migration Plan\n")
	fmt.Fprintf(w, "Source: %s\n", analysis.SourceRoot)
	fmt.Fprintf(w, "System: %s", analysis.System)
	if analysis.SystemConfidence > 0 {
		fmt.Fprintf(w, " (confidence: %.0f%%)", analysis.SystemConfidence*100)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w)

	// Summary
	s := analysis.Stats
	fmt.Fprintf(w, "Summary:\n")
	fmt.Fprintf(w, "  Files: %d | Projects: %d | Platforms: %d\n",
		s.TotalFiles, s.Projects, s.Platforms)
	fmt.Fprintf(w, "  Configs: %d | Scripts: %d | Lifecycle: %d\n",
		s.StaticConfigs, s.Scripts, s.LifecycleScripts)

	extras := []string{}
	if s.Secrets > 0 {
		extras = append(extras, fmt.Sprintf("Secrets: %d", s.Secrets))
	}
	if s.Fonts > 0 {
		extras = append(extras, fmt.Sprintf("Fonts: %d", s.Fonts))
	}
	if s.Completions > 0 {
		extras = append(extras, fmt.Sprintf("Completions: %d", s.Completions))
	}
	if s.Templates > 0 {
		extras = append(extras, fmt.Sprintf("Templates: %d", s.Templates))
	}
	if len(extras) > 0 {
		fmt.Fprintf(w, "  %s\n", strings.Join(extras, " | "))
	}
	fmt.Fprintln(w)

	// Operations from Graph (directory renames)
	renameNodes := filterNodesByOp(graph, "rename")
	if len(renameNodes) > 0 {
		fmt.Fprintf(w, "Directory renames (%d):\n", len(renameNodes))
		maxLen := 0
		for _, node := range renameNodes {
			if len(node.Source) > maxLen {
				maxLen = len(node.Source)
			}
		}
		for _, node := range renameNodes {
			// Show relative paths for readability
			source := shortenPath(node.Source, analysis.SourceRoot)
			target := shortenPath(node.Target, analysis.SourceRoot)
			fmt.Fprintf(w, "  %-*s  →  %s\n", maxLen-len(analysis.SourceRoot), source, target)
		}
		fmt.Fprintln(w)
	}

	// Lifecycle scripts from Analysis
	if len(analysis.Scripts) > 0 {
		fmt.Fprintf(w, "Lifecycle scripts (%d):\n", len(analysis.Scripts))
		for _, script := range analysis.Scripts {
			fmt.Fprintf(w, "  %s\n", script.RelPath)

			details := []string{script.Phase}
			if script.PackageManager != "" {
				details = append(details, "manager: "+script.PackageManager)
			}
			if len(script.PackageNames) > 0 {
				if len(script.PackageNames) <= 3 {
					details = append(details, "packages: ["+strings.Join(script.PackageNames, ", ")+"]")
				} else {
					details = append(details, fmt.Sprintf("packages: [%s, ...] (%d total)",
						strings.Join(script.PackageNames[:3], ", "), len(script.PackageNames)))
				}
			}
			details = append(details, fmt.Sprintf("%d lines", script.LineCount))
			fmt.Fprintf(w, "    %s\n", strings.Join(details, " | "))

			for _, obs := range script.Observations {
				fmt.Fprintf(w, "    %s\n", obs)
			}
		}
		fmt.Fprintln(w)
	}

	// Observations
	if len(analysis.Observations) > 0 {
		fmt.Fprintf(w, "Observations:\n")
		for _, obs := range analysis.Observations {
			fmt.Fprintf(w, "  - %s\n", obs)
		}
		fmt.Fprintln(w)
	}

	// Warnings
	if len(analysis.Warnings) > 0 {
		fmt.Fprintf(w, "Warnings:\n")
		for _, warn := range analysis.Warnings {
			fmt.Fprintf(w, "  - %s\n", warn)
		}
		fmt.Fprintln(w)
	}

	// Secrets
	if len(analysis.SecretFindings) > 0 {
		fmt.Fprintf(w, "Secrets detected (%d):\n", len(analysis.SecretFindings))
		for _, secret := range analysis.SecretFindings {
			icon := "🔓" // unlocked
			if secret.Encryption != EncryptNone {
				icon = "🔐" // locked
			}
			encLabel := ""
			if secret.Encryption != EncryptNone {
				encLabel = fmt.Sprintf(" (%s)", secret.Encryption)
			}
			fmt.Fprintf(w, "  %s %s%s\n", icon, secret.RelPath, encLabel)
			fmt.Fprintf(w, "      %s\n", secret.Reason)
		}
		fmt.Fprintln(w)

		// SOPS recommendation if unencrypted secrets exist
		hasUnencrypted := false
		for _, secret := range analysis.SecretFindings {
			if secret.Encryption == EncryptNone {
				hasUnencrypted = true
				break
			}
		}
		if hasUnencrypted {
			formatSOPSRecommendation(w, analysis.SecretFindings)
		}
	}

	// Recommendations (TODOs)
	if len(analysis.Recommendations) > 0 {
		fmt.Fprintf(w, "TODOs after migration:\n")
		for i, rec := range analysis.Recommendations {
			fmt.Fprintf(w, "  %d. %s\n", i+1, rec)
		}
	}

	return nil
}

// filterNodesByOp returns nodes that have the specified operation.
func filterNodesByOp(graph *engine.Graph, opName string) []*engine.Node {
	var nodes []*engine.Node
	for _, node := range graph.Nodes {
		for _, op := range node.Operations {
			if op == opName {
				nodes = append(nodes, node)
				break
			}
		}
	}
	return nodes
}

// shortenPath removes the prefix from a path for display.
func shortenPath(path, prefix string) string {
	if strings.HasPrefix(path, prefix) {
		result := strings.TrimPrefix(path, prefix)
		result = strings.TrimPrefix(result, "/")
		if result == "" {
			return "."
		}
		return result
	}
	return path
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
