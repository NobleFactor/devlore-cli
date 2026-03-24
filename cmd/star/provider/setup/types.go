// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package setup

// ToolsResult holds the outcome of a tool availability check.
type ToolsResult struct {
	Tools        []ToolStatus `starlark:"tools"`
	AllInstalled bool         `starlark:"all_installed"`
	MissingCount int          `starlark:"missing_count"`
	Platform     string       `starlark:"platform"`
}

// ToolStatus represents the installation status of a single tool.
type ToolStatus struct {
	Name        string            `starlark:"name"`
	Binary      string            `starlark:"binary"`
	Description string            `starlark:"description"`
	DocsURL     string            `starlark:"docs_url"`
	Installed   bool              `starlark:"installed"`
	Path        string            `starlark:"path"`
	Install     map[string]string `starlark:"install"`
	InstallCmd  string            `starlark:"install_cmd"`
}

// PrecommitCheckResult holds pre-commit hook status.
type PrecommitCheckResult struct {
	Installed          bool   `starlark:"installed"`
	ConfigExists       bool   `starlark:"config_exists"`
	PrecommitAvailable bool   `starlark:"precommit_available"`
	PrecommitPath      string `starlark:"precommit_path"`
}

// PrecommitInstallResult holds the outcome of a pre-commit install.
type PrecommitInstallResult struct {
	Success          bool   `starlark:"success"`
	Message          string `starlark:"message"`
	AlreadyInstalled bool   `starlark:"already_installed"`
}

// InitConfigResult holds the outcome of config initialization.
type InitConfigResult struct {
	ConfigCreated bool     `starlark:"config_created"`
	ConfigPath    string   `starlark:"config_path"`
	ConfigsSynced []string `starlark:"configs_synced"`
}

// HookInstallResult holds the outcome of a hook install or uninstall.
type HookInstallResult struct {
	Success          bool   `starlark:"success"`
	Message          string `starlark:"message"`
	AlreadyInstalled bool   `starlark:"already_installed"`
}

// HookUninstallResult holds the outcome of a hook uninstall.
type HookUninstallResult struct {
	Success bool   `starlark:"success"`
	Message string `starlark:"message"`
}

// HookCheckResult holds the status of a git hook.
type HookCheckResult struct {
	Installed     bool `starlark:"installed"`
	Exists        bool `starlark:"exists"`
	ManagedByStar bool `starlark:"managed_by_star"`
}
