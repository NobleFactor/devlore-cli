// SPDX-License-Identifier: Apache-2.0
// Copyright Noble Factor. All rights reserved.

package cli

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"

	"github.com/NobleFactor/devlore-cli/cmd/internal/devlore"
	"github.com/NobleFactor/devlore-cli/pkg/fsroot"
	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/xdg"
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
func runSelfInstall(rootCmd *cobra.Command, prefix string, info SelfInstallInfo, flags installFlags) (err error) {

	// One root for the whole install, threaded through every stage below (#405, phase 2b). The prefix is the
	// only tree here whose anchor is an operator-supplied argument rather than an XDG accessor, so it is
	// opened once at the top rather than derived from a package accessor further down.
	//
	// Writable-unconfined because the prefix need not exist yet — a first install creates it — and a confined
	// root requires its anchor to exist.
	prefixRoot := fsroot.OpenWritableUnconfined(prefix)
	defer iox.Close(&err, prefixRoot)

	// 1. Install binary.
	binPath, err := installBinary(prefixRoot, info.Name)
	if err != nil {
		return fmt.Errorf("failed to install binary: %w", err)
	}
	installed := []string{fmt.Sprintf("Binary:      %s", binPath)} // Display lines
	manifestFiles := []string{relPath(prefix, binPath)}            // Paths relative to prefix (for manifest)

	// 2. Install man pages (if man command exists).
	manLines, manPaths, err := installManPagesUnderPrefix(rootCmd, prefixRoot, info.ManHeader)
	if err != nil {
		return err
	}
	installed = append(installed, manLines...)
	manifestFiles = append(manifestFiles, manPaths...)

	// 3. Install completions for the selected shells.
	completionLines, completionPaths, installedShells, err := installShellCompletions(rootCmd, prefixRoot, flags)
	if err != nil {
		return err
	}
	installed = append(installed, completionLines...)
	manifestFiles = append(manifestFiles, completionPaths...)

	// 4. Initialize config and cache (if the tool has config).
	configLines, err := initConfigAndCache(info)
	if err != nil {
		return err
	}
	installed = append(installed, configLines...)

	// 5. Run post-install hooks (e.g., star extensions).
	hookLines, hookPaths := runPostInstallHooks(prefix, info.PostInstallHooks)
	installed = append(installed, hookLines...)
	manifestFiles = append(manifestFiles, hookPaths...)

	// 6. Create writ layer directories.
	if err := initWritLayerDirectories(info.Name); err != nil {
		return err
	}

	// 7. Write manifest.
	if err := writeManifest(prefixRoot, info.Name, info.Version, manifestFiles); err != nil {
		Warn("Failed to write manifest: %v", err)
	}

	printInstallSummary(info.Name, prefix, installed, installedShells)

	return nil
}

// installManPagesUnderPrefix installs the tool's manual pages, or reports why it did not.
//
// Parameters:
//   - `rootCmd`: the command tree the pages are generated from.
//   - `prefix`: the installation prefix.
//   - `header`: the man page header metadata.
//
// Returns:
//   - `installed`: the display lines, one per installed page; empty when no man command is present.
//   - `manifestFiles`: the installed paths relative to `prefix`, for the manifest.
//   - `err`: non-nil when page generation or placement fails.
func installManPagesUnderPrefix(
	rootCmd *cobra.Command, prefixRoot fsroot.Dir, header ManHeader,
) (installed, manifestFiles []string, err error) {

	if !hasMan() {
		Note("Skipping man pages (man command not found)")
		return nil, nil, nil
	}

	manDir := prefixRoot.NewPath("share", "man", "man1")
	manFiles, err := installManPagesTo(rootCmd, prefixRoot, manDir, header)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to install man pages: %w", err)
	}

	for _, f := range manFiles {
		installed = append(installed, fmt.Sprintf("Man page:    %s", f))
		manifestFiles = append(manifestFiles, relPath(prefixRoot.Name(), f))
	}

	return installed, manifestFiles, nil
}

