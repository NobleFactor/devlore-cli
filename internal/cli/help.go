// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

// NewHelpCmd creates a help command that prefers man pages when available.
// This follows git's model: if man pages are installed, display them via pager;
// otherwise fall back to console text output.
func NewHelpCmd(rootCmd *cobra.Command, header ManHeader) *cobra.Command {
	return &cobra.Command{
		Use:   "help [command]",
		Short: "Help about any command",
		Long: `Display help for ` + rootCmd.Name() + ` commands.

If man pages are installed (via self-install or man --install), displays
the man page using your system pager. Otherwise, displays console text help.

This follows git's model: rich documentation when available, with graceful
fallback to built-in help.

Examples:
  ` + rootCmd.Name() + ` help           # help for ` + rootCmd.Name() + ` itself
  ` + rootCmd.Name() + ` help deploy    # help for the deploy command
`,
		Args:               cobra.MaximumNArgs(1),
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Find target command
			targetCmd := rootCmd
			if len(args) > 0 {
				targetCmd, _, _ = rootCmd.Find(args)
				if targetCmd == nil {
					// Unknown command - show root help
					return rootCmd.Help()
				}
			}

			// Build man page path
			manPath := manPagePath(rootCmd.Name(), targetCmd, rootCmd)

			// Check if man page exists
			if _, err := os.Stat(manPath); err == nil {
				// Man page exists - try to display it via pager
				now := time.Now()
				h := &doc.GenManHeader{
					Title:   header.Title,
					Section: header.Section,
					Date:    &now,
					Source:  header.Source,
					Manual:  header.Manual,
				}
				err := DisplayManPage(targetCmd, h)
				if err == nil {
					return nil
				}
				// If man is not available, fall back to console help
				if errors.Is(err, ErrManNotAvailable) {
					return targetCmd.Help()
				}
				return err
			}

			// Fall back to Cobra's built-in help
			return targetCmd.Help()
		},
	}
}

// manPagePath returns the expected path for a command's man page.
// For root command: <tool>.1
// For subcommand: <tool>-<subcommand>.1
func manPagePath(toolName string, targetCmd, rootCmd *cobra.Command) string {
	if targetCmd == rootCmd {
		return filepath.Join(ManPath(), toolName+".1")
	}

	// Build the command path (e.g., "lore-deploy" for "lore deploy")
	var parts []string
	for cmd := targetCmd; cmd != nil && cmd != rootCmd; cmd = cmd.Parent() {
		parts = append([]string{cmd.Name()}, parts...)
	}
	manName := toolName + "-" + strings.Join(parts, "-") + ".1"
	return filepath.Join(ManPath(), manName)
}
