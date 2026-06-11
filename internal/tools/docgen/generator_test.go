// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package docgen

import (
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func TestShouldSkip(t *testing.T) {
	tests := []struct {
		name     string
		cmd      *cobra.Command
		expected bool
	}{
		{
			name:     "hidden command is skipped",
			cmd:      &cobra.Command{Use: "secret", Hidden: true},
			expected: true,
		},
		{
			name:     "help command is skipped",
			cmd:      &cobra.Command{Use: "help"},
			expected: true,
		},
		{
			name:     "completion command is skipped",
			cmd:      &cobra.Command{Use: "completion"},
			expected: true,
		},
		{
			name:     "man command is skipped",
			cmd:      &cobra.Command{Use: "man"},
			expected: true,
		},
		{
			name:     "version command is skipped",
			cmd:      &cobra.Command{Use: "version"},
			expected: true,
		},
		{
			name:     "normal command is not skipped",
			cmd:      &cobra.Command{Use: "add"},
			expected: false,
		},
		{
			name:     "normal command with args is not skipped",
			cmd:      &cobra.Command{Use: "repo [name]"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldSkip(tt.cmd)
			if got != tt.expected {
				t.Errorf("shouldSkip(%q) = %v, want %v", tt.cmd.Name(), got, tt.expected)
			}
		})
	}
}

func TestOutputPath(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *cobra.Command
		outDir   string
		expected string
	}{
		{
			name: "fsroot command",
			setup: func() *cobra.Command {
				return &cobra.Command{Use: "writ"}
			},
			outDir:   "dir",
			expected: filepath.Join("dir", "writ.md"),
		},
		{
			name: "subcommand",
			setup: func() *cobra.Command {
				root := &cobra.Command{Use: "writ"}
				child := &cobra.Command{Use: "add"}
				root.AddCommand(child)
				return child
			},
			outDir:   "dir",
			expected: filepath.Join("dir", "writ", "add.md"),
		},
		{
			name: "nested subcommand",
			setup: func() *cobra.Command {
				root := &cobra.Command{Use: "writ"}
				sub := &cobra.Command{Use: "repo"}
				leaf := &cobra.Command{Use: "init"}
				root.AddCommand(sub)
				sub.AddCommand(leaf)
				return leaf
			},
			outDir:   "dir",
			expected: filepath.Join("dir", "writ", "repo", "init.md"),
		},
		{
			name: "absolute output directory",
			setup: func() *cobra.Command {
				root := &cobra.Command{Use: "writ"}
				child := &cobra.Command{Use: "status"}
				root.AddCommand(child)
				return child
			},
			outDir:   filepath.Join("tmp", "docs"),
			expected: filepath.Join("tmp", "docs", "writ", "status.md"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.setup()
			got := outputPath(cmd, tt.outDir)
			if got != tt.expected {
				t.Errorf("outputPath() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestBuildPageData(t *testing.T) {
	root := &cobra.Command{
		Use:   "writ",
		Short: "manage workstation",
		Long:  "Manage workstation configuration and packages.",
	}
	root.PersistentFlags().BoolP("verbose", "v", false, "enable verbose output")

	child := &cobra.Command{
		Use:     "add [package]",
		Short:   "add a package",
		Long:    "Add a package to the workstation configuration.",
		Example: "  writ add vim\n  writ add --force neovim",
	}
	child.Flags().BoolP("force", "f", false, "force installation")
	root.AddCommand(child)

	grandchild := &cobra.Command{
		Use:   "hidden-cmd",
		Short: "hidden",
	}
	grandchild.Hidden = true
	root.AddCommand(grandchild)

	helpCmd := &cobra.Command{Use: "help", Short: "help about commands"}
	root.AddCommand(helpCmd)

	data := BuildPageData(child, "devlore", "1.2.3")

	// Title should be the full command name.
	if data.Title != "writ add" {
		t.Errorf("Title = %q, want %q", data.Title, "writ add")
	}

	// Description should be the Short field.
	if data.Description != "add a package" {
		t.Errorf("Description = %q, want %q", data.Description, "add a package")
	}

	// Tool should be the passed tool name.
	if data.Tool != "devlore" {
		t.Errorf("Tool = %q, want %q", data.Tool, "devlore")
	}

	// Command should be the command's own name.
	if data.Command != "add" {
		t.Errorf("Command = %q, want %q", data.Command, "add")
	}

	// Parent should be the full parent name.
	if data.Parent != "writ" {
		t.Errorf("Parent = %q, want %q", data.Parent, "writ")
	}

	// Version should match.
	if data.Version != "1.2.3" {
		t.Errorf("Version = %q, want %q", data.Version, "1.2.3")
	}

	// Long should be trimmed.
	if data.Long != "Add a package to the workstation configuration." {
		t.Errorf("Long = %q, want %q", data.Long, "Add a package to the workstation configuration.")
	}

	// Options should include the local --force flag.
	foundForce := false
	for _, opt := range data.Options {
		if opt.Name == "--force, -f" {
			foundForce = true
			if opt.Description != "force installation" {
				t.Errorf("--force description = %q, want %q", opt.Description, "force installation")
			}
		}
	}
	if !foundForce {
		t.Errorf("expected --force flag in Options, got %v", data.Options)
	}

	// GlobalFlags should include the inherited --verbose flag.
	foundVerbose := false
	for _, gf := range data.GlobalFlags {
		if gf.Name == "--verbose, -v" {
			foundVerbose = true
		}
	}
	if !foundVerbose {
		t.Errorf("expected --verbose flag in GlobalFlags, got %v", data.GlobalFlags)
	}

	// Examples should be trimmed per line.
	if data.Examples != "writ add vim\nwrit add --force neovim" {
		t.Errorf("Examples = %q, want %q", data.Examples, "writ add vim\nwrit add --force neovim")
	}

	// ParentCmd should reference the fsroot.
	if data.ParentCmd == nil {
		t.Fatal("expected ParentCmd to be non-nil")
	}
	if data.ParentCmd.Name != "writ" {
		t.Errorf("ParentCmd.ReceiverName = %q, want %q", data.ParentCmd.Name, "writ")
	}
	if data.ParentCmd.Path != "/cli/writ/" {
		t.Errorf("ParentCmd.Path = %q, want %q", data.ParentCmd.Path, "/cli/writ/")
	}

	// Now test fsroot command -- should have no ParentCmd and should list visible children.
	rootData := BuildPageData(root, "devlore", "1.2.3")

	if rootData.ParentCmd != nil {
		t.Errorf("expected fsroot ParentCmd to be nil, got %v", rootData.ParentCmd)
	}

	// Children should include "add" but not "hidden-cmd" or "help".
	childNames := make(map[string]bool)
	for _, c := range rootData.Children {
		childNames[c.Name] = true
	}
	if !childNames["writ add"] {
		t.Errorf("expected child 'writ add' in Children, got %v", rootData.Children)
	}
	if childNames["writ hidden-cmd"] {
		t.Error("hidden child should be excluded from Children")
	}
	if childNames["writ help"] {
		t.Error("help child should be excluded from Children")
	}
}

func TestFormatFlagName(t *testing.T) {
	tests := []struct {
		name     string
		flag     *pflag.Flag
		expected string
	}{
		{
			name: "flag with shorthand",
			flag: &pflag.Flag{
				Name:      "verbose",
				Shorthand: "v",
			},
			expected: "--verbose, -v",
		},
		{
			name: "flag without shorthand",
			flag: &pflag.Flag{
				Name: "output",
			},
			expected: "--output",
		},
		{
			name: "single-word flag with shorthand",
			flag: &pflag.Flag{
				Name:      "force",
				Shorthand: "f",
			},
			expected: "--force, -f",
		},
		{
			name: "hyphenated flag without shorthand",
			flag: &pflag.Flag{
				Name: "dry-run",
			},
			expected: "--dry-run",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatFlagName(tt.flag)
			if got != tt.expected {
				t.Errorf("formatFlagName() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestTrimExampleLines(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "single line no whitespace",
			input:    "writ add vim",
			expected: "writ add vim",
		},
		{
			name:     "leading and trailing whitespace",
			input:    "  writ add vim  ",
			expected: "writ add vim",
		},
		{
			name:     "multiline with leading spaces",
			input:    "  writ add vim\n  writ add neovim",
			expected: "writ add vim\nwrit add neovim",
		},
		{
			name:     "multiline with surrounding blank lines",
			input:    "\n  writ add vim\n  writ add neovim\n",
			expected: "writ add vim\nwrit add neovim",
		},
		{
			name:     "tabs and spaces mixed",
			input:    "\twrit add vim  \n\t  writ add neovim\t",
			expected: "writ add vim\nwrit add neovim",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := trimExampleLines(tt.input)
			if got != tt.expected {
				t.Errorf("trimExampleLines(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestCommandParts(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *cobra.Command
		expected []string
	}{
		{
			name: "fsroot command",
			setup: func() *cobra.Command {
				return &cobra.Command{Use: "writ"}
			},
			expected: []string{"writ"},
		},
		{
			name: "subcommand",
			setup: func() *cobra.Command {
				root := &cobra.Command{Use: "writ"}
				child := &cobra.Command{Use: "add"}
				root.AddCommand(child)
				return child
			},
			expected: []string{"writ", "add"},
		},
		{
			name: "deeply nested",
			setup: func() *cobra.Command {
				root := &cobra.Command{Use: "writ"}
				sub := &cobra.Command{Use: "repo"}
				leaf := &cobra.Command{Use: "init"}
				root.AddCommand(sub)
				sub.AddCommand(leaf)
				return leaf
			},
			expected: []string{"writ", "repo", "init"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := tt.setup()
			got := commandParts(cmd)
			if len(got) != len(tt.expected) {
				t.Fatalf("commandParts() returned %d parts, want %d: %v", len(got), len(tt.expected), got)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("commandParts()[%d] = %q, want %q", i, got[i], tt.expected[i])
				}
			}
		})
	}
}
