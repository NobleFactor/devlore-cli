// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// RepoScanResult holds the results of scanning a potential writ repository.
type RepoScanResult struct {
	Path string

	// Git information
	IsGit    bool
	Remote   string
	Branch   string
	Dirty    int // count of uncommitted changes
	GitError string

	// Structure detection
	Structure    RepoStructure
	HomePath     string   // path to detected Home-equivalent directory
	Projects     []ProjectInfo
	NestedUnder  string   // non-empty if projects are nested (e.g., "Configs")
	TemplateCount int
	SecretCount   int
	HasRecipients bool

	// Migration guidance
	Migrations []Migration
}

// RepoStructure describes the detected repository layout.
type RepoStructure int

const (
	StructureUnknown     RepoStructure = iota
	StructureCompatible                // Home/<project>/ with "." segments
	StructurePartial                   // Has Home/ but needs migration
	StructureTraditional               // Dotfiles at root
	StructureStow                      // Stow-like packages at root
)

// String returns a human-readable label for the structure type.
func (s RepoStructure) String() string {
	switch s {
	case StructureCompatible:
		return "writ-compatible"
	case StructurePartial:
		return "partial match"
	case StructureTraditional:
		return "traditional dotfiles"
	case StructureStow:
		return "stow-like"
	default:
		return "unknown"
	}
}

// ProjectInfo describes a detected project directory.
type ProjectInfo struct {
	Name     string
	Path     string
	Segment  string // segment suffix (e.g., "Darwin", "Unix")
	FileCount int
	Templates int
	Secrets   int
}

// Migration describes a recommended migration step.
type Migration struct {
	Issue    string   // what's wrong
	Commands []string // suggested git commands to fix it
}

// ScanRepo inspects a directory and reports on its suitability as a writ repository.
func ScanRepo(path string) *RepoScanResult {
	result := &RepoScanResult{Path: path}

	// Check path exists
	info, err := os.Stat(path)
	if err != nil {
		result.Migrations = append(result.Migrations, Migration{
			Issue: fmt.Sprintf("Path does not exist: %s", path),
		})
		return result
	}
	if !info.IsDir() {
		result.Migrations = append(result.Migrations, Migration{
			Issue: fmt.Sprintf("Not a directory: %s", path),
		})
		return result
	}

	// Check git status
	scanGit(result)

	// Look for Home/ directory
	homePath := filepath.Join(path, "Home")
	if dirExists(homePath) {
		result.HomePath = homePath
		scanHomeDir(result, homePath)
	} else {
		// Check for alternative structures
		scanAlternativeStructures(result)
	}

	return result
}

// scanGit checks git repository status.
func scanGit(result *RepoScanResult) {
	gitDir := filepath.Join(result.Path, ".git")
	if !dirExists(gitDir) {
		return
	}
	result.IsGit = true

	// Get remote
	if out, err := gitCmd(result.Path, "remote", "get-url", "origin"); err == nil {
		result.Remote = strings.TrimSpace(out)
	}

	// Get branch
	if out, err := gitCmd(result.Path, "branch", "--show-current"); err == nil {
		result.Branch = strings.TrimSpace(out)
	}

	// Count uncommitted changes
	if out, err := gitCmd(result.Path, "status", "--short"); err == nil {
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if lines[0] != "" {
			result.Dirty = len(lines)
		}
	}
}

// scanHomeDir scans the Home/ directory for projects and structure.
func scanHomeDir(result *RepoScanResult, homePath string) {
	entries, err := os.ReadDir(homePath)
	if err != nil {
		return
	}

	// Check if projects are directly under Home/ or nested one level deeper.
	// Writ expects: Home/<project>/
	// Detect: Home/<intermediary>/<project>/
	if len(entries) == 1 && entries[0].IsDir() {
		// Single subdirectory — check if it contains project-like dirs
		nested := entries[0].Name()
		nestedPath := filepath.Join(homePath, nested)
		nestedEntries, err := os.ReadDir(nestedPath)
		if err == nil && hasProjectDirs(nestedEntries) {
			result.NestedUnder = nested
			result.HomePath = nestedPath
			scanProjects(result, nestedPath)

			result.Migrations = append(result.Migrations, Migration{
				Issue: fmt.Sprintf("Extra nesting: writ expects Home/<project>/, not Home/%s/<project>/", nested),
				Commands: []string{
					fmt.Sprintf("cd %s && git mv Home/%s/* Home/ && rmdir Home/%s", result.Path, nested, nested),
				},
			})
			return
		}
	}

	// Projects directly under Home/
	if hasProjectDirs(entries) {
		scanProjects(result, homePath)
	}
}

