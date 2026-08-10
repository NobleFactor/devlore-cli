// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package cli

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"

	"github.com/NobleFactor/devlore-cli/schema"
)

// =============================================================================
// Types
// =============================================================================

// SelfInstallInfo contains metadata needed for self-installation.
type SelfInstallInfo struct {
	Name               string                  // Tool name (e.g., "lore", "writ", "star")
	Version            string                  // Semantic version (e.g., "0.4.0"), set via ldflags
	ManHeader          ManHeader               // Man page header metadata
	ConfigInfo         *ConfigInfo             // Config schema and defaults (nil to skip config init)
	PostInstallHooks   []func(string) []string // Hooks run after install; return installed file paths (relative to prefix)
	PostUninstallHooks []func(string) error    // Hooks run after uninstall
}

// manifest records every file installed by self install/upgrade.
type manifest struct {
	Tool      string          `json:"tool"`
	Version   string          `json:"version"`
	Prefix    string          `json:"prefix"`
	Installed string          `json:"installed"`
	Files     []manifestEntry `json:"files"`
}

// manifestEntry records one installed file.
type manifestEntry struct {
	Path   string `json:"path"`   // Relative to prefix
	SHA256 string `json:"sha256"` // Hex-encoded SHA-256
}

// installFlags holds the flag values for self install.
type installFlags struct {
	Shells []string
}

// =============================================================================
// Command Construction
// =============================================================================

// NewSelfCmd creates the "self" command group with install, upgrade, and uninstall subcommands.
func NewSelfCmd(rootCmd *cobra.Command, info SelfInstallInfo) *cobra.Command {
	selfCmd := &cobra.Command{
		Use:   "self",
		Short: "Self-management commands",
	}

	selfCmd.AddCommand(newInstallCmd(rootCmd, info))
	selfCmd.AddCommand(newUpgradeCmd(rootCmd, info))
	selfCmd.AddCommand(newUninstallCmd(rootCmd, info))

	return selfCmd
}

// newInstallCmd creates the "self install" subcommand.
func newInstallCmd(rootCmd *cobra.Command, info SelfInstallInfo) *cobra.Command {
	var shells []string

	cmd := &cobra.Command{
		Use:   "install [prefix]",
		Short: "Install " + info.Name + " and supporting files",
		Long: `Install ` + info.Name + ` and all supporting files to the specified prefix directory.

This command:
  1. Copies the binary to <prefix>/bin/` + info.Name + `
  2. Installs man pages to <prefix>/share/man/man1/ (if man command exists)
  3. Installs shell completions (auto-detects bash, fish, pwsh, zsh or use --shell)
  4. Initializes config and cache directories (if applicable)
  5. Writes a manifest for uninstall tracking

Example:
  ` + info.Name + ` self install           # defaults to ~/.local
  ` + info.Name + ` self install ~/.local
  ` + info.Name + ` self install /usr/local --shell bash --shell zsh

After installation, ensure <prefix>/bin is in your PATH.
`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			prefix := defaultPrefix()
			if len(args) > 0 {
				prefix = expandTilde(args[0])
			}
			return runSelfInstall(rootCmd, prefix, info, installFlags{Shells: shells})
		},
	}

	cmd.Flags().StringArrayVar(&shells, "shell", nil,
		"Shell to install completions for (repeatable, e.g., --shell bash --shell zsh)")

	return cmd
}

// newUpgradeCmd creates the "self upgrade" subcommand.
func newUpgradeCmd(rootCmd *cobra.Command, info SelfInstallInfo) *cobra.Command {
	var shells []string

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade " + info.Name + " in place",
		Long: `Upgrade ` + info.Name + ` by overwriting the currently installed binary and refreshing
man pages, completions, config, and cache.

The installation prefix is resolved from the running binary's location.
No prefix argument is needed.

Example:
  ` + info.Name + ` self upgrade
`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			prefix, err := resolveInstalledPrefix(info.Name)
			if err != nil {
				return err
			}
			return runSelfInstall(rootCmd, prefix, info, installFlags{Shells: shells})
		},
	}

	cmd.Flags().StringArrayVar(&shells, "shell", nil,
		"Shell to install completions for (repeatable, e.g., --shell bash --shell zsh)")

	return cmd
}