// installShellCompletions installs completions for the requested shells, detecting them when none were named.
//
// Parameters:
//   - `rootCmd`: the command tree the completions are generated from.
//   - `prefix`: the installation prefix.
//   - `flags`: the install flags; `Shells` names the shells explicitly.
//
// Returns:
//   - `installed`: the display lines, one per installed completion.
//   - `manifestFiles`: the installed paths relative to `prefix`, for the manifest.
//   - `shells`: the shells actually installed for, which the summary reports setup instructions for.
//   - `err`: non-nil when completion generation or placement fails.
func installShellCompletions(
	rootCmd *cobra.Command, prefixRoot fsroot.Dir, flags installFlags,
) (installed, manifestFiles, shells []string, err error) {

	shells = flags.Shells
	if len(shells) == 0 {
		if shells = detectShells(); len(shells) == 0 {
			Note("No shells detected for completions")
			return nil, nil, nil, nil
		}
	}

	completionPaths, err := installCompletionsForShells(rootCmd, prefixRoot, shells)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to install completions: %w", err)
	}

	for _, p := range completionPaths {
		installed = append(installed, fmt.Sprintf("Completion:  %s", p))
		manifestFiles = append(manifestFiles, relPath(prefixRoot.Name(), p))
	}

	return installed, manifestFiles, shells, nil
}

// initConfigAndCache initializes the tool's config and cache directories, when it has config at all.
//
// Parameters:
//   - `info`: the install descriptor; a nil `ConfigInfo` means the tool has no config to initialize.
//
// Returns:
//   - `[]string`: the display lines for the created config paths and cache directory.
//   - `error`: non-nil when either initialization fails.
func initConfigAndCache(info SelfInstallInfo) ([]string, error) {

	if info.ConfigInfo == nil {
		return nil, nil
	}

	configPaths, err := initDevloreConfig(info)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize config: %w", err)
	}

	var installed []string
	for _, p := range configPaths {
		installed = append(installed, fmt.Sprintf("Config:      %s", p))
	}

	cachePath, err := initDevloreCache(info.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize cache: %w", err)
	}

	return append(installed, fmt.Sprintf("Cache:       %s", cachePath)), nil
}

// runPostInstallHooks runs each post-install hook and collects what it placed.
//
// Parameters:
//   - `prefix`: the installation prefix, passed to every hook.
//   - `hooks`: the hooks to run, in order; each returns the paths it installed, relative to `prefix`.
//
// Returns:
//   - `installed`: the display lines, one per installed file.
//   - `manifestFiles`: the installed paths relative to `prefix`, for the manifest.
func runPostInstallHooks(prefix string, hooks []func(string) []string) (installed, manifestFiles []string) {

	for _, hook := range hooks {
		for _, f := range hook(prefix) {
			installed = append(installed, fmt.Sprintf("Extension:   %s", filepath.Join(prefix, f)))
			manifestFiles = append(manifestFiles, f)
		}
	}

	return installed, manifestFiles
}

// initWritLayerDirectories creates and reports writ's layer directories; a no-op for every other tool.
//
// Parameters:
//   - `toolName`: the tool being installed; only `writ` has layer directories.
//
// Returns:
//   - `error`: non-nil when the directories cannot be created.
func initWritLayerDirectories(toolName string) error {

	if toolName != "writ" {
		return nil
	}

	layerPaths, err := initWritLayers()
	if err != nil {
		return fmt.Errorf("failed to create layer directories: %w", err)
	}

	if len(layerPaths) == 0 {
		return nil
	}

	Note("")
	Note("Layer directories:")
	for _, p := range layerPaths {
		Note("  %s", p)
	}

	return nil
}

// printInstallSummary reports what was installed, where, and what the user must still do.
//
// Parameters:
//   - `toolName`: the installed tool.
//   - `prefix`: the installation prefix.
//   - `installed`: the display lines accumulated by every install step.
//   - `installedShells`: the shells completions were installed for; drives the setup instructions.
func printInstallSummary(toolName, prefix string, installed, installedShells []string) {

	Success("Installed %s to %s", toolName, prefix)
	Note("")
	for _, line := range installed {
		Note("  %s", line)
	}

	Note("")
	Note("Add %s to your PATH if not already present.", filepath.Join(prefix, "bin"))
	printShellSetupInstructions(installedShells, toolName)
}

