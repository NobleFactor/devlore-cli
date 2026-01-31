// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package migrate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/internal/execution"
	"github.com/NobleFactor/devlore-cli/internal/model"
)

// FormatMigrationPlan renders the execution Graph and MigrationAnalysis as
// human-readable output. This is the derived view that replaces the legacy
// MigrationPlan struct.
//
// Supported formats: "text" (default), "yaml", "json"
// For "explain" format, use FormatMigrationExplain which requires an AI provider.
func FormatMigrationPlan(w io.Writer, graph *execution.Graph, analysis *MigrationAnalysis, format string) error {
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
	ID        string `json:"id" yaml:"id"`
	Type      string `json:"type" yaml:"type"`
	Source    string `json:"source,omitempty" yaml:"source,omitempty"`
	Target    string `json:"target,omitempty" yaml:"target,omitempty"`
	DependsOn string `json:"depends_on,omitempty" yaml:"depends_on,omitempty"`
}

func formatMigrationYAML(w io.Writer, graph *execution.Graph, analysis *MigrationAnalysis) error {
	view := buildMigrationView(graph, analysis)
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	return enc.Encode(view)
}

func formatMigrationJSON(w io.Writer, graph *execution.Graph, analysis *MigrationAnalysis) error {
	view := buildMigrationView(graph, analysis)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(view)
}

func buildMigrationView(graph *execution.Graph, analysis *MigrationAnalysis) *migrationView {
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

func formatMigrationText(w io.Writer, graph *execution.Graph, analysis *MigrationAnalysis) error {
	formatHeader(w, analysis)
	formatSummary(w, analysis.Stats)
	formatRenames(w, graph, analysis.SourceRoot)
	formatScripts(w, analysis.Scripts)
	formatStringList(w, "Observations", analysis.Observations)
	formatStringList(w, "Warnings", analysis.Warnings)
	formatSecrets(w, analysis.SecretFindings)
	formatRecommendations(w, analysis.Recommendations)
	return nil
}

func formatHeader(w io.Writer, analysis *MigrationAnalysis) {
	_, _ = fmt.Fprintf(w, "Migration Plan\n")
	_, _ = fmt.Fprintf(w, "Source: %s\n", analysis.SourceRoot)
	_, _ = fmt.Fprintf(w, "System: %s", analysis.System)
	if analysis.SystemConfidence > 0 {
		_, _ = fmt.Fprintf(w, " (confidence: %.0f%%)", analysis.SystemConfidence*100)
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w)
}

func formatSummary(w io.Writer, s MigrationStats) {
	_, _ = fmt.Fprintf(w, "Summary:\n")
	_, _ = fmt.Fprintf(w, "  Files: %d | Projects: %d | Platforms: %d\n",
		s.TotalFiles, s.Projects, s.Platforms)
	_, _ = fmt.Fprintf(w, "  Configs: %d | Scripts: %d | Lifecycle: %d\n",
		s.StaticConfigs, s.Scripts, s.LifecycleScripts)

	extras := collectExtraStats(s)
	if len(extras) > 0 {
		_, _ = fmt.Fprintf(w, "  %s\n", strings.Join(extras, " | "))
	}
	_, _ = fmt.Fprintln(w)
}

func collectExtraStats(s MigrationStats) []string {
	var extras []string
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
	return extras
}

func formatRenames(w io.Writer, graph *execution.Graph, sourceRoot string) {
	renameNodes := filterNodesByOp(graph, "rename")
	if len(renameNodes) == 0 {
		return
	}

	_, _ = fmt.Fprintf(w, "Directory renames (%d):\n", len(renameNodes))
	maxLen := 0
	for _, node := range renameNodes {
		if len(node.Source) > maxLen {
			maxLen = len(node.Source)
		}
	}
	for _, node := range renameNodes {
		source := shortenPath(node.Source, sourceRoot)
		target := shortenPath(node.Target, sourceRoot)
		_, _ = fmt.Fprintf(w, "  %-*s  →  %s\n", maxLen-len(sourceRoot), source, target)
	}
	_, _ = fmt.Fprintln(w)
}

func formatScripts(w io.Writer, scripts []ScriptAnalysis) {
	if len(scripts) == 0 {
		return
	}

	_, _ = fmt.Fprintf(w, "Lifecycle scripts (%d):\n", len(scripts))
	for _, script := range scripts {
		formatScript(w, script)
	}
	_, _ = fmt.Fprintln(w)
}

func formatScript(w io.Writer, script ScriptAnalysis) {
	_, _ = fmt.Fprintf(w, "  %s\n", script.RelPath)
	_, _ = fmt.Fprintf(w, "    %s | %d lines\n", script.Phase, script.LineCount)

	if len(script.Resolved) > 0 {
		var names []string
		for _, r := range script.Resolved {
			names = append(names, r.LorePackage)
		}
		_, _ = fmt.Fprintf(w, "    Lore packages: %s\n", strings.Join(names, ", "))
	}

	if len(script.Unresolved) > 0 {
		var installs []string
		for _, u := range script.Unresolved {
			installs = append(installs, fmt.Sprintf("%s:%s", u.Manager, u.Name))
		}
		_, _ = fmt.Fprintf(w, "    Unknown: %s\n", strings.Join(installs, ", "))
	}

	for _, obs := range script.Observations {
		_, _ = fmt.Fprintf(w, "    %s\n", obs)
	}
}

func formatStringList(w io.Writer, title string, items []string) {
	if len(items) == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "%s:\n", title)
	for _, item := range items {
		_, _ = fmt.Fprintf(w, "  - %s\n", item)
	}
	_, _ = fmt.Fprintln(w)
}