// newUninstallCmd creates the "self uninstall" subcommand.
func newUninstallCmd(_ *cobra.Command, info SelfInstallInfo) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "uninstall [prefix]",
		Short: "Remove " + info.Name + " and supporting files",
		Long: `Remove ` + info.Name + ` and all files installed by "self install".

Reads the installation manifest and removes only files that have not been
modified since installation. Modified files are skipped and reported.
Empty directories left behind are cleaned up.

Example:
  ` + info.Name + ` self uninstall           # resolves prefix from binary location
  ` + info.Name + ` self uninstall ~/.local
  ` + info.Name + ` self uninstall --force    # skip confirmation prompt
`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			var prefix string
			if len(args) > 0 {
				prefix = expandTilde(args[0])
			} else {
				resolved, err := resolveInstalledPrefix(info.Name)
				if err != nil {
					return err
				}
				prefix = resolved
			}

			if !force {
				Note("This will remove %s from %s.", info.Name, prefix)
				Note("Modified files will be preserved.")
				fmt.Print("Continue? [y/N] ")
				reader := bufio.NewReader(os.Stdin)
				answer, err := reader.ReadString('\n')
				if err != nil {
					return fmt.Errorf("failed to read input: %w", err)
				}
				answer = strings.TrimSpace(strings.ToLower(answer))
				if answer != "y" && answer != "yes" {
					Note("Aborted.")
					return nil
				}
			}

			return runSelfUninstall(prefix, info)
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Skip confirmation prompt")

	return cmd
}

// =============================================================================
// Install / Upgrade
// =============================================================================

// runSelfInstall performs the complete installation.
//
//nolint:gocognit,gocyclo // orchestration function with sequential install steps
func runSelfInstall(rootCmd *cobra.Command, prefix string, info SelfInstallInfo, flags installFlags) error {
	var installed []string     // Display lines
	var manifestFiles []string // Paths relative to prefix (for manifest)

	// 1. Install binary.
	binPath, err := installBinary(prefix, info.Name)
	if err != nil {
		return fmt.Errorf("failed to install binary: %w", err)
	}
	installed = append(installed, fmt.Sprintf("Binary:      %s", binPath))
	manifestFiles = append(manifestFiles, relPath(prefix, binPath))

	// 2. Install man pages (if man command exists).
	if hasMan() {
		manPath := filepath.Join(prefix, "share", "man", "man1")
		manFiles, err := installManPagesTo(rootCmd, manPath, info.ManHeader)
		if err != nil {
			return fmt.Errorf("failed to install man pages: %w", err)
		}
		for _, f := range manFiles {
			installed = append(installed, fmt.Sprintf("Man page:    %s", f))
			manifestFiles = append(manifestFiles, relPath(prefix, f))
		}
	} else {
		Note("Skipping man pages (man command not found)")
	}

	// 3. Determine which shells to install completions for.
	shells := flags.Shells
	if len(shells) == 0 {
		shells = detectShells()
		if len(shells) == 0 {
			Note("No shells detected for completions")
		}
	}

	// 4. Install completions for selected shells.
	var installedShells []string
	if len(shells) > 0 {
		completionPaths, err := installCompletionsForShells(rootCmd, prefix, shells)
		if err != nil {
			return fmt.Errorf("failed to install completions: %w", err)
		}
		for _, p := range completionPaths {
			installed = append(installed, fmt.Sprintf("Completion:  %s", p))
			manifestFiles = append(manifestFiles, relPath(prefix, p))
		}
		installedShells = shells
	}

	// 5. Initialize config (if tool has config).
	if info.ConfigInfo != nil {
		configPaths, err := initDevloreConfig(info)
		if err != nil {
			return fmt.Errorf("failed to initialize config: %w", err)
		}
		for _, p := range configPaths {
			installed = append(installed, fmt.Sprintf("Config:      %s", p))
		}

		// 6. Initialize cache.
		cachePath, err := initDevloreCache(info.Name)
		if err != nil {
			return fmt.Errorf("failed to initialize cache: %w", err)
		}
		installed = append(installed, fmt.Sprintf("Cache:       %s", cachePath))
	}

	// 7. Run post-install hooks (e.g., star extensions).
	for _, hook := range info.PostInstallHooks {
		hookFiles := hook(prefix)
		for _, f := range hookFiles {
			installed = append(installed, fmt.Sprintf("Extension:   %s", filepath.Join(prefix, f)))
			manifestFiles = append(manifestFiles, f)
		}
	}

	// 8. Create writ layer directories.
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

	// 9. Write manifest.
	if err := writeManifest(prefix, info.Name, info.Version, manifestFiles); err != nil {
		Warn("Failed to write manifest: %v", err)
	}

	// Print summary.
	Success("Installed %s to %s", info.Name, prefix)
	Note("")
	for _, line := range installed {
		Note("  %s", line)
	}

	binDir := filepath.Join(prefix, "bin")
	Note("")
	Note("Add %s to your PATH if not already present.", binDir)
	printShellSetupInstructions(installedShells, info.Name)

	return nil
}

