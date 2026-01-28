// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
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
//	./tool self-install ~/.local --shell bash --shell zsh
//	./tool self-install ~/.local --personal-repo=~/dotfiles
//
// This performs complete installation:
//   - Copies binary to <root>/bin/
//   - Installs man pages to <root>/share/man/man1/ (if man command exists)
//   - Installs completions for detected shells (or specified via --shell)
//   - Initializes config in XDG_CONFIG_HOME/devlore/
//   - Initializes cache in XDG_CACHE_HOME/devlore/
//   - Scans and registers repos (if --personal-repo or --team-repo provided)
func NewSelfInstallCmd(rootCmd *cobra.Command, info SelfInstallInfo) *cobra.Command {
	var shells []string

	cmd := &cobra.Command{
		Use:   "self-install <root-directory>",
		Short: "Complete installation to specified directory",
		Long: `Install ` + info.Name + ` and all supporting files to the specified root directory.

This command:
  1. Copies the binary to <root>/bin/` + info.Name + `
  2. Installs man pages to <root>/share/man/man1/ (if man command exists)
  3. Installs shell completions (auto-detects bash, zsh, fish or use --shell)
  4. Creates shared config at $XDG_CONFIG_HOME/devlore/config.yaml
  5. Creates tool config at $XDG_CONFIG_HOME/devlore/config.d/` + info.Name + `.yaml
  6. Initializes cache directory at $XDG_CACHE_HOME/devlore/` + info.Name + `/
  7. Scans and registers repositories (if --personal-repo or --team-repo provided)

Shell completions are auto-detected by default. Use --shell to override:
  ./` + info.Name + ` self-install ~/.local --shell bash --shell zsh

Repository flags accept a local path. The repo is scanned for writ compatibility
and migration guidance is provided if the structure doesn't match.

Example:
  ./` + info.Name + ` self-install ~/.local
  ./` + info.Name + ` self-install ~/.local --shell bash --shell zsh
  ./` + info.Name + ` self-install ~/.local --personal-repo=~/dotfiles

After installation, ensure <root>/bin is in your PATH.
`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := args[0]
			root = expandTilde(root)

			personalRepo, _ := cmd.Flags().GetString("personal-repo")
			teamRepo, _ := cmd.Flags().GetString("team-repo")

			return runSelfInstall(rootCmd, root, info, installFlags{
				Personal: expandTilde(personalRepo),
				Team:     expandTilde(teamRepo),
				Shells:   shells,
			})
		},
	}

	cmd.Flags().String("personal-repo", "", "Path to personal environment repository")
	cmd.Flags().String("team-repo", "", "Path to team environment repository")
	cmd.Flags().StringArrayVar(&shells, "shell", nil, "Shell to install completions for (repeatable, e.g., --shell bash --shell zsh)")

	return cmd
}

// installFlags holds the flag values for self-install.
type installFlags struct {
	Personal string   // --personal-repo
	Team     string   // --team-repo
	Shells   []string // --shell (repeatable)
}

// detectShells returns a list of shells available on the system.
func detectShells() []string {
	var shells []string
	for _, shell := range []string{"bash", "zsh", "fish"} {
		if _, err := exec.LookPath(shell); err == nil {
			shells = append(shells, shell)
		}
	}
	return shells
}

