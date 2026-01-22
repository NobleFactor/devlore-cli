// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package tree

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// String returns a human-readable representation of the tree.
func (t *Tree) String() string {
	var sb strings.Builder

	// Header
	sb.WriteString(fmt.Sprintf("Deployment Tree: %s → %s\n\n", t.SourceRoot, t.TargetRoot))

	// Projects
	sb.WriteString(fmt.Sprintf("Projects: %s\n\n", strings.Join(t.Projects, ", ")))

	// Matched directories
	sb.WriteString("Matched directories:\n")
	for _, m := range t.MatchedDirs {
		sb.WriteString(fmt.Sprintf("  %s/\n", filepath.Base(m.Path)))
	}
	sb.WriteString("\n")

	// Collision warnings
	if t.HasCollisions() {
		sb.WriteString(fmt.Sprintf("Collisions (%d):\n", len(t.Collisions)))
		for _, c := range t.Collisions {
			sb.WriteString(fmt.Sprintf("  %s: %s (specificity %d) overrides %s (specificity %d)\n",
				c.Target,
				filepath.Base(filepath.Dir(c.Winner)),
				c.WinnerSpecificity,
				filepath.Base(filepath.Dir(c.Loser)),
				c.LoserSpecificity))
		}
		sb.WriteString("\n")
	}

	// Summary
	sb.WriteString(fmt.Sprintf("Files (%d):\n", len(t.Nodes)))
	sb.WriteString(fmt.Sprintf("  Links: %d, Templates: %d, Secrets: %d\n\n",
		t.LinkCount(), t.TemplateCount(), t.SecretCount()))

	// File list
	for _, n := range t.Nodes {
		// Format: relTarget → target [operations]
		ops := formatOps(n.Operations)
		sb.WriteString(fmt.Sprintf("  %-40s → ~/%s %s\n",
			n.RelTarget, n.RelTarget, ops))
	}

	return sb.String()
}

// formatOps formats operations for display.
func formatOps(ops Operations) string {
	if len(ops) == 1 && ops[0] == OpLink {
		return "[link]"
	}
	return "[" + strings.Join(ops.Strings(), ", ") + "]"
}

// JSON returns the tree as JSON.
func (t *Tree) JSON() ([]byte, error) {
	return json.MarshalIndent(t, "", "  ")
}

// CompactString returns a compact summary of the tree.
func (t *Tree) CompactString() string {
	return fmt.Sprintf("%d files (%d links, %d templates, %d secrets) from %d directories",
		t.FileCount(), t.LinkCount(), t.TemplateCount(), t.SecretCount(), len(t.MatchedDirs))
}

// NodesByProject returns nodes grouped by project.
func (t *Tree) NodesByProject() map[string][]*Node {
	groups := make(map[string][]*Node)
	for _, n := range t.Nodes {
		groups[n.Project] = append(groups[n.Project], n)
	}
	return groups
}

// NodesByOperation returns nodes grouped by primary operation type.
func (t *Tree) NodesByOperation() map[Operation][]*Node {
	groups := make(map[Operation][]*Node)
	for _, n := range t.Nodes {
		if len(n.Operations) > 0 {
			// Group by first operation (primary type)
			primary := n.Operations[0]
			// But if it's decrypt or expand, that's the interesting one
			for _, op := range n.Operations {
				if op == OpDecrypt || op == OpExpand {
					primary = op
					break
				}
			}
			groups[primary] = append(groups[primary], n)
		}
	}
	return groups
}