// =============================================================================
// Uninstall
// =============================================================================

// runSelfUninstall removes files recorded in the manifest.
//
//nolint:gocognit // orchestration function with sequential uninstall steps
func runSelfUninstall(prefix string, info SelfInstallInfo) error {
	m, err := readManifest(prefix, info.Name)
	if err != nil {
		return fmt.Errorf("no manifest found at %s — was %s installed with 'self install'? (%w)",
			manifestPath(prefix, info.Name), info.Name, err)
	}

	var removed, skipped []string

	for _, entry := range m.Files {
		absPath := filepath.Join(prefix, entry.Path)

		currentHash, err := fileSHA256(absPath)
		if err != nil {
			// File already gone — that's fine.
			if os.IsNotExist(err) {
				continue
			}
			Warn("Cannot read %s: %v (skipping)", absPath, err)
			skipped = append(skipped, absPath)
			continue
		}

		if currentHash != entry.SHA256 {
			skipped = append(skipped, absPath)
			continue
		}

		if err := os.Remove(absPath); err != nil {
			Warn("Failed to remove %s: %v", absPath, err)
			skipped = append(skipped, absPath)
			continue
		}
		removed = append(removed, absPath)
	}

	// Clean up empty directories left behind.
	cleanEmptyDirs(prefix, m.Files)

	// Remove the manifest itself (best-effort).
	mPath := manifestPath(prefix, info.Name)
	_ = os.Remove(mPath)                   //nolint:errcheck // best-effort cleanup
	_ = removeIfEmpty(filepath.Dir(mPath)) //nolint:errcheck // best-effort cleanup

	// Run post-uninstall hooks.
	for _, hook := range info.PostUninstallHooks {
		if err := hook(prefix); err != nil {
			Warn("Post-uninstall hook failed: %v", err)
		}
	}

	// Remove config and cache (XDG directories).
	if info.ConfigInfo != nil {
		removeDevloreConfig(info.Name)
		removeDevloreCache(info.Name)
	}

	// Print summary.
	Success("Uninstalled %s from %s", info.Name, prefix)
	if len(removed) > 0 {
		Note("")
		Note("Removed %d file(s):", len(removed))
		for _, f := range removed {
			Note("  %s", f)
		}
	}
	if len(skipped) > 0 {
		Note("")
		Note("Skipped %d modified file(s):", len(skipped))
		for _, f := range skipped {
			Note("  %s", f)
		}
	}

	return nil
}

// removeDevloreConfig removes tool-specific config from the XDG config directory.
// The shared config.yaml is left alone — other tools may use it.
func removeDevloreConfig(toolName string) {
	configDir := DevloreConfigHome()
	toolConfig := filepath.Join(configDir, "config.d", toolName+".yaml")
	if err := os.Remove(toolConfig); err != nil && !os.IsNotExist(err) {
		Warn("Failed to remove config %s: %v", toolConfig, err)
	}
	_ = removeIfEmpty(filepath.Join(configDir, "config.d")) //nolint:errcheck // best-effort cleanup
}

// removeDevloreCache removes the tool's cache directory.
func removeDevloreCache(toolName string) {
	cacheDir := filepath.Join(DevloreCacheHome(), toolName)
	if err := os.RemoveAll(cacheDir); err != nil && !os.IsNotExist(err) {
		Warn("Failed to remove cache %s: %v", cacheDir, err)
	}
	_ = removeIfEmpty(DevloreCacheHome()) //nolint:errcheck // best-effort cleanup
}

// =============================================================================
// Manifest
// =============================================================================

