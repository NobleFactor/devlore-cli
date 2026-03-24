// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

// Package lint provides static analysis operations for Go, shell, and markdown files.
package lint

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	shellcheckprov "github.com/NobleFactor/devlore-cli/cmd/star/provider/shellcheck"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

var _ op.ContextProvider = (*Provider)(nil)

// Provider provides static analysis operations: Go linting (golangci-lint), shell linting
// (shellcheck + shfmt), markdown linting (markdownlint-cli2), and tool availability checking.
//
// +devlore:access=immediate
type Provider struct {
	op.ProviderBase
}

// NewProvider creates a lint provider bound to the given context.
func NewProvider(ctx op.Context) *Provider {
	return &Provider{ProviderBase: op.NewProviderBase(ctx)}
}

// Go runs golangci-lint on Go source files.
//
// +devlore:defaults paths=nil,config="",skipModTidy=false
//
// Parameters:
//   - paths: Go package patterns to lint (default ["./..."])
//   - config: path to golangci-lint config file (default: auto-detect or create)
//   - skipModTidy: if true, skip go mod tidy verification
//
// Returns:
//   - GoResult: issues, counts, and pass/fail status
//   - error: if golangci-lint is not installed
func (p *Provider) Go(paths []string, config string, skipModTidy bool) (GoResult, error) {
	if len(paths) == 0 {
		paths = []string{"./..."}
	}

	modTidyPassed := true
	modTidyDetails := ""
	if !skipModTidy {
		modTidyPassed, modTidyDetails = checkModTidy()
	}

	if checkTool("golangci-lint") == "" {
		return GoResult{}, fmt.Errorf("golangci-lint not installed\n  Install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest")
	}

	var configCreated bool
	if config == "" {
		var err error
		config, configCreated, err = ensureGolangciConfig()
		if err != nil {
			return GoResult{}, err
		}
	}

	cmdArgs := []string{"run", "--output.json.path", "stdout"}
	if config != "" {
		if !filepath.IsAbs(config) {
			if absConfig, err := filepath.Abs(config); err == nil {
				config = absConfig
			}
		}
		cmdArgs = append(cmdArgs, "--config="+config)
	}
	cmdArgs = append(cmdArgs, paths...)

	cmd := exec.CommandContext(context.Background(), "golangci-lint", cmdArgs...)
	output, err := cmd.Output()

	var issues []goIssueRaw
	if len(output) > 0 {
		var lintOutput goOutputRaw
		if jsonErr := json.Unmarshal(output, &lintOutput); jsonErr == nil {
			issues = lintOutput.Issues
		}
	}

	if err != nil && len(output) == 0 {
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr := string(exitErr.Stderr)
			if stderr != "" {
				return GoResult{}, fmt.Errorf("golangci-lint failed: %s", strings.TrimSpace(stderr))
			}
		}
	}

	result := GoResult{
		ConfigCreated:  configCreated,
		ModTidyPassed:  modTidyPassed,
		ModTidyDetails: modTidyDetails,
	}

	for _, issue := range issues {
		severity := issue.Severity
		if severity == "" {
			severity = "warning"
		}
		switch severity {
		case "error":
			result.ErrorCount++
		default:
			result.WarningCount++
		}
		result.Issues = append(result.Issues, GoIssue{
			File:        issue.Pos.Filename,
			Line:        issue.Pos.Line,
			Column:      issue.Pos.Column,
			Message:     issue.Text,
			Linter:      issue.FromLinter,
			Severity:    severity,
			SourceLines: issue.SourceLines,
		})
	}

	result.TotalCount = len(result.Issues)
	result.LintPassed = result.ErrorCount == 0 && result.WarningCount == 0
	result.Passed = result.LintPassed && modTidyPassed

	return result, nil
}

