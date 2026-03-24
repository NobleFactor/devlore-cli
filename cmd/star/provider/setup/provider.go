// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

// Package setup provides repository setup operations: tool checks, pre-commit hooks,
// config initialization, and native git hook management.
package setup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	cfg "github.com/NobleFactor/devlore-cli/cmd/star/config"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

var _ op.ContextProvider = (*Provider)(nil)

// Context Data keys.
const (
	// DataKeyDryRun is the context Data key for dry-run mode.
	DataKeyDryRun = "dry_run"

	// DataKeyConfig is the context Data key for the unified config.
	DataKeyConfig = "config"
)

// Provider provides repository setup operations.
//
// +devlore:access=immediate
type Provider struct {
	op.ProviderBase
}

// NewProvider creates a setup provider bound to the given context.
func NewProvider(ctx op.Context) *Provider {
	return &Provider{ProviderBase: op.NewProviderBase(ctx)}
}

func (p *Provider) isDryRun() bool {
	v, _ := p.Context().Data[DataKeyDryRun].(bool)
	return v
}

func (p *Provider) config() *cfg.Config {
	v, _ := p.Context().Data[DataKeyConfig].(*cfg.Config)
	return v
}

func (p *Provider) gitRoot() string {
	root := cfg.GitWorkspaceRoot()
	if root == "" {
		root = "."
	}
	return root
}

// Tools checks whether required development tools are installed.
//
// Returns:
//   - ToolsResult: installation status of each tool
//   - error: never
func (p *Provider) Tools() (ToolsResult, error) {
	result := ToolsResult{AllInstalled: true, Platform: runtime.GOOS}
	for _, tool := range devTools {
		path, _ := exec.LookPath(tool.Binary)
		installed := path != ""
		if !installed {
			result.AllInstalled = false
			result.MissingCount++
		}
		installCmd := tool.Install[runtime.GOOS]
		if installCmd == "" {
			installCmd = fmt.Sprintf("See %s", tool.DocsURL)
		}
		result.Tools = append(result.Tools, ToolStatus{
			Name: tool.Name, Binary: tool.Binary, Description: tool.Description,
			DocsURL: tool.DocsURL, Installed: installed, Path: path,
			Install: tool.Install, InstallCmd: installCmd,
		})
	}
	return result, nil
}

// PrecommitCheck checks whether pre-commit hooks are installed.
//
// Returns:
//   - PrecommitCheckResult: hook installation status
//   - error: never
func (p *Provider) PrecommitCheck() (PrecommitCheckResult, error) {
	precommitPath, _ := exec.LookPath("pre-commit")
	root := p.gitRoot()

	configExists := false
	if _, err := os.Stat(filepath.Join(root, ".pre-commit-config.yaml")); err == nil {
		configExists = true
	}

	hooksInstalled := false
	hookPath := filepath.Join(root, ".git", "hooks", "pre-commit")
	if info, err := os.Stat(hookPath); err == nil && info.Size() > 0 {
		if content, err := os.ReadFile(hookPath); err == nil {
			hooksInstalled = filepath.Ext(hookPath) == "" && len(content) > 100
		}
	}

	return PrecommitCheckResult{
		Installed: hooksInstalled, ConfigExists: configExists,
		PrecommitAvailable: precommitPath != "", PrecommitPath: precommitPath,
	}, nil
}

