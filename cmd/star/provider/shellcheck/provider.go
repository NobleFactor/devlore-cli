// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

// Package shellcheck provides shell script analysis operations for the operation graph.
package shellcheck

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/iox"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

var _ op.Provider = (*Provider)(nil)

// Provider provides shell script analysis operations: lint (shellcheck), format (shfmt),
// parse (structural extraction), and complexity (cyclomatic metrics).
//
// +devlore:access=immediate
type Provider struct {
	op.ProviderBase
}

// NewProvider creates a shellcheck provider bound to the given context.
func NewProvider(ctx *op.RuntimeEnvironment) *Provider {
	return &Provider{ProviderBase: op.NewProviderBase(ctx)}
}

// Lint runs shellcheck on shell scripts and returns structured issues.
//
// +devlore:defaults severity="warning"
//
// Parameters:
//   - path: file or directory to lint
//   - severity: minimum severity level (error, warning, info, style)
//
// Returns:
//   - LintResult: issues grouped by severity with pass/fail
//   - error: if shellcheck is not installed or path is invalid
func (p *Provider) Lint(path, severity string) (LintResult, error) {
	if severity == "" {
		severity = "warning"
	}

	if _, err := exec.LookPath("shellcheck"); err != nil {
		return LintResult{}, fmt.Errorf("shellcheck not installed (install with your package manager)")
	}

	files, err := CollectShellFiles(path)
	if err != nil {
		return LintResult{}, err
	}

	var allIssues []LintIssue
	for _, file := range files {
		issues, lintErr := runShellcheck(file, severity)
		if lintErr != nil {
			if !strings.Contains(lintErr.Error(), "exit status") {
				continue
			}
		}
		allIssues = append(allIssues, issues...)
	}

	result := LintResult{Issues: allIssues, TotalCount: len(allIssues)}
	for _, issue := range allIssues {
		switch issue.Level {
		case "error":
			result.ErrorCount++
		case "warning":
			result.WarningCount++
		case "info":
			result.InfoCount++
		case "style":
			result.StyleCount++
		}
	}
	result.Passed = result.ErrorCount == 0 && result.WarningCount == 0

	return result, nil
}

// Format checks or fixes shell script formatting using shfmt.
//
// +devlore:defaults indent=0,fix=false
//
// Parameters:
//   - path: file or directory to check/format
//   - indent: indentation width (defaults to 4 when 0)
//   - fix: if true, rewrite files in place; if false, check only
//
// Returns:
//   - any: FormatCheckResult (fix=false) or FormatFixResult (fix=true)
//   - error: if shfmt is not installed or path is invalid
func (p *Provider) Format(path string, indent int, fix bool) (any, error) {
	if indent == 0 {
		indent = 4
	}

	if _, err := exec.LookPath("shfmt"); err != nil {
		return nil, fmt.Errorf("shfmt not installed (install with your package manager)")
	}

	files, err := CollectShellFiles(path)
	if err != nil {
		return nil, err
	}

	if fix {
		return formatFix(files, indent)
	}
	return formatCheck(files, indent)
}

// Parse parses shell scripts and extracts structural information.
//
// Parameters:
//   - path: file or directory to parse
//
// Returns:
//   - ParseResult: functions, variables, commands, sources, and line counts
//   - error: if path is invalid
func (p *Provider) Parse(path string) (ParseResult, error) {
	files, err := CollectShellFiles(path)
	if err != nil {
		return ParseResult{}, err
	}

	var result ParseResult
	for _, file := range files {
		parsed, parseErr := parseShellFile(file)
		if parseErr != nil {
			continue
		}
		result.Files = append(result.Files, parsed)
		result.TotalFunctions += len(parsed.Functions)
		result.TotalVariables += len(parsed.Variables)
		result.TotalLOC += parsed.LOC
		result.TotalSLOC += parsed.SLOC
	}

	return result, nil
}