// =============================================================================
// Uninstall
// =============================================================================

// runSelfUninstall removes files recorded in the manifest.
//
//nolint:gocognit // orchestration function with sequential uninstall steps
func runSelfUninstall(prefix string, info SelfInstallInfo) (err error) {

	// One root for the whole uninstall, matching runSelfInstall (#405, phase 2b).
	prefixRoot := fsroot.OpenWritableUnconfined(prefix)
	defer iox.Close(&err, prefixRoot)

	m, err := readManifest(prefix, info.Name)
	if err != nil {
		return fmt.Errorf("no manifest found at %s — was %s installed with 'self install'? (%w)",
			manifestPath(prefix, info.Name), info.Name, err)
	}

	var removed, skipped []string

	for _, entry := range m.Files {
		path := prefixRoot.NewPath(entry.Path)

		currentHash, err := fileSHA256(path.Abs())
		if err != nil {
			// File already gone — that's fine.
			if os.IsNotExist(err) {
				continue
			}
			Warn("Cannot read %s: %v (skipping)", path.Abs(), err)
			skipped = append(skipped, path.Abs())
			continue
		}

		if currentHash != entry.SHA256 {
			skipped = append(skipped, path.Abs())
			continue
		}

		if err := prefixRoot.Remove(path); err != nil {
			Warn("Failed to remove %s: %v", path.Abs(), err)
			skipped = append(skipped, path.Abs())
			continue
		}
		removed = append(removed, path.Abs())
	}

	// Clean up empty directories left behind.
	cleanEmptyDirs(prefixRoot, m.Files)

	// Remove the manifest itself (best-effort).
	mPath := prefixRoot.NewPath(relativeManifestPath(info.Name))
	_ = os.Remove(mPath.Abs())                                                   //nolint:errcheck // best-effort cleanup
	_ = removeIfEmpty(prefixRoot, prefixRoot.NewPath(filepath.Dir(mPath.Rel()))) //nolint:errcheck // best-effort cleanup

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

	// The uninstall path owns the config tree for the length of this removal (#405, phase 2b).
	configRoot := fsroot.OpenWritableUnconfined(devlore.ConfigHome())
	//nolint:errcheck // diagnose-ignored-error: an unconfined root holds no handle, so Close cannot fail; see docs/architecture/2.8-eventing-infrastructure.md
	defer configRoot.Close()

	toolConfig := configRoot.NewPath("config.d", toolName+".yaml")
	if err := configRoot.Remove(toolConfig); err != nil && !os.IsNotExist(err) {
		Warn("Failed to remove config %s: %v", toolConfig.Abs(), err)
	}

	_ = removeIfEmpty(configRoot, configRoot.NewPath("config.d")) //nolint:errcheck // best-effort cleanup
}

// removeDevloreCache removes the tool's cache directory.
func removeDevloreCache(toolName string) {

	cacheRoot := fsroot.OpenWritableUnconfined(devlore.CacheHome())
	//nolint:errcheck // diagnose-ignored-error: an unconfined root holds no handle, so Close cannot fail; see docs/architecture/2.8-eventing-infrastructure.md
	defer cacheRoot.Close()

	cacheDir := cacheRoot.NewPath(toolName)
	if err := cacheRoot.RemoveAll(cacheDir); err != nil && !os.IsNotExist(err) {
		Warn("Failed to remove cache %s: %v", cacheDir.Abs(), err)
	}

	// The cache home is the root's own directory, so its emptiness is judged through the root that anchors it.
	_ = removeIfEmpty(cacheRoot, cacheRoot.NewPath(".")) //nolint:errcheck // best-effort cleanup
}

// =============================================================================
// Manifest
// =============================================================================

// manifestPath returns the path to the manifest file.
func manifestPath(prefix, toolName string) string {
	return filepath.Join(prefix, relativeManifestPath(toolName))
}

