// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025 Noble Factor. All rights reserved.

package init

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/NobleFactor/devlore-cli/internal/ai"
	"github.com/NobleFactor/devlore-cli/internal/registry"
)

// RepoLayer indicates the precedence layer of a repo.
type RepoLayer string

const (
	LayerBase     RepoLayer = "base"
	LayerTeam     RepoLayer = "team"
	LayerPersonal RepoLayer = "personal"
)

// PackageRecommendation represents a recommended package.
type PackageRecommendation struct {
	Name             string `json:"name" yaml:"name"`
	Reason           string `json:"reason" yaml:"reason"`
	Category         string `json:"category" yaml:"category"` // essential, recommended, optional
	PlatformSpecific bool   `json:"platform_specific" yaml:"platform_specific"`
}

// ProjectSuggestion represents a suggested project to create.
type ProjectSuggestion struct {
	Name        string   `json:"name" yaml:"name"`
	Description string   `json:"description" yaml:"description"`
	Files       []string `json:"files,omitempty" yaml:"files,omitempty"`
}

// NextStep represents a step in the initialization process.
type NextStep struct {
	Step    string `json:"step" yaml:"step"`
	Command string `json:"command" yaml:"command"`
	Note    string `json:"note,omitempty" yaml:"note,omitempty"`
}

// InitPlan represents the complete initialization plan.
type InitPlan struct {
	System struct {
		Platform               string   `json:"platform" yaml:"platform"`
		Architecture           string   `json:"architecture" yaml:"architecture"`
		Shell                  string   `json:"shell" yaml:"shell"`
		PackageManagers        []string `json:"package_managers" yaml:"package_managers"`
		ExistingDotfileManager string   `json:"existing_dotfile_manager,omitempty" yaml:"existing_dotfile_manager,omitempty"`
	} `json:"system" yaml:"system"`

	Recommendation struct {
		Action        string `json:"action" yaml:"action"` // "init" or "migrate"
		Reason        string `json:"reason" yaml:"reason"`
		MigrateSource string `json:"migrate_source,omitempty" yaml:"migrate_source,omitempty"`
	} `json:"recommendation" yaml:"recommendation"`

	RepoSetup struct {
		Layer              RepoLayer `json:"layer" yaml:"layer"`
		LayerReason        string    `json:"layer_reason" yaml:"layer_reason"`
		SuggestedStructure []struct {
			Path        string `json:"path" yaml:"path"`
			Description string `json:"description" yaml:"description"`
		} `json:"suggested_structure" yaml:"suggested_structure"`
	} `json:"repo_setup" yaml:"repo_setup"`

	Packages     []PackageRecommendation `json:"packages" yaml:"packages"`
	Projects     []ProjectSuggestion     `json:"projects" yaml:"projects"`
	NextSteps    []NextStep              `json:"next_steps" yaml:"next_steps"`
	Observations []string                `json:"observations,omitempty" yaml:"observations,omitempty"`
	Warnings     []string                `json:"warnings,omitempty" yaml:"warnings,omitempty"`
}

// Options controls init behavior.
type Options struct {
	Layer      string // user-specified layer preference
	Focus      string // development focus (web, backend, devops, etc.)
	Verbose    bool
	Format     string // "text", "yaml", "json"
	AIProvider ai.Provider
	RegClient  *registry.Client
}

// BuildPlan creates an initialization plan based on system detection and AI analysis.
func BuildPlan(ctx context.Context, opts Options) (*InitPlan, error) {
	plan := &InitPlan{}

	// Detect system information
	detectSystem(plan)

	// Check for existing dotfile managers
	detectExistingManager(plan)

	// If existing manager found, recommend migration
	if plan.System.ExistingDotfileManager != "" {
		plan.Recommendation.Action = "migrate"
		plan.Recommendation.Reason = fmt.Sprintf("Detected existing dotfile manager: %s. Use 'writ migrate' to preserve your configuration.", plan.System.ExistingDotfileManager)
		plan.Recommendation.MigrateSource = plan.System.ExistingDotfileManager
		return plan, nil
	}

	// Use AI-assisted analysis if available
	if opts.AIProvider != nil && opts.RegClient != nil {
		return buildPlanWithAI(ctx, opts, plan)
	}

	// Fall back to basic plan
	return buildPlanBasic(opts, plan)
}

