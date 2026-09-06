// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package migrate

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/NobleFactor/devlore-cli/cmd/internal/model"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// MigrationView is the migration plan as a result: the analysis and the execution graph, side by side at
// the top level. It is what `writ migrate --dry-run` emits, and the shared pipeline renders it -- every
// presentation is a presentation of this JSON, so there is no text renderer here and no format decision.
type MigrationView struct {
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

// NewMigrationView builds the plan's result value from the execution graph and its analysis.
//
// Parameters:
//   - `graph`: the execution graph the migration would run.
//   - `analysis`: the analysis the graph was built from.
//
// Returns:
//   - `*MigrationView`: the combined view, ready for the pipeline.
func NewMigrationView(graph *op.Graph, analysis *MigrationAnalysis) *MigrationView {

	var nodes []nodeView
	for _, node := range graph.Nodes() {
		source := immediateString(node, "source_path")
		if source == "" {
			source = immediateString(node, "source") // file.copy keeps `source` (a content read)
		}
		target := immediateString(node, "destination_path")
		nodes = append(nodes, nodeView{
			ID:        node.ID(),
			Operation: string(actionName(node)),
			Source:    source,
			Target:    target,
			Status:    "pending",
		})
	}

	var edges []edgeView
	for _, edge := range graph.Edges() {
		edges = append(edges, edgeView{
			From: edge.From,
			To:   edge.To,
		})
	}

	return &MigrationView{
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
