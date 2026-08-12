// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package deploy

import (
	"fmt"

	"github.com/NobleFactor/devlore-cli/cmd/writ/writ/tree"
	"github.com/NobleFactor/devlore-cli/internal/cli"
	"github.com/NobleFactor/devlore-cli/pkg/op"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/encryption"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/file"
	"github.com/NobleFactor/devlore-cli/pkg/op/provider/pkg"
)

// reportContext narrates the planning context under --verbose.
//
// Parameters:
//   - `cfg`: the deploy configuration to narrate.
func reportContext(cfg *Config) {

	if len(cfg.LayerSources) > 0 {
		cli.Note("Layers: %d sources", len(cfg.LayerSources))
		for _, src := range cfg.LayerSources {
			cli.Note("  %s/%s: %s → %s", src.Layer, src.TargetName, src.SourceRoot, src.TargetRoot)
		}
	} else {
		cli.Note("Source: %s", cfg.SourceRoot)
		cli.Note("Target: %s", cfg.TargetRoot)
	}

	cli.Note("Projects: %v", cfg.Projects)
	cli.Note("Segments: %s", cfg.Segments.String())
}

// reportCollisions warns about the tree build's resolved source conflicts.
//
// Parameters:
//   - `cfg`: the deploy configuration (layer mode selects the message shape).
//   - `collisions`: the resolved collisions to report.
func reportCollisions(cfg *Config, collisions []tree.Collision) {

	if len(cfg.LayerSources) > 0 {
		cli.Warn("%d source collision(s) resolved by layer/specificity:", len(collisions))
		for _, c := range collisions {
			cli.Warn("  %s: using %s [%s] over %s [%s]",
				c.Target, c.Winner, c.WinnerLayer, c.Loser, c.LoserLayer)
		}
		return
	}

	cli.Warn("%d source collision(s) resolved by specificity:", len(collisions))
	for _, c := range collisions {
		cli.Warn("  %s: using %s (specificity %d) over %s (specificity %d)",
			c.Target, c.Winner, c.WinnerSpecificity, c.Loser, c.LoserSpecificity)
	}
}

// formatSummary renders a human-readable deployment summary from a trace's per-action tally.
//
// Parameters:
//   - `s`: the tally from [op.Trace.Summarize].
//
// Returns:
//   - `string`: the summary line (e.g. "3 files (2 links, 1 templates)").
func formatSummary(s op.Summary) string {

	byAction := s.ByAction()

	completed := func(name op.ActionName) int {
		if a, ok := byAction[string(name)]; ok {
			return a.Completed()
		}
		return 0
	}

	links := completed(file.Link)
	templates := completed(file.WriteText) + completed(file.WriteBytes)
	secrets := completed(encryption.DecryptSopsFile)
	copies := completed(file.Copy)
	totalFiles := links + templates + secrets + copies

	packages := completed(pkg.Install) + completed(pkg.Upgrade) + completed(pkg.Remove)

	if packages > 0 {
		result := fmt.Sprintf("%d packages", packages)
		if s.Skipped() > 0 {
			result += fmt.Sprintf(", %d skipped", s.Skipped())
		}
		if s.Failed() > 0 {
			result += fmt.Sprintf(", %d failed", s.Failed())
		}
		return result
	}

	result := fmt.Sprintf("%d files", totalFiles)
	if links > 0 {
		result += fmt.Sprintf(" (%d links", links)
		if templates > 0 {
			result += fmt.Sprintf(", %d templates", templates)
		}
		if secrets > 0 {
			result += fmt.Sprintf(", %d secrets", secrets)
		}
		if copies > 0 {
			result += fmt.Sprintf(", %d copies", copies)
		}
		result += ")"
	}
	if s.Skipped() > 0 {
		result += fmt.Sprintf(", %d skipped", s.Skipped())
	}
	if s.Failed() > 0 {
		result += fmt.Sprintf(", %d failed", s.Failed())
	}
	return result
}
