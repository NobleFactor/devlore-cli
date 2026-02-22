// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package execution

import (
	"fmt"
	"time"

	"github.com/NobleFactor/devlore-cli/pkg/projection"
)

// HydrateGraph replaces stub actions on graph nodes with real actions from the registry.
// This enables loaded/deserialized graphs to be executed. Nodes with no action name
// (e.g., nodes that were never serialized with an action) are skipped.
func HydrateGraph(g *projection.Graph, reg *ActionRegistry) error {
	for _, n := range g.Nodes {
		name := n.ActionName()
		if name == "" {
			continue
		}
		action, ok := reg.Get(name)
		if !ok {
			return fmt.Errorf("hydrate: unknown action %q on node %q", name, n.ID)
		}
		n.Action = action
	}
	return nil
}

// ApplyResults updates node states from execution results.
func ApplyResults(g *projection.Graph, results []*NodeResult) {
	resultMap := make(map[string]*NodeResult)
	for _, r := range results {
		resultMap[r.NodeID] = r
	}

	for _, n := range g.Nodes {
		if r, ok := resultMap[n.ID]; ok {
			switch r.Status {
			case ResultCompleted:
				n.Status = projection.StatusCompleted
			case ResultSkipped:
				n.Status = projection.StatusSkipped
			case ResultFailed:
				n.Status = projection.StatusFailed
				if r.Error != nil {
					n.Error = r.Error.Error()
				}
			}
			n.Timestamp = time.Now().Format(time.RFC3339)
		}
	}
}