// executableName returns the tool's filename as the platform requires it.
//
// Windows will not execute a file without a recognized extension, so an install that copies the binary to
// `bin/writ` produces something the operator cannot run — a successful-looking install of a dead file. Found
// by the self-install scenario on its first Windows run (2026-08-17); every platform's `go build` output
// carries this suffix, and so must every installed copy.
//
// Parameters:
//   - `tool`: the tool name, unsuffixed.
//
// Returns:
//   - `string`: the tool name plus `.exe` on Windows, unchanged elsewhere.
func executableName(tool string) string {

	if runtime.GOOS == "windows" {
		return tool + ".exe"
	}

	return tool
}

// relativeManifestPath returns the manifest's path within the install prefix.
//
// Named separately because a root addresses its contents relatively: [manifestPath] answers "where is it on
// disk", this answers "where is it in the tree", and both derive from one definition.
//
// Parameters:
//   - `toolName`: the installed tool.
//
// Returns:
//   - `string`: the manifest path relative to the install prefix.
func relativeManifestPath(toolName string) string {
	return filepath.Join("share", toolName, "manifest.json")
}

// writeManifest writes the installation manifest.
func writeManifest(prefixRoot fsroot.Dir, toolName, version string, relativePaths []string) error {
	var entries []manifestEntry
	for _, rel := range relativePaths {
		hash, err := fileSHA256(prefixRoot.NewPath(rel).Abs())
		if err != nil {
			// File may not exist (e.g., skipped man pages). Skip silently.
			continue
		}
		entries = append(entries, manifestEntry{Path: rel, SHA256: hash})
	}

	m := manifest{
		Tool:      toolName,
		Version:   version,
		Prefix:    prefixRoot.Name(),
		Installed: time.Now().UTC().Format(time.RFC3339),
		Files:     entries,
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	mPath := prefixRoot.NewPath(relativeManifestPath(toolName))
	if err := prefixRoot.MkdirAll(prefixRoot.NewPath(filepath.Dir(mPath.Rel())), 0o750); err != nil {
		return err
	}

	return prefixRoot.WriteFile(mPath, append(data, '\n'), 0o600)
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
	return xdg.UserHomePath(".local")
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

// expandTilde expands ~ to the user's home directory in a path.
func expandTilde(path string) string {
	if path == "" {
		return ""
	}
	if len(path) >= 2 && path[:2] == "~/" {
		return xdg.UserHomePath(path[2:])
	}
	if path == "~" {
		return xdg.UserHomeDir()
	}
	return path
}

// =============================================================================
// Binary Installation
// =============================================================================

// installBinary copies the current executable to the target location.
func installBinary(prefixRoot fsroot.Dir, name string) (string, error) {
	currentExe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}

	// Confinement: the running executable is wherever the operator launched it from — outside the prefix by
	// definition on a first install, and not ours to confine. The destination side goes through the root.
	currentExe, err = filepath.EvalSymlinks(currentExe)
	if err != nil {
		return "", fmt.Errorf("failed to resolve symlinks: %w", err)
	}

	binDir := prefixRoot.NewPath("bin")
	targetPath := prefixRoot.NewPath("bin", executableName(name))

	if err := prefixRoot.MkdirAll(binDir, 0o750); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", binDir.Abs(), err)
	}

	// For upgrade: even if source == target, copy via temp file to refresh the binary.
	if currentExe == targetPath.Abs() {
		return targetPath.Abs(), nil
	}

	if err := copyFile(prefixRoot, currentExe, targetPath); err != nil {
		return "", err
	}

	if err := prefixRoot.Chmod(targetPath, 0o750); err != nil { //nolint:gosec // G302: binary must be executable
		return "", fmt.Errorf("failed to make executable: %w", err)
	}

	return targetPath.Abs(), nil
}

// =============================================================================
// Man Pages
// =============================================================================

