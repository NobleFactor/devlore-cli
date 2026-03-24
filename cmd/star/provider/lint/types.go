// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package lint

// GoResult holds the outcome of a Go lint run.
type GoResult struct {
	Issues         []GoIssue `starlark:"issues"`
	ErrorCount     int       `starlark:"error_count"`
	WarningCount   int       `starlark:"warning_count"`
	TotalCount     int       `starlark:"total_count"`
	Passed         bool      `starlark:"passed"`
	LintPassed     bool      `starlark:"lint_passed"`
	ConfigCreated  bool      `starlark:"config_created"`
	ModTidyPassed  bool      `starlark:"mod_tidy_passed"`
	ModTidyDetails string    `starlark:"mod_tidy_details"`
}

// GoIssue represents a single golangci-lint finding.
type GoIssue struct {
	File        string   `starlark:"file"`
	Line        int      `starlark:"line"`
	Column      int      `starlark:"column"`
	Message     string   `starlark:"message"`
	Linter      string   `starlark:"linter"`
	Severity    string   `starlark:"severity"`
	SourceLines []string `starlark:"source_lines"`
}

// ShellResult holds the outcome of a combined shellcheck + shfmt lint run.
type ShellResult struct {
	Issues       []ShellIssue       `starlark:"issues"`
	FormatIssues []ShellFormatIssue `starlark:"format_issues"`
	ErrorCount   int                `starlark:"error_count"`
	WarningCount int                `starlark:"warning_count"`
	FilesChecked int                `starlark:"files_checked"`
	LintPassed   bool               `starlark:"lint_passed"`
	FormatPassed bool               `starlark:"format_passed"`
	Passed       bool               `starlark:"passed"`
}

// ShellIssue represents a single shellcheck finding in a lint context.
type ShellIssue struct {
	File    string `starlark:"file"`
	Line    int    `starlark:"line"`
	Column  int    `starlark:"column"`
	Level   string `starlark:"level"`
	Code    int    `starlark:"code"`
	Message string `starlark:"message"`
}

// ShellFormatIssue represents a file that failed shfmt formatting check.
type ShellFormatIssue struct {
	File string `starlark:"file"`
	Diff string `starlark:"diff"`
}

// MarkdownResult holds the outcome of a markdown lint run.
type MarkdownResult struct {
	Issues            []MarkdownIssue    `starlark:"issues"`
	FrontmatterIssues []FrontmatterIssue `starlark:"frontmatter_issues"`
	FilesChecked      int                `starlark:"files_checked"`
	IssueCount        int                `starlark:"issue_count"`
	LintPassed        bool               `starlark:"lint_passed"`
	FrontmatterPassed bool               `starlark:"frontmatter_passed"`
	Passed            bool               `starlark:"passed"`
}

// MarkdownIssue represents a single markdownlint finding.
type MarkdownIssue struct {
	File     string `starlark:"file"`
	Line     int    `starlark:"line"`
	Rule     string `starlark:"rule"`
	Message  string `starlark:"message"`
	Severity string `starlark:"severity"`
}

// FrontmatterIssue represents a frontmatter validation issue.
type FrontmatterIssue struct {
	File    string `starlark:"file"`
	Message string `starlark:"message"`
}

// ToolsResult holds the outcome of a tool availability check.
type ToolsResult struct {
	AllInstalled bool       `starlark:"all_installed"`
	Tools        []ToolInfo `starlark:"tools"`
	InstallCmds  []string   `starlark:"install_cmds"`
}

// ToolInfo represents the installation status of a single tool.
type ToolInfo struct {
	Name       string `starlark:"name"`
	Installed  bool   `starlark:"installed"`
	Path       string `starlark:"path"`
	InstallCmd string `starlark:"install_cmd"`
}
