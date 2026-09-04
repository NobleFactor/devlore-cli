// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/NobleFactor/devlore-cli/cmd/internal/devlore"
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
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
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			h := &doc.GenManHeader{
				Title:   header.Title,
				Section: header.Section,
				Date:    new(time.Now()),
				Source:  header.Source,
				Manual:  header.Manual,
			}

			if install {
				// The command owns the root: --path lets the operator name any directory, and OpenTree
				// because it need not exist yet (#405, phase 2b). An operator naming a relative path is
				// refused rather than resolved against the working directory.
				manRoot, err := OpenTree(installPath)
				if err != nil {
					return err
				}
				defer iox.Close(&err, manRoot)

				return installManPages(rootCmd, h, manRoot)
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

			err = DisplayManPage(targetCmd, h)
			if errors.Is(err, ErrManNotAvailable) {
				return fmt.Errorf("man command not available on this system; use '%s help %s' instead", rootCmd.Name(), targetCmd.Name())
			}
			return err
		},
	}

	defaultPath := devlore.ManPath()
	cmd.Flags().BoolVar(&install, "install", false, "Install man pages to directory")
	cmd.Flags().StringVar(&installPath, "path", defaultPath, "Installation directory for man pages")

	// Hide from help output (like Cobra's built-in completion command)
	cmd.Hidden = true

	return cmd
}

// DisplayManPage generates a man page and displays it with the system pager.
// Returns ErrManNotAvailable if man is not available on this system.
func DisplayManPage(cmd *cobra.Command, header *doc.GenManHeader) (err error) {
	// Check if man command is available
	if !isManAvailable() {
		return ErrManNotAvailable
	}

	// A scratch root owns the temporary tree's lifetime: Close removes it, which is the teardown the
	// hand-rolled CreateTemp plus deferred Remove was approximating (#405, phase 2b). The page is also
	// created 0600 by the root rather than by luck.
	scratch, err := fsroot.OpenScratch(cmd.Name())
	if err != nil {
		return fmt.Errorf("open scratch for man page: %w", err)
	}
	defer iox.Close(&err, scratch)

	page, pagePath, err := scratch.CreateTemp(scratch.NewPath("."), cmd.Name()+".1")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	// Generate man page to temp file
	if err := doc.GenMan(cmd, header, page); err != nil {
		return fmt.Errorf("failed to generate man page: %w", err)
	}
	_ = page.Close() //nolint:errcheck // the scratch tree is removed on Close regardless

	// Display with man command
	manCmd := exec.CommandContext(context.Background(), "man", pagePath.Abs()) //nolint:gosec // G204: argument is a temp file we created

	return RunInteractive(manCmd, "pass --help, or read the pages `self install` writes under the prefix")
}

// isManAvailable checks if the man command is available on this system.
func isManAvailable() bool {
	_, err := exec.LookPath("man")
	return err == nil
}

// installManPages installs man pages for all commands into the directory `manRoot` anchors.
//
// The root is received, never constructed (#405, phase 2b): the command owns it, because `--path` lets the
// operator name any directory.
//
// Parameters:
//   - `rootCmd`: the command tree to generate pages for.
//   - `header`: the man page header metadata.
//   - `manRoot`: the destination directory, opened by the caller.
//
// Returns:
//   - `error`: non-nil when the directory cannot be created or generation fails.
func installManPages(rootCmd *cobra.Command, header *doc.GenManHeader, manRoot fsroot.Dir) error {
	// Create directory if needed. NewPath(".") is the root's own directory, so the 0750 is applied by the
	// root that anchors it and reaches a Windows DACL rather than being ignored.
	if err := manRoot.MkdirAll(manRoot.NewPath("."), 0o750); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", manRoot.Name(), err)
	}

	// Unsandboxed: cobra's generator writes the page files itself, given a directory path — those writes are
	// not ours to route through the root. What we own, the directory and its mode, goes through it above.
	if err := doc.GenManTree(rootCmd, header, manRoot.Name()); err != nil {
		return fmt.Errorf("failed to generate man pages: %w", err)
	}

	Success("Man pages installed to %s", manRoot.Name())
	Note("Ensure this path is in your MANPATH:")
	Note("  export MANPATH=\"%s:$MANPATH\"", filepath.Dir(manRoot.Name()))

	return nil
}