// Shell runs shellcheck and shfmt on shell scripts.
//
// +devlore:defaults files=nil,severity="warning",indent=0
//
// Parameters:
//   - files: shell script files to lint
//   - severity: minimum shellcheck severity (default "warning")
//   - indent: shfmt indentation width (defaults to 4 when 0)
//
// Returns:
//   - ShellResult: lint issues, format issues, and pass/fail status
//   - error: if shellcheck or shfmt is not installed
func (p *Provider) Shell(files []string, severity string, indent int) (ShellResult, error) {
	if severity == "" {
		severity = "warning"
	}
	if indent == 0 {
		indent = 4
	}

	if len(files) == 0 {
		return ShellResult{LintPassed: true, FormatPassed: true, Passed: true}, nil
	}

	if checkTool("shellcheck") == "" {
		return ShellResult{}, fmt.Errorf("shellcheck not installed\n  Install: %s", shellcheckInstallCmd())
	}
	if checkTool("shfmt") == "" {
		return ShellResult{}, fmt.Errorf("shfmt not installed\n  Install: go install mvdan.cc/sh/v3/cmd/shfmt@latest")
	}

	result := ShellResult{FilesChecked: len(files)}

	for _, file := range files {
		issues, lintErr := runShellcheckForLint(file, severity)
		if lintErr != nil {
			continue
		}
		for _, issue := range issues {
			switch issue.Level {
			case "error":
				result.ErrorCount++
			case "warning":
				result.WarningCount++
			}
			result.Issues = append(result.Issues, ShellIssue{
				File: issue.File, Line: issue.Line, Column: issue.Column,
				Level: issue.Level, Code: issue.Code, Message: issue.Message,
			})
		}
	}

	for _, file := range files {
		cmd := exec.CommandContext(context.Background(), "shfmt", "-d", "-i", fmt.Sprintf("%d", indent), "-ci", file)
		output, fmtErr := cmd.CombinedOutput()
		if fmtErr != nil {
			result.FormatIssues = append(result.FormatIssues, ShellFormatIssue{
				File: file, Diff: string(output),
			})
		}
	}

	result.LintPassed = result.ErrorCount == 0 && result.WarningCount == 0
	result.FormatPassed = len(result.FormatIssues) == 0
	result.Passed = result.LintPassed && result.FormatPassed

	return result, nil
}

// Markdown runs markdownlint-cli2 and frontmatter validation on markdown files.
//
// +devlore:defaults files=nil,fix=false
//
// Parameters:
//   - files: markdown files to lint
//   - fix: if true, apply automatic fixes
//
// Returns:
//   - MarkdownResult: lint issues, frontmatter issues, and pass/fail status
//   - error: if markdownlint-cli2 is not installed
func (p *Provider) Markdown(files []string, fix bool) (MarkdownResult, error) {
	if len(files) == 0 {
		return MarkdownResult{LintPassed: true, FrontmatterPassed: true, Passed: true}, nil
	}

	if checkTool("markdownlint-cli2") == "" {
		return MarkdownResult{}, fmt.Errorf("markdownlint-cli2 not installed\n  Install: %s", markdownlintInstallCmd())
	}

	lintIssues, err := runMarkdownLint(files, fix)
	if err != nil {
		return MarkdownResult{}, err
	}

	fmIssues, err := checkFrontmatter(files)
	if err != nil {
		return MarkdownResult{}, fmt.Errorf("checking frontmatter: %w", err)
	}

	result := MarkdownResult{
		FilesChecked:      len(files),
		IssueCount:        len(lintIssues),
		LintPassed:        len(lintIssues) == 0,
		FrontmatterPassed: len(fmIssues) == 0,
	}
	result.Passed = result.LintPassed && result.FrontmatterPassed

	for _, issue := range lintIssues {
		result.Issues = append(result.Issues, MarkdownIssue{
			File: issue.file, Line: issue.line, Rule: issue.rule,
			Message: issue.message, Severity: issue.severity,
		})
	}
	for _, issue := range fmIssues {
		result.FrontmatterIssues = append(result.FrontmatterIssues, FrontmatterIssue{
			File: issue.file, Message: issue.message,
		})
	}

	return result, nil
}