// hasMan returns true if the man command is available.
func hasMan() bool {
	_, err := exec.LookPath("man")
	return err == nil
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
func runSelfInstall(rootCmd *cobra.Command, root string, info SelfInstallInfo, flags installFlags) error {
	var installed []string
	var installedShells []string

	// 1. Install binary
	binPath, err := installBinary(root, info.Name)
	if err != nil {
		return fmt.Errorf("failed to install binary: %w", err)
	}
	installed = append(installed, fmt.Sprintf("Binary:      %s", binPath))

	// 2. Install man pages (if man command exists)
	if hasMan() {
		manPath := filepath.Join(root, "share", "man", "man1")
		manFiles, err := installManPagesTo(rootCmd, manPath, info.ManHeader)
		if err != nil {
			return fmt.Errorf("failed to install man pages: %w", err)
		}
		for _, f := range manFiles {
			installed = append(installed, fmt.Sprintf("Man page:    %s", f))
		}
	} else {
		Note("Skipping man pages (man command not found)")
	}

	// 3. Determine which shells to install completions for
	shells := flags.Shells
	if len(shells) == 0 {
		// Auto-detect available shells
		shells = detectShells()
		if len(shells) == 0 {
			Note("No shells detected for completions")
		}
	}

	// 4. Install completions for selected shells
	if len(shells) > 0 {
		completionPaths, err := installCompletionsForShells(rootCmd, root, shells)
		if err != nil {
			return fmt.Errorf("failed to install completions: %w", err)
		}
		for _, p := range completionPaths {
			installed = append(installed, fmt.Sprintf("Completion:  %s", p))
		}
		installedShells = shells
	}

	// 5. Initialize config (unified devlore namespace)
	configPaths, err := initDevloreConfig(info)
	if err != nil {
		return fmt.Errorf("failed to initialize config: %w", err)
	}
	for _, p := range configPaths {
		installed = append(installed, fmt.Sprintf("Config:      %s", p))
	}

	// 6. Initialize cache (unified devlore namespace)
	cachePath, err := initDevloreCache(info.Name)
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

	// Print shell completion setup instructions for installed shells
	printShellSetupInstructions(installedShells)

	// 7. Scan and register repos
	if flags.Personal != "" {
		fmt.Fprintf(os.Stderr, "\n")
		scanAndRegisterRepo(flags.Personal, "personal", info.Name)
	}
	if flags.Team != "" {
		fmt.Fprintf(os.Stderr, "\n")
		scanAndRegisterRepo(flags.Team, "team", info.Name)
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

// shellConfig defines how to install completions for each shell.
type shellConfig struct {
	name     string
	relPath  string
	filename string
	genFunc  func(*cobra.Command, *os.File) error
}

// getShellConfigs returns the configuration for all supported shells.
func getShellConfigs(cmdName string) map[string]shellConfig {
	return map[string]shellConfig{
		"bash": {
			name:     "bash",
			relPath:  filepath.Join("share", "bash-completion", "completions"),
			filename: cmdName,
			genFunc:  func(cmd *cobra.Command, f *os.File) error { return cmd.GenBashCompletion(f) },
		},
		"zsh": {
			name:     "zsh",
			relPath:  filepath.Join("share", "zsh", "site-functions"),
			filename: "_" + cmdName,
			genFunc:  func(cmd *cobra.Command, f *os.File) error { return cmd.GenZshCompletion(f) },
		},
		"fish": {
			name:     "fish",
			relPath:  filepath.Join("share", "fish", "vendor_completions.d"),
			filename: cmdName + ".fish",
			genFunc:  func(cmd *cobra.Command, f *os.File) error { return cmd.GenFishCompletion(f, true) },
		},
	}
}

// installCompletionsForShells installs completions for the specified shells.
func installCompletionsForShells(rootCmd *cobra.Command, root string, shells []string) ([]string, error) {
	var paths []string
	configs := getShellConfigs(rootCmd.Name())

	for _, shellName := range shells {
		shell, ok := configs[shellName]
		if !ok {
			Warn("Unknown shell: %s (skipping)", shellName)
			continue
		}

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

// printShellSetupInstructions prints setup instructions for installed shells.
func printShellSetupInstructions(shells []string) {
	if len(shells) == 0 {
		return
	}

	fmt.Fprintf(os.Stderr, "\n")
	Note("Shell completion setup:")

	for _, shell := range shells {
		switch shell {
		case "zsh":
			fmt.Fprintf(os.Stderr, "\n  For zsh, add to ~/.zshrc:\n")
			fmt.Fprintf(os.Stderr, "    fpath=(~/.local/share/zsh/site-functions $fpath)\n")
			fmt.Fprintf(os.Stderr, "    autoload -Uz compinit && compinit\n")
		case "bash":
			fmt.Fprintf(os.Stderr, "\n  For bash, ensure bash-completion is installed.\n")
		case "fish":
			fmt.Fprintf(os.Stderr, "\n  For fish, completions work automatically.\n")
		}
	}
}

// initDevloreConfig creates the unified devlore config structure.
// Returns paths to created config files.
func initDevloreConfig(info SelfInstallInfo) ([]string, error) {
	var paths []string

	// Create unified devlore config directory
	configDir := DevloreConfigHome()
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	// Create config.d directory for tool-specific configs
	configDDir := filepath.Join(configDir, "config.d")
	if err := os.MkdirAll(configDDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config.d directory: %w", err)
	}

	// Create shared config.yaml if it doesn't exist
	sharedConfigPath := filepath.Join(configDir, "config.yaml")
	if _, err := os.Stat(sharedConfigPath); os.IsNotExist(err) {
		sharedConfig := []byte("# DevLore shared configuration\n# Shared settings for writ and lore\n")
		if err := os.WriteFile(sharedConfigPath, sharedConfig, 0644); err != nil {
			return nil, fmt.Errorf("failed to write shared config: %w", err)
		}
	}
	paths = append(paths, sharedConfigPath)

	// Create tool-specific config in config.d/ if it doesn't exist
	toolConfigPath := filepath.Join(configDDir, info.Name+".yaml")
	if _, err := os.Stat(toolConfigPath); os.IsNotExist(err) {
		if err := os.WriteFile(toolConfigPath, info.ConfigInfo.DefaultConfig, 0644); err != nil {
			return nil, fmt.Errorf("failed to write %s config: %w", info.Name, err)
		}
	}
	paths = append(paths, toolConfigPath)

	return paths, nil
}

// initDevloreCache creates the unified devlore cache structure.
func initDevloreCache(toolName string) (string, error) {
	// Create unified devlore cache directory with tool subdirectory
	cacheDir := filepath.Join(DevloreCacheHome(), toolName)

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	return cacheDir, nil
}
