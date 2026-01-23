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
//	./tool self-install ~/.local --personal-repo=~/dotfiles
//
// This performs complete installation:
//   - Copies binary to <root>/bin/
//   - Installs man pages to <root>/share/man/man1/
//   - Installs completions for bash, zsh, fish
//   - Initializes config in XDG_CONFIG_HOME
//   - Initializes cache in XDG_CACHE_HOME
//   - Scans and registers repos (if --personal-repo or --team-repo provided)
func NewSelfInstallCmd(rootCmd *cobra.Command, info SelfInstallInfo) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "self-install <root-directory>",
		Short: "Complete installation to specified directory",
		Long: `Install ` + info.Name + ` and all supporting files to the specified root directory.

This command:
  1. Copies the binary to <root>/bin/` + info.Name + `
  2. Installs man pages to <root>/share/man/man1/
  3. Installs shell completions for bash, zsh, and fish
  4. Creates default config at $XDG_CONFIG_HOME/` + info.Name + `/config.yaml
  5. Initializes cache directory at $XDG_CACHE_HOME/` + info.Name + `/
  6. Scans and registers repositories (if --personal-repo or --team-repo provided)

Repository flags accept a local path. The repo is scanned for writ compatibility
and migration guidance is provided if the structure doesn't match.

Example:
  ./` + info.Name + ` self-install ~/.local
  ./` + info.Name + ` self-install ~/.local --personal-repo=~/dotfiles
  ./` + info.Name + ` self-install ~/.local --personal-repo=~/dotfiles --team-repo=~/work/configs

After installation, ensure <root>/bin is in your PATH.
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := args[0]
			root = expandTilde(root)

			personalRepo, _ := cmd.Flags().GetString("personal-repo")
			teamRepo, _ := cmd.Flags().GetString("team-repo")

			return runSelfInstall(rootCmd, root, info, repoFlags{
				Personal: expandTilde(personalRepo),
				Team:     expandTilde(teamRepo),
			})
		},
	}

	cmd.Flags().String("personal-repo", "", "Path to personal environment repository")
	cmd.Flags().String("team-repo", "", "Path to team environment repository")

	return cmd
}

// repoFlags holds the --personal-repo and --team-repo flag values.
type repoFlags struct {
	Personal string
	Team     string
}

// expandTilde expands ~ to $HOME in a path.
func expandTilde(path string) string {
	if path == "" {
		return ""
	}
	if len(path) >= 2 && path[:2] == "~/" {
		return filepath.Join(os.Getenv("HOME"), path[2:])
	}
	if path == "~" {
		return os.Getenv("HOME")
	}
	return path
}

// runSelfInstall performs the complete installation.
func runSelfInstall(rootCmd *cobra.Command, root string, info SelfInstallInfo, repos repoFlags) error {
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
	Success("Installed %s to %s", info.Name, root)
	fmt.Fprintf(os.Stderr, "\n")
	for _, line := range installed {
		Note("  %s", line)
	}

	// Print PATH reminder
	binDir := filepath.Join(root, "bin")
	Note("Add %s to your PATH if not already present.", binDir)

	// 6. Scan and register repos
	if repos.Personal != "" {
		fmt.Fprintf(os.Stderr, "\n")
		scanAndRegisterRepo(repos.Personal, "personal", info.Name)
	}
	if repos.Team != "" {
		fmt.Fprintf(os.Stderr, "\n")
		scanAndRegisterRepo(repos.Team, "team", info.Name)
	}

	return nil
}

// scanAndRegisterRepo scans a repository path, reports results, and registers
// it in the shared config. If migration is needed, guidance is printed but the
// repo is still registered so the user can migrate and re-run self-install to verify.
func scanAndRegisterRepo(path, layer, tool string) {
	result := ScanRepo(path)
	result.PrintReport()

	// Register the repo regardless of migration status
	if err := RegisterRepo(tool, RepoEntry{
		Layer: layer,
		Path:  path,
		URL:   result.Remote,
	}); err != nil {
		Error("Failed to register %s repo: %v", layer, err)
		return
	}

	if result.NeedsMigration() {
		Warn("Registered %s repo despite migration needs — fix and re-run self-install to verify", layer)
	} else {
		Success("Registered %s repo: %s", layer, path)
	}
}

// installBinary copies the current executable to the target location.
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

	// Copy binary (preserve the build artifact)
	if err := copyFile(currentExe, targetPath); err != nil {
		return "", err
	}

	// Make executable
	if err := os.Chmod(targetPath, 0755); err != nil {
		return "", fmt.Errorf("failed to make executable: %w", err)
	}

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