// manifestPath returns the path to the manifest file.
func manifestPath(prefix, toolName string) string {
	return filepath.Join(prefix, "share", toolName, "manifest.json")
}

// writeManifest writes the installation manifest.
func writeManifest(prefix, toolName, version string, relativePaths []string) error {
	var entries []manifestEntry
	for _, rel := range relativePaths {
		absPath := filepath.Join(prefix, rel)
		hash, err := fileSHA256(absPath)
		if err != nil {
			// File may not exist (e.g., skipped man pages). Skip silently.
			continue
		}
		entries = append(entries, manifestEntry{Path: rel, SHA256: hash})
	}

	m := manifest{
		Tool:      toolName,
		Version:   version,
		Prefix:    prefix,
		Installed: time.Now().UTC().Format(time.RFC3339),
		Files:     entries,
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	mPath := manifestPath(prefix, toolName)
	if err := os.MkdirAll(filepath.Dir(mPath), 0o750); err != nil {
		return err
	}

	return os.WriteFile(mPath, append(data, '\n'), 0o600)
}

// readManifest reads the installation manifest.
func readManifest(prefix, toolName string) (*manifest, error) {
	data, err := os.ReadFile(manifestPath(prefix, toolName))
	if err != nil {
		return nil, err
	}

	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// =============================================================================
// Prefix Resolution
// =============================================================================

// defaultPrefix returns the default installation prefix (~/.local).
func defaultPrefix() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.Getenv("HOME"), ".local")
	}
	return filepath.Join(home, ".local")
}

// resolveInstalledPrefix determines the installation prefix from the running binary's
// location. For a binary at <prefix>/bin/<tool>, this returns <prefix>.
func resolveInstalledPrefix(toolName string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("cannot determine executable path: %w", err)
	}

	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return "", fmt.Errorf("cannot resolve symlinks: %w", err)
	}

	// Expect <prefix>/bin/<tool>
	dir := filepath.Dir(exe)   // <prefix>/bin
	base := filepath.Base(dir) // bin
	if base != "bin" {
		return "", fmt.Errorf("cannot determine installation prefix: %s is not in a <prefix>/bin/ directory", exe)
	}

	prefix := filepath.Dir(dir) // <prefix>
	_ = toolName                // reserved for future validation

	return prefix, nil
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

// =============================================================================
// Binary Installation
// =============================================================================

// installBinary copies the current executable to the target location.
func installBinary(prefix, name string) (string, error) {
	currentExe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	currentExe, err = filepath.EvalSymlinks(currentExe)
	if err != nil {
		return "", fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	binDir := filepath.Join(prefix, "bin")
	targetPath := filepath.Join(binDir, name)

	if err := os.MkdirAll(binDir, 0o750); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", binDir, err)
	}

	// For upgrade: even if source == target, copy via temp file to refresh the binary.
	if currentExe == targetPath {
		return targetPath, nil
	}

	if err := copyFile(currentExe, targetPath); err != nil {
		return "", err
	}

	if err := os.Chmod(targetPath, 0o750); err != nil { //nolint:gosec // G302: binary must be executable
		return "", fmt.Errorf("failed to make executable: %w", err)
	}

	return targetPath, nil
}

// =============================================================================
// Man Pages
// =============================================================================

