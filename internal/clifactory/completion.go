// SPDX-License-Identifier: MIT
// Copyright (c) 2025 Noble Factor. All rights reserved.

// Package clifactory provides shared CLI infrastructure for lore and writ.
package clifactory

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// NewCompletionCmd creates the completion command for shell completions.
// Usage:
//
//	tool completion bash              # output to stdout
//	tool completion bash --install    # install to XDG_DATA_HOME
//	eval "$(tool completion bash)"
func NewCompletionCmd(rootCmd *cobra.Command) *cobra.Command {
	var install bool

	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Long: `Generate shell completion scripts for the specified shell.

To load completions:

Bash:
  $ source <(` + rootCmd.Name() + ` completion bash)
  # Or install to XDG location:
  $ ` + rootCmd.Name() + ` completion bash --install

Zsh:
  $ source <(` + rootCmd.Name() + ` completion zsh)
  # Or install to XDG location:
  $ ` + rootCmd.Name() + ` completion zsh --install

Fish:
  $ ` + rootCmd.Name() + ` completion fish | source
  # Or install to XDG location:
  $ ` + rootCmd.Name() + ` completion fish --install

PowerShell:
  PS> ` + rootCmd.Name() + ` completion powershell | Out-String | Invoke-Expression
`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			shell := args[0]

			if install {
				return installCompletion(rootCmd, shell)
			}

			return outputCompletion(rootCmd, shell)
		},
	}

	cmd.Flags().BoolVar(&install, "install", false, "Install completion to XDG-compliant directory")

	return cmd
}

// outputCompletion writes completion script to stdout.
func outputCompletion(rootCmd *cobra.Command, shell string) error {
	switch shell {
	case "bash":
		return rootCmd.GenBashCompletion(os.Stdout)
	case "zsh":
		return rootCmd.GenZshCompletion(os.Stdout)
	case "fish":
		return rootCmd.GenFishCompletion(os.Stdout, true)
	case "powershell":
		return rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
	}
	return nil
}

// installCompletion installs completion script to XDG-compliant directory.
func installCompletion(rootCmd *cobra.Command, shell string) error {
	var path string
	var filename string

	switch shell {
	case "bash":
		path = BashCompletionPath()
		filename = rootCmd.Name()
	case "zsh":
		path = ZshCompletionPath()
		filename = "_" + rootCmd.Name()
	case "fish":
		path = FishCompletionPath()
		filename = rootCmd.Name() + ".fish"
	case "powershell":
		return fmt.Errorf("PowerShell completion --install not supported; output to stdout and add to profile")
	}

	// Create directory
	if err := os.MkdirAll(path, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", path, err)
	}

	// Create file
	fullPath := filepath.Join(path, filename)
	f, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create %s: %w", fullPath, err)
	}
	defer f.Close()

	// Generate completion
	switch shell {
	case "bash":
		err = rootCmd.GenBashCompletion(f)
	case "zsh":
		err = rootCmd.GenZshCompletion(f)
	case "fish":
		err = rootCmd.GenFishCompletion(f, true)
	}

	if err != nil {
		return fmt.Errorf("failed to generate completion: %w", err)
	}

	fmt.Printf("Installed %s completion to %s\n", shell, fullPath)

	// Print shell-specific instructions
	switch shell {
	case "bash":
		fmt.Println("\nEnsure bash-completion is configured to read from XDG_DATA_HOME:")
		fmt.Printf("  export BASH_COMPLETION_USER_DIR=\"%s\"\n", filepath.Dir(path))
	case "zsh":
		fmt.Println("\nEnsure your fpath includes the XDG location:")
		fmt.Printf("  fpath=(%s $fpath)\n", path)
	}

	return nil
}