// EnsureTools checks whether required development tools are installed.
//
// Returns:
//   - ToolsResult: installation status of each tool
//   - error: never (always succeeds)
func (p *Provider) EnsureTools() (ToolsResult, error) {
	tools := []struct {
		name, binary, installCmd string
	}{
		{"golangci-lint", "golangci-lint", "curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin"},
		{"shellcheck", "shellcheck", shellcheckInstallCmd()},
		{"shfmt", "shfmt", "go install mvdan.cc/sh/v3/cmd/shfmt@latest"},
		{"markdownlint-cli2", "markdownlint-cli2", markdownlintInstallCmd()},
	}

	result := ToolsResult{AllInstalled: true}
	for _, tool := range tools {
		path := checkTool(tool.binary)
		installed := path != ""
		if !installed {
			result.AllInstalled = false
			result.InstallCmds = append(result.InstallCmds, tool.installCmd)
		}
		result.Tools = append(result.Tools, ToolInfo{
			Name: tool.name, Installed: installed, Path: path, InstallCmd: tool.installCmd,
		})
	}

	return result, nil
}

// region UNEXPORTED FUNCTIONS

// golangci-lint JSON output types (for unmarshaling only).
type goOutputRaw struct {
	Issues []goIssueRaw `json:"Issues"`
}

type goIssueRaw struct {
	FromLinter  string   `json:"FromLinter"`
	Text        string   `json:"Text"`
	Severity    string   `json:"Severity"`
	SourceLines []string `json:"SourceLines"`
	Pos         goPosRaw `json:"Pos"`
}

type goPosRaw struct {
	Filename string `json:"Filename"`
	Line     int    `json:"Line"`
	Column   int    `json:"Column"`
}

func checkModTidy() (bool, string) {
	tidyCmd := exec.Command("go", "mod", "tidy")
	if output, err := tidyCmd.CombinedOutput(); err != nil {
		return false, fmt.Sprintf("go mod tidy failed: %s\n%s", err, string(output))
	}
	diffCmd := exec.Command("git", "diff", "--exit-code", "go.mod", "go.sum")
	if output, err := diffCmd.CombinedOutput(); err != nil {
		return false, fmt.Sprintf("go.mod or go.sum not tidy:\n%s", string(output))
	}
	return true, ""
}

func checkTool(binary string) string {
	path, err := exec.LookPath(binary)
	if err != nil {
		return ""
	}
	return path
}

func shellcheckInstallCmd() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install shellcheck"
	case "linux":
		return "sudo apt-get install shellcheck"
	default:
		return "See https://github.com/koalaman/shellcheck#installing"
	}
}

func markdownlintInstallCmd() string {
	switch runtime.GOOS {
	case "darwin":
		return "brew install markdownlint-cli2"
	default:
		return "npm install -g markdownlint-cli2"
	}
}

func runShellcheckForLint(path, severity string) ([]shellcheckprov.LintIssue, error) {
	cmd := exec.CommandContext(context.Background(), "shellcheck", "-f", "json", "-x", "--severity="+severity, path)
	output, err := cmd.Output()
	if err != nil && len(output) == 0 {
		return nil, nil
	}
	if len(output) == 0 {
		return nil, nil
	}
	var issues []shellcheckprov.LintIssue
	if err := json.Unmarshal(output, &issues); err != nil {
		return nil, fmt.Errorf("parsing shellcheck output: %w", err)
	}
	return issues, nil
}

const defaultGolangciConfig = `# SPDX-License-Identifier: MIT
# golangci-lint v2 configuration for NobleFactor Go projects

version: "2"

run:
  timeout: 5m
  modules-download-mode: readonly

formatters:
  enable:
    - gofmt
    - goimports

linters:
  default: none
  enable:
    - errcheck
    - govet
    - staticcheck
    - ineffassign
    - unused
    - gocyclo
    - gocognit
    - unparam
    - unconvert
    - gocritic
    - gosec
    - misspell
    - nilerr
    - bodyclose
    - durationcheck
    - errorlint
    - noctx
    - revive
    - whitespace

  settings:
    gocyclo:
      min-complexity: 15
    gocognit:
      min-complexity: 20
    gocritic:
      enabled-tags:
        - diagnostic
        - performance
        - style
      disabled-checks:
        - whyNoLint
        - hugeParam
    revive:
      rules:
        - name: blank-imports
        - name: context-as-argument
        - name: context-keys-type
        - name: dot-imports
        - name: error-return
        - name: error-strings
        - name: error-naming
        - name: exported
        - name: increment-decrement
        - name: indent-error-flow
        - name: package-comments
        - name: range
        - name: receiver-naming
        - name: time-naming
        - name: unexported-return
        - name: var-declaration
        - name: var-naming
    gosec:
      excludes:
        - G104
        - G304
    misspell:
      locale: US
    errcheck:
      check-type-assertions: true
      check-blank: true
      exclude-functions:
        - io.Copy
        - (*os.File).Close
        - (io.Closer).Close

  exclusions:
    generated: lax
    rules:
      - path: _test\.go
        linters:
          - gocyclo
          - gocognit
          - errcheck
          - gosec
      - path: \.pb\.go$
        linters:
          - all
    paths:
      - vendor
      - testdata
      - .git

output:
  formats:
    text:
      path: stdout
  print-issued-lines: true
  print-linter-name: true
  sort-results: true
`