// PrecommitInstall installs pre-commit hooks.
//
// Returns:
//   - PrecommitInstallResult: success/failure and message
//   - error: if pre-commit execution fails
func (p *Provider) PrecommitInstall() (PrecommitInstallResult, error) {
	root := p.gitRoot()

	if _, err := os.Stat(filepath.Join(root, ".pre-commit-config.yaml")); os.IsNotExist(err) {
		return PrecommitInstallResult{Message: "No .pre-commit-config.yaml found"}, nil
	}

	precommitPath, err := exec.LookPath("pre-commit")
	if err != nil {
		return PrecommitInstallResult{Message: "pre-commit not installed. Run: star setup tools"}, nil
	}

	hookPath := filepath.Join(root, ".git", "hooks", "pre-commit")
	if _, err := os.Stat(hookPath); err == nil {
		return PrecommitInstallResult{Success: true, Message: "Pre-commit hooks already installed", AlreadyInstalled: true}, nil
	}

	if p.isDryRun() {
		return PrecommitInstallResult{Success: true, Message: "[dry-run] would install pre-commit hooks"}, nil
	}

	cmd := exec.Command(precommitPath, "install")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return PrecommitInstallResult{Message: fmt.Sprintf("Failed to install hooks: %s", string(output))}, nil
	}

	return PrecommitInstallResult{Success: true, Message: "Pre-commit hooks installed"}, nil
}

// InitConfig creates star/config.yaml and syncs tool configs.
//
// Returns:
//   - InitConfigResult: config creation status and synced files
//   - error: if config creation or sync fails
func (p *Provider) InitConfig() (InitConfigResult, error) {
	root := p.gitRoot()
	starConfigPath := filepath.Join(root, "star", "config.yaml")
	result := InitConfigResult{ConfigPath: starConfigPath}

	if _, err := os.Stat(starConfigPath); os.IsNotExist(err) {
		if p.isDryRun() {
			result.ConfigCreated = true
		} else {
			if err := os.MkdirAll(filepath.Join(root, "star"), 0o755); err != nil {
				return InitConfigResult{}, fmt.Errorf("creating star directory: %w", err)
			}
			if err := os.WriteFile(starConfigPath, []byte(defaultStarConfig), 0o644); err != nil {
				return InitConfigResult{}, fmt.Errorf("creating star/config.yaml: %w", err)
			}
			result.ConfigCreated = true
		}
	}

	if p.isDryRun() {
		result.ConfigsSynced = []string{".golangci.yaml", ".markdownlint-cli2.yaml"}
	} else if c := p.config(); c != nil {
		synced, err := c.Sync()
		if err != nil {
			return InitConfigResult{}, fmt.Errorf("syncing configs: %w", err)
		}
		if synced.GolangciLint != "" {
			result.ConfigsSynced = append(result.ConfigsSynced, synced.GolangciLint)
		}
		if synced.MarkdownLint != "" {
			result.ConfigsSynced = append(result.ConfigsSynced, synced.MarkdownLint)
		}
	}

	return result, nil
}

// InstallHook installs a native git hook managed by star.
//
// Parameters:
//   - name: hook name (pre-commit, pre-push, commit-msg, post-commit)
//
// Returns:
//   - HookInstallResult: success/failure and message
//   - error: if hook name is invalid or write fails
func (p *Provider) InstallHook(name string) (HookInstallResult, error) {
	if !validHooks[name] {
		return HookInstallResult{}, fmt.Errorf("invalid hook name: %s (valid: pre-commit, pre-push, commit-msg, post-commit)", name)
	}

	root := p.gitRoot()
	if _, err := os.Stat(filepath.Join(root, ".git")); os.IsNotExist(err) {
		return HookInstallResult{Message: "Not a git repository (no .git directory)"}, nil
	}

	hookPath := filepath.Join(root, ".git", "hooks", name)
	hookContent := nativeHookScript(name)

	if data, err := os.ReadFile(hookPath); err == nil {
		if string(data) == hookContent {
			return HookInstallResult{Success: true, Message: "Star hook already installed", AlreadyInstalled: true}, nil
		}
		if !strings.Contains(string(data), "Installed by star") {
			return HookInstallResult{Message: "Existing hook found (not managed by star). Remove it first or use --force"}, nil
		}
	}

	if err := os.MkdirAll(filepath.Join(root, ".git", "hooks"), 0o755); err != nil {
		return HookInstallResult{}, fmt.Errorf("creating hooks directory: %w", err)
	}

	if p.isDryRun() {
		return HookInstallResult{Success: true, Message: fmt.Sprintf("[dry-run] would install %s hook", name)}, nil
	}

	if err := os.WriteFile(hookPath, []byte(hookContent), 0o755); err != nil {
		return HookInstallResult{}, fmt.Errorf("writing hook: %w", err)
	}

	return HookInstallResult{Success: true, Message: fmt.Sprintf("Installed %s hook", name)}, nil
}