// scanProjects scans project directories and checks segment conventions.
func scanProjects(result *RepoScanResult, projectsPath string) {
	entries, err := os.ReadDir(projectsPath)
	if err != nil {
		return
	}

	var dashSegments []string
	var dotSegments []string

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		projPath := filepath.Join(projectsPath, name)

		pi := ProjectInfo{
			Name: name,
			Path: projPath,
		}

		// Detect segment separator and extract segment
		if idx := strings.LastIndex(name, "."); idx > 0 {
			pi.Segment = name[idx+1:]
			pi.Name = name[:idx]
			dotSegments = append(dotSegments, entry.Name())
		} else if idx := strings.LastIndex(name, "-"); idx > 0 {
			candidate := name[idx+1:]
			if isKnownSegment(candidate) {
				pi.Segment = candidate
				pi.Name = name[:idx]
				dashSegments = append(dashSegments, entry.Name())
			}
		}

		// Count files, templates, secrets
		filepath.Walk(projPath, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			pi.FileCount++
			if strings.HasSuffix(info.Name(), ".template") {
				pi.Templates++
				result.TemplateCount++
			}
			if strings.HasSuffix(info.Name(), ".age") {
				pi.Secrets++
				result.SecretCount++
			}
			return nil
		})

		result.Projects = append(result.Projects, pi)
	}

	// Check for .age-recipients
	recipientsPath := filepath.Join(result.Path, ".age-recipients")
	if fileExists(recipientsPath) {
		result.HasRecipients = true
	}

	// Sort projects by name for consistent output
	sort.Slice(result.Projects, func(i, j int) bool {
		return result.Projects[i].Name < result.Projects[j].Name
	})

	// Determine the relative path from repo root to the projects directory
	relProjects, _ := filepath.Rel(result.Path, projectsPath)

	// Determine structure compatibility
	if len(dashSegments) > 0 && len(dotSegments) == 0 {
		result.Structure = StructurePartial

		// Generate rename commands with full relative paths
		var cmds []string
		for _, ds := range dashSegments {
			idx := strings.LastIndex(ds, "-")
			newName := ds[:idx] + "." + ds[idx+1:]
			oldRel := filepath.Join(relProjects, ds)
			newRel := filepath.Join(relProjects, newName)
			cmds = append(cmds, fmt.Sprintf("git mv %s %s", oldRel, newRel))
		}
		result.Migrations = append(result.Migrations, Migration{
			Issue:    "Segment separator: writ uses \".\" (e.g., all.Darwin), not \"-\" (e.g., all-Darwin)",
			Commands: cmds,
		})
	} else if len(result.Migrations) == 0 {
		result.Structure = StructureCompatible
	} else {
		result.Structure = StructurePartial
	}
}

// scanAlternativeStructures checks for traditional or stow-like repos.
func scanAlternativeStructures(result *RepoScanResult) {
	entries, err := os.ReadDir(result.Path)
	if err != nil {
		return
	}

	var dotfiles []string
	var stowDirs []string

	for _, entry := range entries {
		name := entry.Name()
		if name == ".git" || name == ".gitignore" || name == ".gitmodules" {
			continue
		}

		if strings.HasPrefix(name, ".") && !entry.IsDir() {
			dotfiles = append(dotfiles, name)
		}

		if entry.IsDir() && !strings.HasPrefix(name, ".") {
			// Check if this dir contains dotfiles (stow pattern)
			subEntries, err := os.ReadDir(filepath.Join(result.Path, name))
			if err == nil {
				for _, se := range subEntries {
					if strings.HasPrefix(se.Name(), ".") {
						stowDirs = append(stowDirs, name)
						break
					}
				}
			}
		}
	}

	if len(dotfiles) > 2 {
		result.Structure = StructureTraditional
		result.Migrations = append(result.Migrations, Migration{
			Issue: fmt.Sprintf("Traditional dotfiles repo (found %s at root)", strings.Join(dotfiles, ", ")),
			Commands: []string{
				"mkdir -p Home/all",
				fmt.Sprintf("git mv %s Home/all/", strings.Join(dotfiles, " ")),
			},
		})
	} else if len(stowDirs) > 1 {
		result.Structure = StructureStow
		result.Migrations = append(result.Migrations, Migration{
			Issue: fmt.Sprintf("Stow-like repo (directories with dotfiles: %s)", strings.Join(stowDirs, ", ")),
			Commands: []string{
				"mkdir Home",
				fmt.Sprintf("git mv %s Home/", strings.Join(stowDirs, " ")),
				"# Rename if desired: git mv Home/zsh Home/all",
			},
		})
	} else {
		result.Structure = StructureUnknown
		result.Migrations = append(result.Migrations, Migration{
			Issue: "No Home/ directory found and structure not recognized",
			Commands: []string{
				"mkdir -p Home/all",
				"# Move your dotfiles into Home/all/",
			},
		})
	}
}