// detectSystem populates system information.
func detectSystem(plan *InitPlan) {
	// Platform
	switch runtime.GOOS {
	case "darwin":
		plan.System.Platform = "Darwin"
	case "linux":
		plan.System.Platform = detectLinuxDistro()
	case "windows":
		plan.System.Platform = "Windows"
	default:
		plan.System.Platform = runtime.GOOS
	}

	// Architecture
	switch runtime.GOARCH {
	case "amd64":
		plan.System.Architecture = "amd64"
	case "arm64":
		plan.System.Architecture = "arm64"
	default:
		plan.System.Architecture = runtime.GOARCH
	}

	// Shell
	plan.System.Shell = detectShell()

	// Package managers
	plan.System.PackageManagers = detectPackageManagers()
}

// detectLinuxDistro attempts to identify the Linux distribution.
func detectLinuxDistro() string {
	// Try /etc/os-release first
	data, err := os.ReadFile("/etc/os-release")
	if err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "ID=") {
				id := strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
				switch id {
				case "debian", "ubuntu":
					return "Linux.Debian"
				case "fedora", "rhel", "centos", "rocky", "almalinux":
					return "Linux.Fedora"
				case "arch", "manjaro":
					return "Linux.Arch"
				default:
					return "Linux"
				}
			}
		}
	}
	return "Linux"
}

// detectShell returns the user's default shell.
func detectShell() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return "unknown"
	}
	return filepath.Base(shell)
}

// detectPackageManagers returns available package managers.
func detectPackageManagers() []string {
	var managers []string
	checks := map[string]string{
		"brew":   "brew",
		"apt":    "apt-get",
		"dnf":    "dnf",
		"pacman": "pacman",
		"winget": "winget",
		"choco":  "choco",
	}

	for name, cmd := range checks {
		if _, err := findExecutable(cmd); err == nil {
			managers = append(managers, name)
		}
	}
	return managers
}

// findExecutable checks if a command exists in PATH.
func findExecutable(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		// Try common locations
		paths := []string{
			"/usr/local/bin/" + name,
			"/opt/homebrew/bin/" + name,
			"/usr/bin/" + name,
		}
		for _, p := range paths {
			if _, err := os.Stat(p); err == nil {
				return p, nil
			}
		}
		return "", fmt.Errorf("not found")
	}
	return path, nil
}

// detectExistingManager checks for existing dotfile managers.
func detectExistingManager(plan *InitPlan) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	checks := []struct {
		path   string
		name   string
		isDir  bool
	}{
		{filepath.Join(home, ".chezmoi.toml"), "chezmoi", false},
		{filepath.Join(home, ".chezmoi.yaml"), "chezmoi", false},
		{filepath.Join(home, ".chezmoiroot"), "chezmoi", false},
		{filepath.Join(home, ".yadm"), "yadm", true},
		{filepath.Join(home, ".config", "yadm"), "yadm", true},
		{filepath.Join(home, ".tuckr"), "tuckr", true},
	}

	for _, check := range checks {
		info, err := os.Stat(check.path)
		if err != nil {
			continue
		}
		if check.isDir && info.IsDir() {
			plan.System.ExistingDotfileManager = check.name
			return
		}
		if !check.isDir && !info.IsDir() {
			plan.System.ExistingDotfileManager = check.name
			return
		}
	}

	// Check for bare git repo
	gitDir := filepath.Join(home, ".git")
	if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
		plan.System.ExistingDotfileManager = "bare-git"
	}
}