// Complexity calculates complexity metrics for shell scripts.
//
// Parameters:
//   - path: file or directory to analyze
//
// Returns:
//   - ComplexityResult: per-function cyclomatic complexity, nesting, and hotspots
//   - error: if path is invalid
func (p *Provider) Complexity(path string) (ComplexityResult, error) {
	files, err := CollectShellFiles(path)
	if err != nil {
		return ComplexityResult{}, err
	}

	var result ComplexityResult
	for _, file := range files {
		cf, funcs := analyzeComplexity(file)
		result.Files = append(result.Files, cf)
		for _, fn := range funcs {
			result.TotalFunctions++
			result.TotalCyclomatic += fn.Cyclomatic
			if fn.Cyclomatic > result.MaxCyclomatic {
				result.MaxCyclomatic = fn.Cyclomatic
				result.MaxCycloFunc = fn.Name
			}
			if fn.Cyclomatic > 10 || fn.NestingDepth > 4 {
				result.Hotspots = append(result.Hotspots, Hotspot{
					Name:         fn.Name,
					File:         filepath.Base(file),
					Line:         fn.Line,
					Cyclomatic:   fn.Cyclomatic,
					NestingDepth: fn.NestingDepth,
					LOC:          fn.LOC,
				})
			}
		}
	}

	if result.TotalFunctions > 0 {
		result.AvgCyclomatic = float64(result.TotalCyclomatic) / float64(result.TotalFunctions)
	}

	return result, nil
}

// region UNEXPORTED FUNCTIONS

// runShellcheck executes shellcheck on a file and returns issues.
func runShellcheck(path, severity string) ([]LintIssue, error) {
	cmd := exec.CommandContext(context.Background(), "shellcheck", "-f", "json", "-x", "--severity="+severity, path)
	output, err := cmd.Output()
	if err != nil && len(output) == 0 {
		return nil, nil
	}
	if len(output) == 0 {
		return nil, nil
	}

	var issues []LintIssue
	if err := json.Unmarshal(output, &issues); err != nil {
		return nil, fmt.Errorf("parsing shellcheck output: %w", err)
	}
	return issues, nil
}

// formatCheck runs shfmt -d (diff mode) and returns failures.
func formatCheck(files []string, indent int) (FormatCheckResult, error) {
	result := FormatCheckResult{FilesChecked: len(files), Passed: true}
	for _, file := range files {
		cmd := exec.CommandContext(context.Background(), "shfmt", "-d", "-i", fmt.Sprintf("%d", indent), "-ci", file)
		output, err := cmd.CombinedOutput()
		if err != nil {
			result.Passed = false
			result.FilesFailed = append(result.FilesFailed, FormatFailedFile{
				File: file,
				Diff: string(output),
			})
		}
	}
	return result, nil
}

// formatFix runs shfmt -w (write mode) and returns formatted file count.
func formatFix(files []string, indent int) (FormatFixResult, error) {
	result := FormatFixResult{FilesChecked: len(files)}
	for _, file := range files {
		checkCmd := exec.CommandContext(context.Background(), "shfmt", "-d", "-i", fmt.Sprintf("%d", indent), "-ci", file)
		if output, err := checkCmd.CombinedOutput(); err != nil && len(output) > 0 {
			writeCmd := exec.CommandContext(context.Background(), "shfmt", "-w", "-i", fmt.Sprintf("%d", indent), "-ci", file)
			if err := writeCmd.Run(); err != nil {
				return FormatFixResult{}, fmt.Errorf("failed to format %s: %w", file, err)
			}
			result.FilesFormatted++
		}
	}
	return result, nil
}

// parseShellFile parses a single shell file using regex patterns.
func parseShellFile(path string) (ParsedFile, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return ParsedFile{}, err
	}

	result := ParsedFile{Path: path}
	lines := strings.Split(string(content), "\n")
	result.LOC = len(lines)

	inFunction := false
	functionStartLine := 0
	functionName := ""
	braceDepth := 0
	commandsSeen := make(map[string]bool)
	sourcesSeen := make(map[string]bool)

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			result.Blanks++
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			result.Comments++
			continue
		}

		if !inFunction {
			if matched := matchFunctionDef(trimmed); matched != "" {
				inFunction = true
				functionName = matched
				functionStartLine = lineNum
				braceDepth = strings.Count(line, "{") - strings.Count(line, "}")
				if braceDepth <= 0 {
					result.Functions = append(result.Functions, ShellFunction{
						Name: functionName, Line: functionStartLine, EndLine: lineNum, BodyLines: 1,
					})
					inFunction = false
				}
				continue
			}
		}

		if inFunction {
			braceDepth += strings.Count(line, "{") - strings.Count(line, "}")
			if braceDepth <= 0 {
				result.Functions = append(result.Functions, ShellFunction{
					Name: functionName, Line: functionStartLine, EndLine: lineNum, BodyLines: lineNum - functionStartLine + 1,
				})
				inFunction = false
			}
		}

		if varName, varValue := matchVariableAssign(trimmed); varName != "" {
			result.Variables = append(result.Variables, ShellVariable{Name: varName, Line: lineNum, Value: varValue})
		}

		if source := matchSourceCommand(trimmed); source != "" {
			if !sourcesSeen[source] {
				sourcesSeen[source] = true
				result.Sources = append(result.Sources, source)
			}
		}

		if cmd := matchExternalCommand(trimmed); cmd != "" {
			if !commandsSeen[cmd] {
				commandsSeen[cmd] = true
				result.Commands = append(result.Commands, cmd)
			}
		}
	}

	result.SLOC = result.LOC - result.Blanks - result.Comments
	return result, nil
}