// PrintReport outputs the scan result to stderr.
func (r *RepoScanResult) PrintReport() {
	Note("Scanning repo: %s", r.Path)

	// Git info
	if r.IsGit {
		gitInfo := fmt.Sprintf("Git: yes (branch: %s", r.Branch)
		if r.Dirty > 0 {
			gitInfo += fmt.Sprintf(", %d uncommitted", r.Dirty)
		}
		gitInfo += ")"
		Note("%s", gitInfo)
		if r.Remote != "" {
			Note("  origin: %s", r.Remote)
		}
	} else {
		Warn("Git: no (not a git repository)")
	}

	// Structure
	Note("Structure: %s", r.Structure)

	// Projects
	if len(r.Projects) > 0 {
		fmt.Fprintf(os.Stderr, "\n")
		Note("Projects:")
		for _, p := range r.Projects {
			suffix := ""
			if p.Segment != "" {
				suffix = fmt.Sprintf(" [%s]", p.Segment)
			}
			details := fmt.Sprintf("%d files", p.FileCount)
			if p.Templates > 0 {
				details += fmt.Sprintf(", %d templates", p.Templates)
			}
			if p.Secrets > 0 {
				details += fmt.Sprintf(", %d secrets", p.Secrets)
			}
			Note("  %-24s %s%s", p.Name, details, suffix)
		}
	}

	// Features
	fmt.Fprintf(os.Stderr, "\n")
	if r.TemplateCount > 0 {
		Note("Templates: %d (.template)", r.TemplateCount)
	}
	if r.SecretCount > 0 {
		Note("Secrets: %d (.age)", r.SecretCount)
	}
	if r.HasRecipients {
		Note("Recipients: .age-recipients found")
	}

	// Migrations
	if len(r.Migrations) > 0 {
		fmt.Fprintf(os.Stderr, "\n")
		Warn("Migration needed:")
		for i, m := range r.Migrations {
			fmt.Fprintf(os.Stderr, "\n")
			Warn("  %d. %s", i+1, m.Issue)
			for _, cmd := range m.Commands {
				Note("     %s", cmd)
			}
		}
	} else {
		fmt.Fprintf(os.Stderr, "\n")
		Success("No migration needed")
	}
}

// NeedsMigration returns true if the scan found issues that require migration.
func (r *RepoScanResult) NeedsMigration() bool {
	return len(r.Migrations) > 0
}

// isKnownSegment returns true if the string is a recognized platform segment.
func isKnownSegment(s string) bool {
	known := []string{
		"Darwin", "Linux", "Windows", "FreeBSD",
		"Unix", "Debian", "Ubuntu", "Fedora", "RHEL",
		"ARM64", "AMD64",
	}
	for _, k := range known {
		if strings.EqualFold(s, k) {
			return true
		}
	}
	return false
}

// hasProjectDirs returns true if the entries contain directories that look like projects.
func hasProjectDirs(entries []os.DirEntry) bool {
	dirs := 0
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			dirs++
		}
	}
	return dirs >= 1
}

// dirExists returns true if path exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// fileExists returns true if path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// gitCmd runs a git command in the given directory and returns stdout.
func gitCmd(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}