// buildPlanWithAI uses AI for recommendations.
func buildPlanWithAI(ctx context.Context, opts Options, plan *InitPlan) (*InitPlan, error) {
	// Load AI prompt
	prompt, err := opts.RegClient.AIPrompt("init-environment.txt")
	if err != nil {
		return buildPlanBasic(opts, plan)
	}

	// Build system info for AI
	systemInfo := fmt.Sprintf(`Platform: %s
Architecture: %s
Shell: %s
Package managers: %s
User-specified layer preference: %s
Development focus: %s`,
		plan.System.Platform,
		plan.System.Architecture,
		plan.System.Shell,
		strings.Join(plan.System.PackageManagers, ", "),
		opts.Layer,
		opts.Focus,
	)

	// Call AI
	resp, err := opts.AIProvider.Chat(ctx, ai.ChatRequest{
		SystemPrompt: prompt,
		Messages: []ai.Message{
			{Role: ai.RoleUser, Content: systemInfo},
		},
		Temperature: 0,
		JSONMode:    true,
	})
	if err != nil {
		return buildPlanBasic(opts, plan)
	}

	// Parse AI response
	if err := json.Unmarshal([]byte(resp.Content), plan); err != nil {
		return buildPlanBasic(opts, plan)
	}

	plan.Recommendation.Action = "init"
	plan.Recommendation.Reason = "No existing dotfile manager detected. Ready to initialize a new environment."

	return plan, nil
}

// buildPlanBasic creates a basic plan without AI.
func buildPlanBasic(opts Options, plan *InitPlan) (*InitPlan, error) {
	plan.Recommendation.Action = "init"
	plan.Recommendation.Reason = "No existing dotfile manager detected. Ready to initialize a new environment."

	// Default layer
	layer := RepoLayer(opts.Layer)
	if layer == "" {
		layer = LayerPersonal
	}
	plan.RepoSetup.Layer = layer
	plan.RepoSetup.LayerReason = "Personal layer is recommended for individual configurations."

	// Suggested structure
	plan.RepoSetup.SuggestedStructure = []struct {
		Path        string `json:"path" yaml:"path"`
		Description string `json:"description" yaml:"description"`
	}{
		{"shell/", "Shell configuration (.bashrc, .zshrc, etc.)"},
		{"git/", "Git configuration (.gitconfig, .gitignore_global)"},
		{"ssh/", "SSH configuration (config, not keys)"},
	}

	// Basic package recommendations based on platform
	plan.Packages = getBasicPackages(plan.System.Platform)

	// Suggested projects
	plan.Projects = []ProjectSuggestion{
		{Name: "shell", Description: "Shell configuration and aliases"},
		{Name: "git", Description: "Git configuration"},
	}

	// Next steps
	plan.NextSteps = []NextStep{
		{Step: "Create repository", Command: fmt.Sprintf("writ repo init --layer=%s", layer)},
		{Step: "Add shell configs", Command: "mkdir -p shell && cp ~/.bashrc shell/ # or .zshrc"},
		{Step: "Deploy configs", Command: "writ add shell"},
	}

	plan.Observations = []string{
		fmt.Sprintf("Detected platform: %s/%s", plan.System.Platform, plan.System.Architecture),
		fmt.Sprintf("Default shell: %s", plan.System.Shell),
		"Run with AI enabled for detailed recommendations",
	}

	return plan, nil
}

// getBasicPackages returns platform-appropriate package recommendations.
func getBasicPackages(platform string) []PackageRecommendation {
	common := []PackageRecommendation{
		{Name: "git", Reason: "Version control", Category: "essential"},
		{Name: "gh", Reason: "GitHub CLI for PR workflows", Category: "recommended"},
		{Name: "jq", Reason: "JSON processing", Category: "recommended"},
		{Name: "ripgrep", Reason: "Fast code search", Category: "recommended"},
		{Name: "fzf", Reason: "Fuzzy finder", Category: "recommended"},
	}

	switch {
	case platform == "Darwin":
		return append(common, PackageRecommendation{
			Name:             "homebrew",
			Reason:           "Package manager for macOS",
			Category:         "essential",
			PlatformSpecific: true,
		})
	case strings.HasPrefix(platform, "Linux"):
		return common
	default:
		return common
	}
}