// analyzeComplexity analyzes complexity for a shell file.
func analyzeComplexity(path string) (ComplexityFile, []FunctionComplexity) {
	content, err := os.ReadFile(path)
	if err != nil {
		return ComplexityFile{Path: path}, nil
	}

	lines := strings.Split(string(content), "\n")
	var functions []FunctionComplexity

	inFunction := false
	functionName := ""
	functionStartLine := 0
	braceDepth := 0
	var functionLines []string

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		if !inFunction {
			if matched := matchFunctionDef(trimmed); matched != "" {
				inFunction = true
				functionName = matched
				functionStartLine = lineNum
				braceDepth = strings.Count(line, "{") - strings.Count(line, "}")
				functionLines = []string{line}
				if braceDepth <= 0 {
					fc := calculateFunctionComplexity(functionName, functionStartLine, functionLines)
					functions = append(functions, fc)
					inFunction = false
					functionLines = nil
				}
				continue
			}
		}

		if inFunction {
			functionLines = append(functionLines, line)
			braceDepth += strings.Count(line, "{") - strings.Count(line, "}")
			if braceDepth <= 0 {
				fc := calculateFunctionComplexity(functionName, functionStartLine, functionLines)
				functions = append(functions, fc)
				inFunction = false
				functionLines = nil
			}
		}
	}

	cf := ComplexityFile{Path: path, FunctionCount: len(functions)}
	for _, fn := range functions {
		cf.TotalCyclo += fn.Cyclomatic
		cf.Functions = append(cf.Functions, fn)
	}

	return cf, functions
}

// calculateFunctionComplexity calculates complexity for a function body.
func calculateFunctionComplexity(name string, startLine int, lines []string) FunctionComplexity {
	fc := FunctionComplexity{Name: name, Line: startLine, LOC: len(lines), Cyclomatic: 1}

	currentNesting := 0
	maxNesting := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if strings.HasPrefix(trimmed, "if ") || strings.HasPrefix(trimmed, "elif ") {
			fc.Cyclomatic++
			currentNesting++
			if currentNesting > maxNesting {
				maxNesting = currentNesting
			}
		}

		if strings.HasPrefix(trimmed, "for ") || strings.HasPrefix(trimmed, "while ") || strings.HasPrefix(trimmed, "until ") {
			fc.Cyclomatic++
			currentNesting++
			if currentNesting > maxNesting {
				maxNesting = currentNesting
			}
		}

		if strings.HasSuffix(trimmed, ";;") {
			fc.Cyclomatic++
		}

		fc.Cyclomatic += strings.Count(trimmed, " && ")
		fc.Cyclomatic += strings.Count(trimmed, " || ")

		if trimmed == "fi" || trimmed == "done" || trimmed == "esac" {
			currentNesting--
			if currentNesting < 0 {
				currentNesting = 0
			}
		}

		for i := range 10 {
			fc.ParameterRefs += strings.Count(line, fmt.Sprintf("$%d", i))
		}
		fc.ParameterRefs += strings.Count(line, "$@")
		fc.ParameterRefs += strings.Count(line, "$*")
		fc.ParameterRefs += strings.Count(line, "$#")
	}

	fc.NestingDepth = maxNesting
	return fc
}