func formatSecrets(w io.Writer, secrets []SecretFinding) {
	if len(secrets) == 0 {
		return
	}

	_, _ = fmt.Fprintf(w, "Secrets detected (%d):\n", len(secrets))
	hasUnencrypted := false
	for _, secret := range secrets {
		formatSecret(w, secret)
		if secret.Encryption == EncryptNone {
			hasUnencrypted = true
		}
	}
	_, _ = fmt.Fprintln(w)

	if hasUnencrypted {
		formatSOPSRecommendation(w, secrets)
	}
}

func formatSecret(w io.Writer, secret SecretFinding) {
	icon := "🔓"
	encLabel := ""
	if secret.Encryption != EncryptNone {
		icon = "🔐"
		encLabel = fmt.Sprintf(" (%s)", secret.Encryption)
	}
	_, _ = fmt.Fprintf(w, "  %s %s%s\n", icon, secret.RelPath, encLabel)
	_, _ = fmt.Fprintf(w, "      %s\n", secret.Reason)
}

func formatRecommendations(w io.Writer, recommendations []string) {
	if len(recommendations) == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "TODOs after migration:\n")
	for i, rec := range recommendations {
		_, _ = fmt.Fprintf(w, "  %d. %s\n", i+1, rec)
	}
}

// filterNodesByOp returns nodes that have the specified operation.
func filterNodesByOp(graph *execution.Graph, opName string) []*execution.Node {
	var nodes []*execution.Node
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
	_, _ = fmt.Fprintf(w, "SOPS Setup Recommendation:\n")
	_, _ = fmt.Fprintf(w, "  1. Install SOPS: brew install sops  # or: go install github.com/getsops/sops/v3/cmd/sops@latest\n")
	_, _ = fmt.Fprintf(w, "  2. Create age key: age-keygen -o ~/.config/sops/age/keys.txt\n")
	_, _ = fmt.Fprintf(w, "  3. Create .sops.yaml with your public key:\n")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "     # .sops.yaml\n")
	_, _ = fmt.Fprintf(w, "     creation_rules:\n")

	// Collect unique patterns
	patterns := make(map[string]bool)
	for _, s := range secrets {
		if s.Encryption == EncryptNone && s.SuggestedPattern != "" {
			patterns[s.SuggestedPattern] = true
		}
	}

	for pattern := range patterns {
		_, _ = fmt.Fprintf(w, "       - path_regex: %s\n", pattern)
		_, _ = fmt.Fprintf(w, "         age: \"<your-age-public-key>\"\n")
	}

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "  4. Encrypt each secret: sops encrypt --in-place <file>\n")
	_, _ = fmt.Fprintf(w, "  5. Commit .sops.yaml and encrypted files\n")
	_, _ = fmt.Fprintln(w)
}

// FormatMigrationExplain uses AI to generate a natural language explanation
// of the migration analysis. This provides a conversational summary that
// highlights key findings and actionable recommendations.
func FormatMigrationExplain(ctx context.Context, w io.Writer, analysis *MigrationAnalysis, provider model.Provider) error {
	if provider == nil {
		return fmt.Errorf("AI provider required for explain format")
	}

	// Serialize analysis to JSON for the AI
	analysisJSON, err := json.MarshalIndent(analysis, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal analysis: %w", err)
	}

	prompt := `You are a helpful assistant explaining a dotfiles migration analysis.
Given the structured analysis below, provide a clear, conversational summary that:
1. Describes what kind of repository this is and its structure
2. Highlights key findings (projects, platforms, scripts, secrets)
3. Points out any concerns or warnings
4. Summarizes recommended next steps

Be concise but informative. Use a friendly, helpful tone. Format with markdown.
Do not repeat the raw data - synthesize and explain it.`

	userMessage := fmt.Sprintf("Please explain this migration analysis:\n\n```json\n%s\n```", string(analysisJSON))

	resp, err := provider.Chat(ctx, model.ChatRequest{
		SystemPrompt: prompt,
		Messages: []model.Message{
			{Role: model.RoleUser, Content: userMessage},
		},
		Temperature: 0.3,
	})
	if err != nil {
		return fmt.Errorf("AI explanation failed: %w", err)
	}

	_, _ = fmt.Fprintln(w, resp.Content)
	return nil
}
