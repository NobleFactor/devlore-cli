// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

// ErrManNotAvailable indicates the man command is not available on this system.
var ErrManNotAvailable = errors.New("man command not available")

// ManHeader contains metadata for man page generation.
type ManHeader struct {
	Title   string
	Section string
	Source  string
	Manual  string
}

// NewManCmd creates the man command for displaying/installing man pages.
// Usage:
//
//	tool man              # display man page with pager
//	tool man --install    # install to ~/.local/share/man/man1/
//	tool man deploy       # display man page for subcommand
func NewManCmd(rootCmd *cobra.Command, header ManHeader) *cobra.Command {
	var install bool
	var installPath string

	cmd := &cobra.Command{
		Use:   "man [command]",
		Short: "Display or install man pages",
		Long: `Generate and display man pages for ` + rootCmd.Name() + ` commands.

By default, displays the man page using your system pager.
Use --install to install man pages to a directory.

Examples:
  ` + rootCmd.Name() + ` man              # display main man page
  ` + rootCmd.Name() + ` man deploy       # display man page for deploy command
  ` + rootCmd.Name() + ` man --install    # install all man pages
`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			h := &doc.GenManHeader{
				Title:   header.Title,
				Section: header.Section,
				Date:    new(time.Now()),
				Source:  header.Source,
				Manual:  header.Manual,
			}

			if install {
				return installManPages(rootCmd, h, installPath)
			}

			// Find the command to document
			targetCmd := rootCmd
			if len(args) == 1 {
				var err error
				targetCmd, _, err = rootCmd.Find(args)
				if err != nil || targetCmd == nil {
					return fmt.Errorf("unknown command: %s", args[0])
				}
			}

			err := DisplayManPage(targetCmd, h)
			if errors.Is(err, ErrManNotAvailable) {
				return fmt.Errorf("man command not available on this system; use '%s help %s' instead", rootCmd.Name(), targetCmd.Name())
			}
			return err
		},
	}

	defaultPath := ManPath()
	cmd.Flags().BoolVar(&install, "install", false, "Install man pages to directory")
	cmd.Flags().StringVar(&installPath, "path", defaultPath, "Installation directory for man pages")

	// Hide from help output (like Cobra's built-in completion command)
	cmd.Hidden = true

	return cmd
}

// DisplayManPage generates a man page and displays it with the system pager.
// Returns ErrManNotAvailable if man is not available on this system.
func DisplayManPage(cmd *cobra.Command, header *doc.GenManHeader) error {
	// Check if man command is available
	if !isManAvailable() {
		return ErrManNotAvailable
	}

	// Create temp file for man page
	tmpFile, err := os.CreateTemp("", cmd.Name()+".1")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() { os.Remove(tmpFile.Name()) }() //nolint:errcheck,gosec // best-effort cleanup; G703: temp file path

	// Generate man page to temp file
	if err := doc.GenMan(cmd, header, tmpFile); err != nil {
		return fmt.Errorf("failed to generate man page: %w", err)
	}
	_ = tmpFile.Close()

	// Display with man command
	manCmd := exec.CommandContext(context.Background(), "man", tmpFile.Name()) //nolint:gosec // G204: argument is a temp file we created
	manCmd.Stdout = os.Stdout
	manCmd.Stderr = os.Stderr
	manCmd.Stdin = os.Stdin

	return manCmd.Run()
}

// isManAvailable checks if the man command is available on this system.
func isManAvailable() bool {
	_, err := exec.LookPath("man")
	return err == nil
}

// installManPages installs man pages for all commands to the specified directory.
func installManPages(rootCmd *cobra.Command, header *doc.GenManHeader, path string) error {
	// Create directory if needed
	if err := os.MkdirAll(path, 0o750); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", path, err)
	}

	// Generate man pages
	if err := doc.GenManTree(rootCmd, header, path); err != nil {
		return fmt.Errorf("failed to generate man pages: %w", err)
	}

	fmt.Printf("Man pages installed to %s\n", path)
	fmt.Println("Ensure this path is in your MANPATH:")
	fmt.Printf("  export MANPATH=\"%s:$MANPATH\"\n", filepath.Dir(path))

	return nil
}