// matchFunctionDef matches shell function definitions and returns the function name.
func matchFunctionDef(line string) string {
	if strings.Contains(line, "()") {
		parts := strings.SplitN(line, "()", 2)
		if len(parts) >= 1 {
			name := strings.TrimSpace(parts[0])
			name = strings.TrimPrefix(name, "function ")
			if isValidFunctionName(name) {
				return name
			}
		}
	}
	if strings.HasPrefix(line, "function ") {
		rest := strings.TrimPrefix(line, "function ")
		parts := strings.Fields(rest)
		if len(parts) >= 1 && isValidFunctionName(parts[0]) {
			return parts[0]
		}
	}
	return ""
}

// isValidFunctionName checks if a string is a valid shell function name.
func isValidFunctionName(name string) bool {
	if name == "" {
		return false
	}
	for i, c := range name {
		if i == 0 {
			if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && c != '_' {
				return false
			}
		} else {
			if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' && c != '-' {
				return false
			}
		}
	}
	return true
}

// matchVariableAssign matches variable assignments.
func matchVariableAssign(line string) (name, value string) {
	work := line
	for _, prefix := range []string{"export ", "local ", "declare ", "readonly ", "typeset "} {
		if strings.HasPrefix(work, prefix) {
			work = strings.TrimPrefix(work, prefix)
			break
		}
	}
	if idx := strings.Index(work, "="); idx > 0 {
		name = strings.TrimSpace(work[:idx])
		if strings.ContainsAny(name, " \t[]()") {
			return "", ""
		}
		if isValidFunctionName(name) {
			value = strings.TrimSpace(work[idx+1:])
			return name, value
		}
	}
	return "", ""
}

// matchSourceCommand matches source/. commands.
func matchSourceCommand(line string) string {
	if strings.HasPrefix(line, "source ") {
		return strings.TrimSpace(strings.TrimPrefix(line, "source "))
	}
	if strings.HasPrefix(line, ". ") {
		return strings.TrimSpace(strings.TrimPrefix(line, ". "))
	}
	return ""
}

// matchExternalCommand extracts the first command word (excluding builtins).
func matchExternalCommand(line string) string {
	if strings.HasPrefix(line, "#") || strings.Contains(line, "=") {
		return ""
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	cmd := fields[0]

	builtins := map[string]bool{
		"if": true, "then": true, "else": true, "elif": true, "fi": true,
		"for": true, "do": true, "done": true, "while": true, "until": true,
		"case": true, "esac": true, "in": true,
		"function": true, "return": true, "exit": true,
		"local": true, "export": true, "declare": true, "readonly": true,
		"source": true, ".": true, "eval": true, "exec": true,
		"cd": true, "pwd": true, "echo": true, "printf": true,
		"read": true, "set": true, "unset": true, "shift": true,
		"test": true, "[": true, "[[": true, "true": true, "false": true,
		"{": true, "}": true, "(": true, ")": true,
		"break": true, "continue": true,
	}

	if builtins[cmd] {
		return ""
	}
	if len(cmd) > 0 && strings.ContainsAny(cmd[:1], "$\"'`-|&;<>()[]{}") {
		return ""
	}
	return cmd
}

// CollectShellFiles returns all shell files in a path.
func CollectShellFiles(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		if isShellFile(path) {
			return []string{path}, nil
		}
		return nil, nil
	}

	var files []string
	walkErr := filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "vendor" || d.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if !d.IsDir() && isShellFile(p) {
			files = append(files, p)
		}
		return nil
	})
	return files, walkErr
}

// isShellFile checks if a file is a shell script.
func isShellFile(path string) bool {

	ext := filepath.Ext(path)

	if ext == ".sh" || ext == ".bash" || ext == ".zsh" {
		return true
	}

	f, err := os.Open(path)

	if err != nil {
		return false
	}

	defer iox.Close(&err, f)

	buf := make([]byte, 256)
	n, err := f.Read(buf)

	if err != nil || n == 0 {
		return false
	}

	firstLine := string(buf[:n])
	if idx := strings.Index(firstLine, "\n"); idx > 0 {
		firstLine = firstLine[:idx]
	}

	if strings.HasPrefix(firstLine, "#!/") {
		if strings.Contains(firstLine, "/bash") ||
			strings.Contains(firstLine, "/sh") ||
			strings.Contains(firstLine, "/zsh") ||
			strings.Contains(firstLine, "env bash") ||
			strings.Contains(firstLine, "env sh") ||
			strings.Contains(firstLine, "env zsh") {
			return true
		}
	}

	return strings.Contains(firstLine, "# shellcheck shell=")
}

// endregion
