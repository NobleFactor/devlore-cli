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

	"github.com/NobleFactor/devlore-cli/internal/model"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// FormatMigrationPlan renders the execution Graph and MigrationAnalysis as
// human-readable output.
//
// Supported formats: "text" (default), "yaml", "json"
// For "explain" format, use FormatMigrationExplain which requires an AI provider.
func FormatMigrationPlan(w io.Writer, graph *op.Graph, analysis *MigrationAnalysis, format string) error {
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
// Outputs both analysis and execution_graph at the top level as per the plan.
type migrationView struct {
	Analysis       *MigrationAnalysis  `json:"analysis" yaml:"analysis"`
	ExecutionGraph *executionGraphView `json:"execution_graph" yaml:"execution_graph"`
}

// executionGraphView represents the execution graph for serialization.
type executionGraphView struct {
	Version string       `json:"version" yaml:"version"`
	Tool    string       `json:"tool" yaml:"tool"`
	State   string       `json:"state" yaml:"state"`
	Context graphContext `json:"context,omitempty" yaml:"context,omitempty"`
	Nodes   []nodeView   `json:"nodes" yaml:"nodes"`
	Edges   []edgeView   `json:"edges" yaml:"edges"`
}

type graphContext struct {
	SourceRoot string `json:"source_root,omitempty" yaml:"source_root,omitempty"`
}

// nodeView represents a single node in the execution graph.
type nodeView struct {
	ID        string `json:"id" yaml:"id"`
	Operation string `json:"operation" yaml:"operation"`
	Source    string `json:"source,omitempty" yaml:"source,omitempty"`
	Target    string `json:"target,omitempty" yaml:"target,omitempty"`
	Status    string `json:"status" yaml:"status"`
}

// edgeView represents a dependency between nodes.
type edgeView struct {
	From string `json:"from" yaml:"from"`
	To   string `json:"to" yaml:"to"`
}

func formatMigrationYAML(w io.Writer, graph *op.Graph, analysis *MigrationAnalysis) error {
	view := buildMigrationView(graph, analysis)
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	return enc.Encode(view)
}

func formatMigrationJSON(w io.Writer, graph *op.Graph, analysis *MigrationAnalysis) error {
	view := buildMigrationView(graph, analysis)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(view)
}

func buildMigrationView(graph *op.Graph, analysis *MigrationAnalysis) *migrationView {
	// Build nodes
	var nodes []nodeView
	for _, node := range graph.Nodes {
		source, _ := node.GetSlot("source").(string) //nolint:errcheck // zero value (empty) is acceptable
		target, _ := node.GetSlot("path").(string)   //nolint:errcheck // zero value (empty) is acceptable
		nodes = append(nodes, nodeView{
			ID:        node.ID,
			Operation: node.ActionName(),
			Source:    source,
			Target:    target,
			Status:    string(node.Status),
		})
	}

	// Build edges
	var edges []edgeView
	for _, edge := range graph.Edges {
		edges = append(edges, edgeView{
			From: edge.From,
			To:   edge.To,
		})
	}

	return &migrationView{
		Analysis: analysis,
		ExecutionGraph: &executionGraphView{
			Version: "1.0",
			Tool:    "writ",
			State:   "pending",
			Context: graphContext{
				SourceRoot: analysis.SourceRoot,
			},
			Nodes: nodes,
			Edges: edges,
		},
	}
}

func formatMigrationText(w io.Writer, graph *op.Graph, analysis *MigrationAnalysis) error {
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
	_, _ = fmt.Fprintf(w, "Migration Plan\n")                  //nolint:errcheck // table output
	_, _ = fmt.Fprintf(w, "Source: %s\n", analysis.SourceRoot) //nolint:errcheck // table output
	_, _ = fmt.Fprintf(w, "System: %s", analysis.System)       //nolint:errcheck // status output
	if analysis.SystemConfidence > 0 {
		_, _ = fmt.Fprintf(w, " (confidence: %.0f%%)", analysis.SystemConfidence*100) //nolint:errcheck // status output
	}
	_, _ = fmt.Fprintln(w) //nolint:errcheck // table output
	_, _ = fmt.Fprintln(w) //nolint:errcheck // table output
}

func formatSummary(w io.Writer, s MigrationStats) {
	_, _ = fmt.Fprintf(w, "Summary:\n")                                   //nolint:errcheck // status output
	_, _ = fmt.Fprintf(w, "  Files: %d | Projects: %d | Platforms: %d\n", //nolint:errcheck // table output
		s.TotalFiles, s.Projects, s.Platforms)
	_, _ = fmt.Fprintf(w, "  Configs: %d | Scripts: %d | Lifecycle: %d\n", //nolint:errcheck // table output
		s.StaticConfigs, s.Scripts, s.LifecycleScripts)

	extras := collectExtraStats(s)
	if len(extras) > 0 {
		_, _ = fmt.Fprintf(w, "  %s\n", strings.Join(extras, " | ")) //nolint:errcheck // table output
	}
	_, _ = fmt.Fprintln(w) //nolint:errcheck // table output
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

func formatRenames(w io.Writer, graph *op.Graph, sourceRoot string) {
	renameNodes := filterNodesByAction(graph, "file.move")
	if len(renameNodes) == 0 {
		return
	}

	_, _ = fmt.Fprintf(w, "Directory renames (%d):\n", len(renameNodes)) //nolint:errcheck // table output
	maxLen := 0
	for _, node := range renameNodes {
		source, _ := node.GetSlot("source").(string) //nolint:errcheck // zero value (empty) is acceptable
		if len(source) > maxLen {
			maxLen = len(source)
		}
	}
	for _, node := range renameNodes {
		src, _ := node.GetSlot("source").(string) //nolint:errcheck // zero value (empty) is acceptable
		tgt, _ := node.GetSlot("path").(string)   //nolint:errcheck // zero value (empty) is acceptable
		source := shortenPath(src, sourceRoot)
		target := shortenPath(tgt, sourceRoot)
		_, _ = fmt.Fprintf(w, "  %-*s  →  %s\n", maxLen-len(sourceRoot), source, target) //nolint:errcheck // table output
	}
	_, _ = fmt.Fprintln(w) //nolint:errcheck // status output
}

func formatScripts(w io.Writer, scripts []ScriptAnalysis) {
	if len(scripts) == 0 {
		return
	}

	_, _ = fmt.Fprintf(w, "Lifecycle scripts (%d):\n", len(scripts)) //nolint:errcheck // table output
	for i := range scripts {
		formatScript(w, &scripts[i])
	}
	_, _ = fmt.Fprintln(w) //nolint:errcheck // status output
}

func formatScript(w io.Writer, script *ScriptAnalysis) {
	_, _ = fmt.Fprintf(w, "  %s\n", script.RelPath)                              //nolint:errcheck // table output
	_, _ = fmt.Fprintf(w, "    %s | %d lines\n", script.Phase, script.LineCount) //nolint:errcheck // table output

	if len(script.Resolved) > 0 {
		var names []string
		for _, r := range script.Resolved {
			names = append(names, r.LorePackage)
		}
		_, _ = fmt.Fprintf(w, "    Lore packages: %s\n", strings.Join(names, ", ")) //nolint:errcheck // table output
	}

	if len(script.Unresolved) > 0 {
		var installs []string
		for _, u := range script.Unresolved {
			installs = append(installs, fmt.Sprintf("%s:%s", u.Manager, u.Name))
		}
		_, _ = fmt.Fprintf(w, "    Unknown: %s\n", strings.Join(installs, ", ")) //nolint:errcheck // table output
	}

	for _, obs := range script.Observations {
		_, _ = fmt.Fprintf(w, "    %s\n", obs) //nolint:errcheck // table output
	}
}

func formatStringList(w io.Writer, title string, items []string) {
	if len(items) == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "%s:\n", title) //nolint:errcheck // table output
	for _, item := range items {
		_, _ = fmt.Fprintf(w, "  - %s\n", item) //nolint:errcheck // table output
	}
	_, _ = fmt.Fprintln(w) //nolint:errcheck // status output
}

func formatSecrets(w io.Writer, secrets []SecretFinding) {
	if len(secrets) == 0 {
		return
	}

	_, _ = fmt.Fprintf(w, "Secrets detected (%d):\n", len(secrets)) //nolint:errcheck // table output
	hasUnencrypted := false
	for _, secret := range secrets {
		formatSecret(w, secret)
		if secret.Encryption == EncryptNone {
			hasUnencrypted = true
		}
	}
	_, _ = fmt.Fprintln(w) //nolint:errcheck // table output

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
	_, _ = fmt.Fprintf(w, "  %s %s%s\n", icon, secret.RelPath, encLabel) //nolint:errcheck // table output
	_, _ = fmt.Fprintf(w, "      %s\n", secret.Reason)                   //nolint:errcheck // table output
}

func formatRecommendations(w io.Writer, recommendations []string) {
	if len(recommendations) == 0 {
		return
	}
	_, _ = fmt.Fprintf(w, "TODOs after migration:\n") //nolint:errcheck // table output
	for i, rec := range recommendations {
		_, _ = fmt.Fprintf(w, "  %d. %s\n", i+1, rec) //nolint:errcheck // table output
	}
}

// filterNodesByAction returns nodes that have the specified action.
func filterNodesByAction(graph *op.Graph, actionName string) []*op.Node {
	var nodes []*op.Node
	for _, node := range graph.Nodes {
		if node.ActionName() == actionName {
			nodes = append(nodes, node)
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
	_, _ = fmt.Fprintf(w, "SOPS Setup Recommendation:\n")                                                                        //nolint:errcheck // table output
	_, _ = fmt.Fprintf(w, "  1. Install SOPS: brew install sops  # or: go install github.com/getsops/sops/v3/cmd/sops@latest\n") //nolint:errcheck // table output
	_, _ = fmt.Fprintf(w, "  2. Create age key: age-keygen -o ~/.config/sops/age/keys.txt\n")                                    //nolint:errcheck // table output
	_, _ = fmt.Fprintf(w, "  3. Create .sops.yaml with your public key:\n")                                                      //nolint:errcheck // table output
	_, _ = fmt.Fprintln(w)                                                                                                       //nolint:errcheck // table output
	_, _ = fmt.Fprintf(w, "     # .sops.yaml\n")                                                                                 //nolint:errcheck // table output
	_, _ = fmt.Fprintf(w, "     creation_rules:\n")                                                                              //nolint:errcheck // table output

	// Collect unique patterns
	patterns := make(map[string]bool)
	for _, s := range secrets {
		if s.Encryption == EncryptNone && s.SuggestedPattern != "" {
			patterns[s.SuggestedPattern] = true
		}
	}

	for pattern := range patterns {
		_, _ = fmt.Fprintf(w, "       - path_regex: %s\n", pattern)        //nolint:errcheck // table output
		_, _ = fmt.Fprintf(w, "         age: \"<your-age-public-key>\"\n") //nolint:errcheck // table output
	}

	_, _ = fmt.Fprintln(w)                                                              //nolint:errcheck // table output
	_, _ = fmt.Fprintf(w, "  4. Encrypt each secret: sops encrypt --in-place <file>\n") //nolint:errcheck // table output
	_, _ = fmt.Fprintf(w, "  5. Commit .sops.yaml and encrypted files\n")               //nolint:errcheck // table output
	_, _ = fmt.Fprintln(w)                                                              //nolint:errcheck // status output
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

	_, _ = fmt.Fprintln(w, resp.Content) //nolint:errcheck // status output
	return nil
}