// installManPagesTo generates and installs man pages.
func installManPagesTo(rootCmd *cobra.Command, path string, header ManHeader) ([]string, error) {
	if err := os.MkdirAll(path, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", path, err)
	}

	now := time.Now()
	h := &doc.GenManHeader{
		Title:   header.Title,
		Section: header.Section,
		Date:    &now,
		Source:  header.Source,
		Manual:  header.Manual,
	}

	if err := doc.GenManTree(rootCmd, h, path); err != nil {
		return nil, fmt.Errorf("failed to generate man pages: %w", err)
	}

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

// =============================================================================
// Shell Completions
// =============================================================================

// shellCompletionPath returns the installation path and filename for a shell's completion file.
func shellCompletionPath(shell, cmdName string) (relPath, filename string) {
	switch shell {
	case "bash":
		return filepath.Join("share", "bash-completion", "completions"), cmdName
	case "fish":
		return filepath.Join("share", "fish", "vendor_completions.d"), cmdName + ".fish"
	case "pwsh":
		return filepath.Join("share", "powershell", "completions"), cmdName + ".ps1"
	case "zsh":
		return filepath.Join("share", "zsh", "site-functions"), "_" + cmdName
	default:
		return "", ""
	}
}

// installCompletionsForShells installs completions for the specified shells.
func installCompletionsForShells(rootCmd *cobra.Command, prefix string, shells []string) ([]string, error) {
	var paths []string

	for _, shellName := range shells {
		rel, filename := shellCompletionPath(shellName, rootCmd.Name())
		if rel == "" {
			Warn("Unknown shell: %s (skipping)", shellName)
			continue
		}

		dir := filepath.Join(prefix, rel)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return paths, fmt.Errorf("failed to create %s completion directory: %w", shellName, err)
		}

		fullPath := filepath.Join(dir, filename)
		f, err := os.Create(fullPath)
		if err != nil {
			return paths, fmt.Errorf("failed to create %s completion file: %w", shellName, err)
		}

		var genErr error
		switch shellName {
		case "bash":
			genErr = rootCmd.GenBashCompletionV2(f, true)
		case "fish":
			genErr = rootCmd.GenFishCompletion(f, true)
		case "pwsh":
			genErr = rootCmd.GenPowerShellCompletionWithDesc(f)
		case "zsh":
			genErr = rootCmd.GenZshCompletion(f)
		default:
			_ = f.Close()
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
		case "pwsh":
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

// detectShells returns available shells on the system.
func detectShells() []string {
	var shells []string

	if _, err := exec.LookPath("bash"); err == nil {
		shells = append(shells, "bash")
	}
	if _, err := exec.LookPath("fish"); err == nil {
		shells = append(shells, "fish")
	}
	if _, err := exec.LookPath("pwsh"); err == nil {
		shells = append(shells, "pwsh")
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

// =============================================================================
// Config and Cache
// =============================================================================

// initDevloreConfig creates the unified devlore config structure.
func initDevloreConfig(info SelfInstallInfo) ([]string, error) {
	if info.ConfigInfo == nil {
		return nil, nil
	}

	var paths []string

	configDir := DevloreConfigHome()
	if err := os.MkdirAll(configDir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	configDDir := filepath.Join(configDir, "config.d")
	if err := os.MkdirAll(configDDir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create config.d directory: %w", err)
	}

	sharedConfigPath := filepath.Join(configDir, "config.yaml")
	if _, err := os.Stat(sharedConfigPath); os.IsNotExist(err) {
		if err := os.WriteFile(sharedConfigPath, schema.SharedDefaultConfig, 0o600); err != nil {
			return nil, fmt.Errorf("failed to write shared config: %w", err)
		}
	}
	paths = append(paths, sharedConfigPath)

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
	cacheDir := filepath.Join(DevloreCacheHome(), toolName)

	if err := os.MkdirAll(cacheDir, 0o750); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	return cacheDir, nil
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

// =============================================================================
// File Helpers
// =============================================================================

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

// CopyDir recursively copies a directory tree.
func CopyDir(src, dst string) error {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)

	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("%s is not a directory", src)
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := CopyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// CollectFiles returns all file paths under dir, relative to base.
func CollectFiles(base, dir string) []string {
	var files []string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error { //nolint:errcheck // errors handled inside callback
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(base, path)
		if err != nil {
			return err
		}
		files = append(files, rel)
		return nil
	})
	return files
}

// fileSHA256 computes the SHA-256 hash of a file.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// relPath returns path relative to prefix.
func relPath(prefix, path string) string {
	rel, err := filepath.Rel(prefix, path)
	if err != nil {
		return path
	}
	return rel
}

// cleanEmptyDirs removes empty directories that contained manifest files.
func cleanEmptyDirs(prefix string, entries []manifestEntry) {
	// Collect unique parent directories, deepest first.
	dirs := make(map[string]bool)
	for _, entry := range entries {
		dir := filepath.Dir(filepath.Join(prefix, entry.Path))
		for dir != prefix && dir != "." && dir != "/" {
			dirs[dir] = true
			dir = filepath.Dir(dir)
		}
	}

	// Try removing each directory (only succeeds if empty).
	for dir := range dirs {
		_ = removeIfEmpty(dir)
	}
}

// removeIfEmpty removes a directory only if it is empty.
func removeIfEmpty(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return os.Remove(dir)
	}
	return nil
}
