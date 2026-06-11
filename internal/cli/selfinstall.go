// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

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

	"github.com/NobleFactor/devlore-cli/schema"
)

// SelfInstallInfo contains metadata needed for self-installation.
type SelfInstallInfo struct {
	Name       string     // Tool name (e.g., "lore", "writ")
	ManHeader  ManHeader  // Man page header metadata
	ConfigInfo ConfigInfo // Config schema and defaults
}

// NewSelfInstallCmd creates the self-install command.
// Usage:
//
//	./tool self-install --prefix=~/.local
//	./tool self-install --prefix=~/.local --shell bash --shell zsh
//
// This performs complete installation:
//   - Copies binary to <fsroot>/bin/
//   - Installs man pages to <fsroot>/share/man/man1/ (if man command exists)
//   - Installs completions for detected shells (or specified via --shell)
//   - Initializes config in XDG_CONFIG_HOME/devlore/
//   - Initializes cache in XDG_CACHE_HOME/devlore/
//   - Creates layer directories in XDG_DATA_HOME/devlore/writ/layers/
func NewSelfInstallCmd(rootCmd *cobra.Command, info SelfInstallInfo) *cobra.Command {
	var shells []string
	var prefix string

	cmd := &cobra.Command{
		Use:   "self-install --prefix=<directory>",
		Short: "Complete installation to specified directory",
		Long: `Install ` + info.Name + ` and all supporting files to the specified prefix directory.

This command:
  1. Copies the binary to <prefix>/bin/` + info.Name + `
  2. Installs man pages to <prefix>/share/man/man1/ (if man command exists)
  3. Installs shell completions (auto-detects bash, fish, powershell, zsh or use --shell)
  4. Creates shared config at $XDG_CONFIG_HOME/devlore/config.yaml
  5. Creates tool config at $XDG_CONFIG_HOME/devlore/config.d/` + info.Name + `.yaml
  6. Initializes cache directory at $XDG_CACHE_HOME/devlore/
  7. Creates layer directories at $XDG_DATA_HOME/devlore/writ/layers/

Shell completions are auto-detected by default. Use --shell to override:
  ` + info.Name + ` self-install --prefix=~/.local --shell bash --shell zsh

Example:
  ` + info.Name + ` self-install --prefix=~/.local
  ` + info.Name + ` self-install --prefix=~/.local --shell bash --shell zsh

After installation, ensure <prefix>/bin is in your PATH.
`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if prefix == "" {
				return fmt.Errorf("--prefix is required")
			}
			root := expandTilde(prefix)

			return runSelfInstall(rootCmd, root, info, installFlags{
				Shells: shells,
			})
		},
	}

	cmd.Flags().StringVar(&prefix, "prefix", "", "Installation prefix directory (required, e.g., ~/.local)")
	cmd.Flags().StringArrayVar(&shells, "shell", nil, "Shell to install completions for (repeatable, e.g., --shell bash --shell zsh)")

	return cmd
}

// installFlags holds the flag values for self-install.
type installFlags struct {
	Shells []string // --shell (repeatable)
}

