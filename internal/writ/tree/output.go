// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package tree

import (
	"fmt"
	"path/filepath"
	"strings"
)

// CompactString returns a compact summary of the build result.
func (r *BuildResult) CompactString() string {
	return fmt.Sprintf("%d files (%d links, %d templates, %d secrets) from %d directories",
		r.FileCount(), r.LinkCount(), r.TemplateCount(), r.SecretCount(), len(r.MatchedDirs))
}

// String returns a human-readable representation of the build result.
func (r *BuildResult) String() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Deployment: %s → %s\n\n", r.SourceRoot, r.TargetRoot))
	sb.WriteString(fmt.Sprintf("Projects: %s\n\n", strings.Join(r.Projects, ", ")))

	sb.WriteString("Matched directories:\n")
	for _, m := range r.MatchedDirs {
		sb.WriteString(fmt.Sprintf("  %s/\n", filepath.Base(m.Path)))
	}
	sb.WriteString("\n")

	if r.HasCollisions() {
		sb.WriteString(fmt.Sprintf("Collisions (%d):\n", len(r.Collisions)))
		for _, c := range r.Collisions {
			sb.WriteString(fmt.Sprintf("  %s: %s (specificity %d) overrides %s (specificity %d)\n",
				c.Target,
				filepath.Base(filepath.Dir(c.Winner)),
				c.WinnerSpecificity,
				filepath.Base(filepath.Dir(c.Loser)),
				c.LoserSpecificity))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("Files (%d):\n", r.FileCount()))
	sb.WriteString(fmt.Sprintf("  Links: %d, Templates: %d, Secrets: %d\n\n",
		r.LinkCount(), r.TemplateCount(), r.SecretCount()))

	for _, n := range r.Graph.Nodes {
		ops := "[" + strings.Join(n.Operations, ", ") + "]"
		sb.WriteString(fmt.Sprintf("  %-40s %s\n", n.ID, ops))
	}

	return sb.String()
}