func ensureGolangciConfig() (string, bool, error) {
	configPath := ".golangci.yaml"
	if _, err := os.Stat(configPath); err == nil {
		return configPath, false, nil
	}
	if _, err := os.Stat(".golangci.yml"); err == nil {
		return ".golangci.yml", false, nil
	}
	if err := os.WriteFile(configPath, []byte(defaultGolangciConfig), 0o644); err != nil {
		return "", false, fmt.Errorf("creating %s: %w", configPath, err)
	}
	return configPath, true, nil
}

// Internal markdown lint issue types (not exported — converted to public types in results).
type mdIssue struct {
	file, rule, message, severity string
	line                          int
}

type fmIssue struct {
	file, message string
}

func runMarkdownLint(files []string, fix bool) ([]mdIssue, error) {
	cmdArgs := append([]string{}, files...)
	if fix {
		cmdArgs = append(cmdArgs, "--fix")
	}
	cmd := exec.CommandContext(context.Background(), "markdownlint-cli2", cmdArgs...)
	output, err := cmd.CombinedOutput()
	issues := parseMarkdownLintOutput(string(output))
	if err != nil && len(issues) == 0 && len(output) > 0 {
		return nil, fmt.Errorf("markdownlint-cli2 failed: %s", strings.TrimSpace(string(output)))
	}
	return issues, nil
}

func parseMarkdownLintOutput(output string) []mdIssue {
	var issues []mdIssue
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Finding:") || strings.HasPrefix(line, "Linting:") ||
			strings.HasPrefix(line, "Summary:") || strings.HasPrefix(line, "markdownlint-cli2") {
			continue
		}
		parts := strings.SplitN(line, " ", 2)
		if len(parts) < 2 {
			continue
		}
		locParts := strings.SplitN(parts[0], ":", 3)
		if len(locParts) < 2 {
			continue
		}
		lineNum := 0
		n, _ := fmt.Sscanf(locParts[1], "%d", &lineNum)
		if n == 0 || lineNum == 0 {
			continue
		}
		rest := parts[1]
		severity := "warning"
		if strings.HasPrefix(rest, "error ") {
			severity = "error"
			rest = rest[6:]
		} else if strings.HasPrefix(rest, "warning ") {
			rest = rest[8:]
		}
		msgParts := strings.SplitN(rest, " ", 2)
		rule := msgParts[0]
		message := ""
		if len(msgParts) > 1 {
			message = msgParts[1]
		}
		issues = append(issues, mdIssue{
			file: locParts[0], line: lineNum, rule: rule, message: message, severity: severity,
		})
	}
	return issues
}

func checkFrontmatter(files []string) ([]fmIssue, error) {
	required := []string{"title", "description"}
	var issues []fmIssue
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", file, err)
		}
		content := string(data)
		if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
			issues = append(issues, fmIssue{file: file, message: "missing frontmatter (must start with ---)"})
			continue
		}
		endIdx := strings.Index(content[4:], "\n---")
		if endIdx == -1 {
			issues = append(issues, fmIssue{file: file, message: "malformed frontmatter (missing closing ---)"})
			continue
		}
		fm := content[4 : 4+endIdx]
		for _, field := range required {
			found := false
			for _, line := range strings.Split(fm, "\n") {
				if strings.HasPrefix(strings.TrimSpace(line), field+":") {
					found = true
					break
				}
			}
			if !found {
				issues = append(issues, fmIssue{file: file, message: fmt.Sprintf("missing required frontmatter field: %s", field)})
			}
		}
	}
	return issues, nil
}

// endregion