// installManPagesTo generates and installs man pages into `dir` within `prefixRoot`.
func installManPagesTo(rootCmd *cobra.Command, prefixRoot fsroot.Dir, dir fsroot.Path, header ManHeader) ([]string, error) {
	if err := prefixRoot.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", dir.Abs(), err)
	}

	now := time.Now()
	h := &doc.GenManHeader{
		Title:   header.Title,
		Section: header.Section,
		Date:    &now,
		Source:  header.Source,
		Manual:  header.Manual,
	}

	// Confinement: cobra's generator writes the page files itself, given a directory path — those writes are
	// not ours to route through the root. What we own, the directory and its mode, went through it above.
	if err := doc.GenManTree(rootCmd, h, dir.Abs()); err != nil {
		return nil, fmt.Errorf("failed to generate man pages: %w", err)
	}

	var files []string
	entries, err := fs.ReadDir(prefixRoot.FS(), dir.Rel())
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, prefixRoot.NewPath(dir.Rel(), e.Name()).Abs())
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
func installCompletionsForShells(rootCmd *cobra.Command, prefixRoot fsroot.Dir, shells []string) ([]string, error) {
	var paths []string

	for _, shellName := range shells {
		rel, filename := shellCompletionPath(shellName, rootCmd.Name())
		if rel == "" {
			Warn("Unknown shell: %s (skipping)", shellName)
			continue
		}

		dir := prefixRoot.NewPath(rel)
		if err := prefixRoot.MkdirAll(dir, 0o750); err != nil {
			return paths, fmt.Errorf("failed to create %s completion directory: %w", shellName, err)
		}

		fullPath := prefixRoot.NewPath(rel, filename)
		f, err := prefixRoot.Create(fullPath)
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

		paths = append(paths, fullPath.Abs())
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
func initDevloreConfig(info SelfInstallInfo) (paths []string, err error) {
	if info.ConfigInfo == nil {
		return nil, nil
	}

	// One root for the config tree; writable-unconfined because it may not exist yet (#405, phase 2b).
	configRoot := fsroot.OpenWritableUnconfined(devlore.ConfigHome())
	defer iox.Close(&err, configRoot)

	// NewPath(".") is the root's own directory, so the 0750 is applied by the root that anchors it and
	// reaches a Windows DACL rather than being ignored.
	if err := configRoot.MkdirAll(configRoot.NewPath("."), 0o750); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	configDDir := configRoot.NewPath("config.d")
	if err := configRoot.MkdirAll(configDDir, 0o750); err != nil {
		return nil, fmt.Errorf("failed to create config.d directory: %w", err)
	}

	sharedConfigPath := configRoot.NewPath("config.yaml")
	if _, err := configRoot.Stat(sharedConfigPath); os.IsNotExist(err) {
		if err := configRoot.WriteFile(sharedConfigPath, schema.SharedDefaultConfig, 0o600); err != nil {
			return nil, fmt.Errorf("failed to write shared config: %w", err)
		}
	}
	paths = append(paths, sharedConfigPath.Abs())

	toolConfigPath := configRoot.NewPath("config.d", info.Name+".yaml")
	if _, err := configRoot.Stat(toolConfigPath); os.IsNotExist(err) {
		if err := configRoot.WriteFile(toolConfigPath, info.ConfigInfo.DefaultConfig, 0o600); err != nil {
			return nil, fmt.Errorf("failed to write %s config: %w", info.Name, err)
		}
	}
	paths = append(paths, toolConfigPath.Abs())

	return paths, nil
}

// initDevloreCache creates the unified devlore cache structure.
func initDevloreCache(toolName string) (path string, err error) {

	cacheRoot := fsroot.OpenWritableUnconfined(devlore.CacheHome())
	defer iox.Close(&err, cacheRoot)

	cacheDir := cacheRoot.NewPath(toolName)
	if err := cacheRoot.MkdirAll(cacheDir, 0o750); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	return cacheDir.Abs(), nil
}

// initWritLayers creates the writ layer directories if they don't exist.
//
// This function is the campaign's LAST item in disguise: the shared CLI package creating one tool's
// directories is why `devlore` still knows what a writ layer is. See the closure list in
// docs/plans/windows-native-permissions.md — it moves to `cmd/writ` as a post-install hook, and this
// conversion is deliberately shallow so that move stays a move.
func initWritLayers() (created []string, err error) {

	layersRoot := fsroot.OpenWritableUnconfined(devlore.WritLayersDir())
	defer iox.Close(&err, layersRoot)

	for _, layer := range []string{"base", "team", "personal"} {
		layerPath := layersRoot.NewPath(layer)
		if _, err := layersRoot.Stat(layerPath); os.IsNotExist(err) {
			if err := layersRoot.MkdirAll(layerPath, 0o750); err != nil {
				return created, err
			}
			created = append(created, layerPath.Abs())
		}
	}

	return created, nil
}

// =============================================================================
// File Helpers
// =============================================================================

// copyFile copies a file from `src` on the host filesystem to `dst` within `dstRoot`.
//
// The source is deliberately a plain path: it is the running executable or a build tree, outside any root we
// own. The destination is the side that gets confined (#405, phase 2b).
//
// Parameters:
//   - `dstRoot`: the tree the destination belongs to, opened by the caller.
//   - `src`: the source file, outside the root.
//   - `dst`: the destination within `dstRoot`.
//
// Returns:
//   - `error`: non-nil when the source cannot be read or the destination cannot be written.
func copyFile(dstRoot fsroot.Dir, src string, dst fsroot.Path) error {
	source, err := os.Open(src) //nolint:gosec // G304: Confinement: the source is a caller-named path outside the root
	if err != nil {
		return fmt.Errorf("failed to open source: %w", err)
	}
	defer func() { _ = source.Close() }()

	dest, err := dstRoot.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination: %w", err)
	}
	defer func() { _ = dest.Close() }()

	if _, err := io.Copy(dest, source); err != nil {
		return fmt.Errorf("failed to copy: %w", err)
	}

	return nil
}

// CopyDir recursively copies a directory tree from the host filesystem into `dstRoot`.
//
// Used by post-install hooks — `cmd/star` copies its extensions this way — so the destination root is
// supplied by the hook rather than constructed here (#405, phase 2b).
//
// Parameters:
//   - `dstRoot`: the tree the destination belongs to, opened by the caller.
//   - `src`: the source directory, outside the root.
//   - `dst`: the destination within `dstRoot`.
//
// Returns:
//   - `error`: non-nil when the source cannot be read or any destination cannot be written.
func CopyDir(dstRoot fsroot.Dir, src string, dst fsroot.Path) error {
	src = filepath.Clean(src)

	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("%s is not a directory", src)
	}

	if err := dstRoot.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := dstRoot.NewPath(dst.Rel(), entry.Name())

		if entry.IsDir() {
			if err := CopyDir(dstRoot, srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(dstRoot, srcPath, dstPath); err != nil {
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
func cleanEmptyDirs(prefixRoot fsroot.Dir, entries []manifestEntry) {

	// Collect unique parent directories, relative to the root so the walk stops at it rather than at a
	// string comparison against the prefix.
	dirs := make(map[string]bool)
	for _, entry := range entries {
		for dir := filepath.Dir(entry.Path); dir != "." && dir != string(filepath.Separator); dir = filepath.Dir(dir) {
			dirs[dir] = true
		}
	}

	// Try removing each directory (only succeeds if empty).
	for dir := range dirs {
		_ = removeIfEmpty(prefixRoot, prefixRoot.NewPath(dir)) //nolint:errcheck // best-effort cleanup
	}
}

// removeIfEmpty removes a directory only if it is empty.
//
// The root is received, never constructed (#405, phase 2b), and here it has to be: this helper is called with
// directories in three different trees — the config tree, the cache tree, and the install prefix — so it is
// the one site whose authority cannot be decided by reading it.
//
// Parameters:
//   - `root`: the tree `dir` belongs to, opened by the caller.
//   - `dir`: the directory within that root.
//
// Returns:
//   - `error`: non-nil when the directory cannot be read, or is non-empty and cannot be removed.
func removeIfEmpty(root fsroot.Dir, dir fsroot.Path) error {

	entries, err := fs.ReadDir(root.FS(), dir.Rel())
	if err != nil {
		return err
	}

	if len(entries) == 0 {
		return root.Remove(dir)
	}

	return nil
}