// FormatPlan writes the init plan in the specified format.
func FormatPlan(w io.Writer, plan *InitPlan, format string) error {
	switch format {
	case "yaml":
		enc := yaml.NewEncoder(w)
		enc.SetIndent(2)
		return enc.Encode(plan)
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(plan)
	default:
		return formatText(w, plan)
	}
}

func formatText(w io.Writer, plan *InitPlan) error {
	fmt.Fprintf(w, "Environment Initialization Plan\n")
	fmt.Fprintf(w, "================================\n\n")

	// System info
	fmt.Fprintf(w, "System:\n")
	fmt.Fprintf(w, "  Platform:     %s\n", plan.System.Platform)
	fmt.Fprintf(w, "  Architecture: %s\n", plan.System.Architecture)
	fmt.Fprintf(w, "  Shell:        %s\n", plan.System.Shell)
	if len(plan.System.PackageManagers) > 0 {
		fmt.Fprintf(w, "  Package Mgrs: %s\n", strings.Join(plan.System.PackageManagers, ", "))
	}
	fmt.Fprintln(w)

	// Recommendation
	fmt.Fprintf(w, "Recommendation: %s\n", plan.Recommendation.Action)
	fmt.Fprintf(w, "  %s\n", plan.Recommendation.Reason)
	if plan.Recommendation.MigrateSource != "" {
		fmt.Fprintf(w, "\n  Run: writ migrate ~/<your-dotfiles>\n")
		return nil
	}
	fmt.Fprintln(w)

	// Repo setup
	if plan.RepoSetup.Layer != "" {
		fmt.Fprintf(w, "Repository Setup:\n")
		fmt.Fprintf(w, "  Layer: %s\n", plan.RepoSetup.Layer)
		fmt.Fprintf(w, "  %s\n", plan.RepoSetup.LayerReason)
		fmt.Fprintln(w)
	}

	// Packages
	if len(plan.Packages) > 0 {
		fmt.Fprintf(w, "Recommended Packages:\n")
		for _, pkg := range plan.Packages {
			marker := " "
			if pkg.Category == "essential" {
				marker = "*"
			}
			fmt.Fprintf(w, "  %s %-12s  %s\n", marker, pkg.Name, pkg.Reason)
		}
		fmt.Fprintln(w)
	}

	// Projects
	if len(plan.Projects) > 0 {
		fmt.Fprintf(w, "Suggested Projects:\n")
		for _, proj := range plan.Projects {
			fmt.Fprintf(w, "  %-10s  %s\n", proj.Name, proj.Description)
		}
		fmt.Fprintln(w)
	}

	// Next steps
	if len(plan.NextSteps) > 0 {
		fmt.Fprintf(w, "Next Steps:\n")
		for i, step := range plan.NextSteps {
			fmt.Fprintf(w, "  %d. %s\n", i+1, step.Step)
			if step.Command != "" {
				fmt.Fprintf(w, "     $ %s\n", step.Command)
			}
			if step.Note != "" {
				fmt.Fprintf(w, "     Note: %s\n", step.Note)
			}
		}
		fmt.Fprintln(w)
	}

	// Observations
	if len(plan.Observations) > 0 {
		fmt.Fprintf(w, "Observations:\n")
		for _, obs := range plan.Observations {
			fmt.Fprintf(w, "  - %s\n", obs)
		}
		fmt.Fprintln(w)
	}

	// Warnings
	if len(plan.Warnings) > 0 {
		fmt.Fprintf(w, "Warnings:\n")
		for _, warn := range plan.Warnings {
			fmt.Fprintf(w, "  ! %s\n", warn)
		}
		fmt.Fprintln(w)
	}

	return nil
}
