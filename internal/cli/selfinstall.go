// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
)

// SelfInstallInfo contains metadata needed for self-installation.
type SelfInstallInfo struct {
	Name          string     // Tool name (e.g., "lore", "writ")
	ManHeader     ManHeader  // Man page header metadata
	ConfigInfo    ConfigInfo // Config schema and defaults
}

// NewSelfInstallCmd creates the self-install command.
// Usage:
//
//	./tool self-install ~/.local
//
// This performs complete installation:
//   - Moves binary to <root>/bin/
//   - Installs man pages to <root>/share/man/man1/
//   - Installs completions for bash, zsh, fish
//   - Initializes config in XDG_CONFIG_HOME
//   - Initializes cache in XDG_CACHE_HOME
func NewSelfInstallCmd(rootCmd *cobra.Command, info SelfInstallInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "self-install <root-directory>",
		Short: "Complete installation to specified directory",
		Long: `Install ` + info.Name + ` and all supporting files to the specified root directory.

This command:
  1. Moves the binary to <root>/bin/` + info.Name + `
  2. Installs man pages to <root>/share/man/man1/
  3. Installs shell completions for bash, zsh, and fish
  4. Creates default config at $XDG_CONFIG_HOME/` + info.Name + `/config.yaml
  5. Initializes cache directory at $XDG_CACHE_HOME/` + info.Name + `/

Example:
  ./` + info.Name + ` self-install ~/.local

After installation, ensure <root>/bin is in your PATH.
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := args[0]

			// Expand ~ if present
			if len(root) >= 2 && root[:2] == "~/" {
				home := os.Getenv("HOME")
				root = filepath.Join(home, root[2:])
			} else if root == "~" {
				root = os.Getenv("HOME")
			}

			return runSelfInstall(rootCmd, root, info)
		},
	}
}

// runSelfInstall performs the complete installation.
func runSelfInstall(rootCmd *cobra.Command, root string, info SelfInstallInfo) error {
	var installed []string

	// 1. Install binary
	binPath, err := installBinary(root, info.Name)
	if err != nil {
		return fmt.Errorf("failed to install binary: %w", err)
	}
	installed = append(installed, fmt.Sprintf("Binary:      %s", binPath))

	// 2. Install man pages
	manPath := filepath.Join(root, "share", "man", "man1")
	manFiles, err := installManPagesTo(rootCmd, manPath, info.ManHeader)
	if err != nil {
		return fmt.Errorf("failed to install man pages: %w", err)
	}
	for _, f := range manFiles {
		installed = append(installed, fmt.Sprintf("Man page:    %s", f))
	}

	// 3. Install completions (all three shells)
	completionPaths, err := installAllCompletions(rootCmd, root)
	if err != nil {
		return fmt.Errorf("failed to install completions: %w", err)
	}
	for _, p := range completionPaths {
		installed = append(installed, fmt.Sprintf("Completion:  %s", p))
	}

	// 4. Initialize config
	configPath, err := initConfig(info)
	if err != nil {
		return fmt.Errorf("failed to initialize config: %w", err)
	}
	installed = append(installed, fmt.Sprintf("Config:      %s", configPath))

	// 5. Initialize cache
	cachePath, err := initCache(info.Name)
	if err != nil {
		return fmt.Errorf("failed to initialize cache: %w", err)
	}
	installed = append(installed, fmt.Sprintf("Cache:       %s", cachePath))

	// Print summary
	fmt.Printf("Installed %s to %s\n\n", info.Name, root)
	for _, line := range installed {
		fmt.Printf("  %s\n", line)
	}

	// Print PATH reminder
	binDir := filepath.Join(root, "bin")
	fmt.Printf("\nAdd %s to your PATH if not already present.\n", binDir)

	return nil
}

// installBinary moves the current executable to the target location.
func installBinary(root, name string) (string, error) {
	// Get current executable path
	currentExe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	// Resolve symlinks
	currentExe, err = filepath.EvalSymlinks(currentExe)
	if err != nil {
		return "", fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	// Target path
	binDir := filepath.Join(root, "bin")
	targetPath := filepath.Join(binDir, name)

	// Create bin directory
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", binDir, err)
	}

	// If source and target are the same, skip
	if currentExe == targetPath {
		return targetPath, nil
	}

	// Copy binary (don't move, in case of failure)
	if err := copyFile(currentExe, targetPath); err != nil {
		return "", err
	}

	// Make executable
	if err := os.Chmod(targetPath, 0755); err != nil {
		return "", fmt.Errorf("failed to make executable: %w", err)
	}

	// Remove original (we copied successfully)
	// Ignore errors here - the original might be read-only or in use
	_ = os.Remove(currentExe)

	return targetPath, nil
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source: %w", err)
	}
	defer source.Close()

	dest, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination: %w", err)
	}
	defer dest.Close()

	if _, err := io.Copy(dest, source); err != nil {
		return fmt.Errorf("failed to copy: %w", err)
	}

	return nil
}

// installManPagesTo installs man pages and returns the list of installed files.
func installManPagesTo(rootCmd *cobra.Command, path string, header ManHeader) ([]string, error) {
	// Create directory
	if err := os.MkdirAll(path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", path, err)
	}

	// Build header
	now := time.Now()
	h := &doc.GenManHeader{
		Title:   header.Title,
		Section: header.Section,
		Date:    &now,
		Source:  header.Source,
		Manual:  header.Manual,
	}

	// Generate man pages
	if err := doc.GenManTree(rootCmd, h, path); err != nil {
		return nil, fmt.Errorf("failed to generate man pages: %w", err)
	}

	// List generated files
	var files []string
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, filepath.Join(path, e.Name()))
		}
	}

	return files, nil
}

// installAllCompletions installs completions for bash, zsh, and fish.
func installAllCompletions(rootCmd *cobra.Command, root string) ([]string, error) {
	var paths []string

	shells := []struct {
		name     string
		relPath  string
		filename string
		genFunc  func(*cobra.Command, *os.File) error
	}{
		{
			name:     "bash",
			relPath:  filepath.Join("share", "bash-completion", "completions"),
			filename: rootCmd.Name(),
			genFunc:  func(cmd *cobra.Command, f *os.File) error { return cmd.GenBashCompletion(f) },
		},
		{
			name:     "zsh",
			relPath:  filepath.Join("share", "zsh", "site-functions"),
			filename: "_" + rootCmd.Name(),
			genFunc:  func(cmd *cobra.Command, f *os.File) error { return cmd.GenZshCompletion(f) },
		},
		{
			name:     "fish",
			relPath:  filepath.Join("share", "fish", "vendor_completions.d"),
			filename: rootCmd.Name() + ".fish",
			genFunc:  func(cmd *cobra.Command, f *os.File) error { return cmd.GenFishCompletion(f, true) },
		},
	}

	for _, shell := range shells {
		dir := filepath.Join(root, shell.relPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return paths, fmt.Errorf("failed to create %s completion directory: %w", shell.name, err)
		}

		fullPath := filepath.Join(dir, shell.filename)
		f, err := os.Create(fullPath)
		if err != nil {
			return paths, fmt.Errorf("failed to create %s completion file: %w", shell.name, err)
		}

		if err := shell.genFunc(rootCmd, f); err != nil {
			f.Close()
			return paths, fmt.Errorf("failed to generate %s completion: %w", shell.name, err)
		}
		f.Close()

		paths = append(paths, fullPath)
	}

	return paths, nil
}

// initConfig creates the default config file if it doesn't exist.
func initConfig(info SelfInstallInfo) (string, error) {
	configDir := filepath.Join(ConfigHome(), info.Name)
	configPath := filepath.Join(configDir, "config.yaml")

	// Create directory
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create config directory: %w", err)
	}

	// Only create if doesn't exist
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := os.WriteFile(configPath, info.ConfigInfo.DefaultConfig, 0644); err != nil {
			return "", fmt.Errorf("failed to write config: %w", err)
		}
	}

	return configPath, nil
}

// initCache creates the cache directory.
func initCache(name string) (string, error) {
	cacheDir := filepath.Join(CacheHome(), name)

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	return cacheDir, nil
}
