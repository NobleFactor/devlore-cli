// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

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

	// Multi-source mode: show all sources
	if len(r.Sources) > 0 {
		_, _ = fmt.Fprintf(&sb, "Deployment: %d layers → %s\n", len(r.Sources), r.TargetRoot)
		for _, src := range r.Sources {
			_, _ = fmt.Fprintf(&sb, "  %s: %s\n", src.Layer, src.SourceRoot)
		}
		sb.WriteString("\n")
	} else {
		// Single-source mode
		_, _ = fmt.Fprintf(&sb, "Deployment: %s → %s\n\n", r.SourceRoot, r.TargetRoot)
	}
	_, _ = fmt.Fprintf(&sb, "Projects: %s\n\n", strings.Join(r.Projects, ", "))

	sb.WriteString("Matched directories:\n")
	for _, m := range r.MatchedDirs {
		_, _ = fmt.Fprintf(&sb, "  %s/\n", filepath.Base(m.Path))
	}
	sb.WriteString("\n")

	if r.HasCollisions() {
		_, _ = fmt.Fprintf(&sb, "Collisions (%d):\n", len(r.Collisions))
		for _, c := range r.Collisions {
			_, _ = fmt.Fprintf(&sb, "  %s: %s (specificity %d) overrides %s (specificity %d)\n",
				c.Target,
				filepath.Base(filepath.Dir(c.Winner)),
				c.WinnerSpecificity,
				filepath.Base(filepath.Dir(c.Loser)),
				c.LoserSpecificity)
		}
		sb.WriteString("\n")
	}

	_, _ = fmt.Fprintf(&sb, "Files (%d):\n", r.FileCount())
	_, _ = fmt.Fprintf(&sb, "  Links: %d, Templates: %d, Secrets: %d\n\n",
		r.LinkCount(), r.TemplateCount(), r.SecretCount())

	for _, f := range r.Files {
		actions := "[" + strings.Join(f.Operations, ", ") + "]"
		_, _ = fmt.Fprintf(&sb, "  %-40s %s\n", f.ID, actions)
	}

	return sb.String()
}