// detectShells returns a list of shells available on the system (alphabetically sorted).
func detectShells() []string {
	var shells []string

	// Check shells in alphabetical order
	if _, err := exec.LookPath("bash"); err == nil {
		shells = append(shells, "bash")
	}
	if _, err := exec.LookPath("fish"); err == nil {
		shells = append(shells, "fish")
	}
	// PowerShell: check for pwsh (cross-platform) or powershell (Windows)
	if _, err := exec.LookPath("pwsh"); err == nil {
		shells = append(shells, "powershell")
	} else if _, err := exec.LookPath("powershell"); err == nil {
		shells = append(shells, "powershell")
	}
	if _, err := exec.LookPath("zsh"); err == nil {
		shells = append(shells, "zsh")
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
//
//nolint:gocognit,gocyclo // orchestration function with sequential install steps
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
	Note("")
	for _, line := range installed {
		Note("  %s", line)
	}

	// Print PATH reminder
	binDir := filepath.Join(root, "bin")
	Note("Add %s to your PATH if not already present.", binDir)

	// Print shell completion setup instructions for installed shells
	printShellSetupInstructions(installedShells, info.Name)

	// 7. Create layer directories (writ only)
	if info.Name == "writ" {
		layerPaths, err := initWritLayers()
		if err != nil {
			return fmt.Errorf("failed to create layer directories: %w", err)
		}
		if len(layerPaths) > 0 {
			Note("")
			Note("Layer directories:")
			for _, p := range layerPaths {
				Note("  %s", p)
			}
		}
	}

	return nil
}

// initWritLayers creates the writ layer directories if they don't exist.
func initWritLayers() ([]string, error) {
	layersDir := WritLayersDir()
	var created []string

	for _, layer := range []string{"base", "team", "personal"} {
		layerPath := filepath.Join(layersDir, layer)
		if _, err := os.Stat(layerPath); os.IsNotExist(err) {
			if err := os.MkdirAll(layerPath, 0o750); err != nil {
				return created, err
			}
			created = append(created, layerPath)
		}
	}

	return created, nil
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
	if err := os.MkdirAll(binDir, 0o750); err != nil {
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
	if err := os.Chmod(targetPath, 0o750); err != nil { //nolint:gosec // G302: binary must be executable
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
	defer func() { _ = source.Close() }()

	dest, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination: %w", err)
	}
	defer func() { _ = dest.Close() }()

	if _, err := io.Copy(dest, source); err != nil {
		return fmt.Errorf("failed to copy: %w", err)
	}

	return nil
}

// installManPagesTo installs man pages and returns the list of installed files.
func installManPagesTo(rootCmd *cobra.Command, path string, header ManHeader) ([]string, error) {
	// Create directory
	if err := os.MkdirAll(path, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", path, err)
	}

	// Build header
	h := &doc.GenManHeader{
		Title:   header.Title,
		Section: header.Section,
		Date:    new(time.Now()),
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

// shellCompletionPath returns the installation path and filename for a shell's completion file.
func shellCompletionPath(shell, cmdName string) (relPath, filename string) {
	switch shell {
	case "bash":
		return filepath.Join("share", "bash-completion", "completions"), cmdName
	case "fish":
		return filepath.Join("share", "fish", "vendor_completions.d"), cmdName + ".fish"
	case "powershell":
		return filepath.Join("share", "powershell", "completions"), cmdName + ".ps1"
	case "zsh":
		return filepath.Join("share", "zsh", "site-functions"), "_" + cmdName
	default:
		return "", ""
	}
}

// installCompletionsForShells installs completions for the specified shells.
// Uses the same Gen* functions that Cobra's completion commands use internally.
func installCompletionsForShells(rootCmd *cobra.Command, root string, shells []string) ([]string, error) {
	var paths []string

	for _, shellName := range shells {
		relPath, filename := shellCompletionPath(shellName, rootCmd.Name())
		if relPath == "" {
			Warn("Unknown shell: %s (skipping)", shellName)
			continue
		}

		dir := filepath.Join(root, relPath)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return paths, fmt.Errorf("failed to create %s completion directory: %w", shellName, err)
		}

		fullPath := filepath.Join(dir, filename)
		f, err := os.Create(fullPath)
		if err != nil {
			return paths, fmt.Errorf("failed to create %s completion file: %w", shellName, err)
		}

		// Generate completions using the same functions Cobra's completion commands use
		var genErr error
		switch shellName {
		case "bash":
			genErr = rootCmd.GenBashCompletionV2(f, true) // true = include descriptions
		case "fish":
			genErr = rootCmd.GenFishCompletion(f, true)
		case "powershell":
			genErr = rootCmd.GenPowerShellCompletionWithDesc(f)
		case "zsh":
			genErr = rootCmd.GenZshCompletion(f)
		default:
			_ = f.Close()
			Warn("Unknown shell: %s (skipping)", shellName)
			continue
		}
		_ = f.Close()

		if genErr != nil {
			return paths, fmt.Errorf("failed to generate %s completion: %w", shellName, genErr)
		}

		paths = append(paths, fullPath)
	}

	return paths, nil
}

// printShellSetupInstructions prints setup instructions for installed shells.
func printShellSetupInstructions(shells []string, toolName string) {
	if len(shells) == 0 {
		return
	}

	Note("")
	Note("Shell completion setup:")

	for _, shell := range shells {
		switch shell {
		case "bash":
			Note("")
			Note("  For bash, ensure bash-completion is installed.")
		case "fish":
			Note("")
			Note("  For fish, completions work automatically.")
		case "powershell":
			Note("")
			Note("  For PowerShell, add to your $PROFILE:")
			Note("    . ~/.local/share/powershell/completions/%s.ps1", toolName)
		case "zsh":
			Note("")
			Note("  For zsh, add to ~/.zshrc:")
			Note("    fpath=(~/.local/share/zsh/site-functions $fpath)")
			Note("    autoload -Uz compinit && compinit")
		}
	}
}

// initDevloreConfig creates the unified devlore config structure.
// Returns paths to created config files.
func initDevloreConfig(info SelfInstallInfo) ([]string, error) {
	var paths []string

	// Create unified devlore config directory
	configDir := DevloreConfigHome()
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	// Create config.d directory for tool-specific configs
	configDDir := filepath.Join(configDir, "config.d")
	if err := os.MkdirAll(configDDir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create config.d directory: %w", err)
	}

	// Create shared config.yaml if it doesn't exist
	sharedConfigPath := filepath.Join(configDir, "config.yaml")
	if _, err := os.Stat(sharedConfigPath); os.IsNotExist(err) {
		if err := os.WriteFile(sharedConfigPath, schema.SharedDefaultConfig, 0o600); err != nil {
			return nil, fmt.Errorf("failed to write shared config: %w", err)
		}
	}
	paths = append(paths, sharedConfigPath)

	// Create tool-specific config in config.d/ if it doesn't exist
	toolConfigPath := filepath.Join(configDDir, info.Name+".yaml")
	if _, err := os.Stat(toolConfigPath); os.IsNotExist(err) {
		if err := os.WriteFile(toolConfigPath, info.ConfigInfo.DefaultConfig, 0o600); err != nil {
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

	if err := os.MkdirAll(cacheDir, 0o750); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	return cacheDir, nil
}