// UninstallHook removes a star-managed git hook.
//
// Parameters:
//   - name: hook name to remove
//
// Returns:
//   - HookUninstallResult: success/failure and message
//   - error: if read/remove fails
func (p *Provider) UninstallHook(name string) (HookUninstallResult, error) {
	root := p.gitRoot()
	hookPath := filepath.Join(root, ".git", "hooks", name)

	data, err := os.ReadFile(hookPath)
	if os.IsNotExist(err) {
		return HookUninstallResult{Success: true, Message: "Hook not installed"}, nil
	}
	if err != nil {
		return HookUninstallResult{}, fmt.Errorf("reading hook: %w", err)
	}

	if !strings.Contains(string(data), "Installed by star") {
		return HookUninstallResult{Message: "Hook exists but is not managed by star"}, nil
	}

	if p.isDryRun() {
		return HookUninstallResult{Success: true, Message: fmt.Sprintf("[dry-run] would remove %s hook", name)}, nil
	}

	if err := os.Remove(hookPath); err != nil {
		return HookUninstallResult{}, fmt.Errorf("removing hook: %w", err)
	}

	return HookUninstallResult{Success: true, Message: fmt.Sprintf("Removed %s hook", name)}, nil
}

// CheckHook checks the status of a git hook.
//
// Parameters:
//   - name: hook name to check
//
// Returns:
//   - HookCheckResult: existence and management status
//   - error: if read fails
func (p *Provider) CheckHook(name string) (HookCheckResult, error) {
	root := p.gitRoot()
	hookPath := filepath.Join(root, ".git", "hooks", name)

	data, err := os.ReadFile(hookPath)
	if os.IsNotExist(err) {
		return HookCheckResult{}, nil
	}
	if err != nil {
		return HookCheckResult{}, fmt.Errorf("reading hook: %w", err)
	}

	managed := strings.Contains(string(data), "Installed by star")
	return HookCheckResult{Installed: managed, Exists: true, ManagedByStar: managed}, nil
}

// region UNEXPORTED

type devTool struct {
	Name, Binary, Description, DocsURL string
	Install                            map[string]string
}

var devTools = []devTool{
	{"golangci-lint", "golangci-lint", "Go linter aggregator", "https://golangci-lint.run/usage/install/",
		map[string]string{"darwin": "brew install golangci-lint", "linux": "curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin"}},
	{"shellcheck", "shellcheck", "Shell script static analysis", "https://github.com/koalaman/shellcheck#installing",
		map[string]string{"darwin": "brew install shellcheck", "linux": "sudo apt-get install shellcheck"}},
	{"shfmt", "shfmt", "Shell script formatter", "https://github.com/mvdan/sh",
		map[string]string{"darwin": "brew install shfmt", "linux": "go install mvdan.cc/sh/v3/cmd/shfmt@latest"}},
	{"markdownlint-cli2", "markdownlint-cli2", "Markdown linter", "https://github.com/DavidAnson/markdownlint-cli2",
		map[string]string{"darwin": "brew install markdownlint-cli2", "linux": "npm install -g markdownlint-cli2"}},
}

var validHooks = map[string]bool{
	"pre-commit": true, "pre-push": true, "commit-msg": true, "post-commit": true,
}

func nativeHookScript(hookName string) string {
	return fmt.Sprintf("#!/bin/sh\n# Installed by star - run 'star setup hooks' to reinstall\nexec star hook %s \"$@\"\n", hookName)
}

const defaultStarConfig = `# star/config.yaml - NobleFactor project configuration
lint:
  go:
    path: "./..."
  shell:
    path: "."
    severity: warning
    indent: 4
  markdown:
    path: "."
    exclude:
      - "vendor/**"
      - "node_modules/**"
    frontmatter:
      required:
        - title
        - description
`

// endregion
