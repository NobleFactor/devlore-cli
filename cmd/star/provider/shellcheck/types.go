// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package shellcheck

// LintResult holds the outcome of a shellcheck lint run.
type LintResult struct {
	Issues       []LintIssue `starlark:"issues"`
	ErrorCount   int         `starlark:"error_count"`
	WarningCount int         `starlark:"warning_count"`
	InfoCount    int         `starlark:"info_count"`
	StyleCount   int         `starlark:"style_count"`
	TotalCount   int         `starlark:"total_count"`
	Passed       bool        `starlark:"passed"`
}

// LintIssue represents a single shellcheck finding.
type LintIssue struct {
	File      string `json:"file"      starlark:"file"`
	Line      int    `json:"line"      starlark:"line"`
	EndLine   int    `json:"endLine"   starlark:"end_line"`
	Column    int    `json:"column"    starlark:"column"`
	EndColumn int    `json:"endColumn" starlark:"end_column"`
	Level     string `json:"level"     starlark:"level"`
	Code      int    `json:"code"      starlark:"code"`
	Message   string `json:"message"   starlark:"message"`
}

// FormatCheckResult holds the outcome of a shfmt check (fix=false).
type FormatCheckResult struct {
	Passed       bool               `starlark:"passed"`
	FilesChecked int                `starlark:"files_checked"`
	FilesFailed  []FormatFailedFile `starlark:"files_failed"`
}

// FormatFailedFile represents a file that failed formatting check.
type FormatFailedFile struct {
	File string `starlark:"file"`
	Diff string `starlark:"diff"`
}

// FormatFixResult holds the outcome of a shfmt fix (fix=true).
type FormatFixResult struct {
	FilesChecked   int `starlark:"files_checked"`
	FilesFormatted int `starlark:"files_formatted"`
}

// ParseResult holds the parsed structure of shell scripts.
type ParseResult struct {
	Files          []ParsedFile `starlark:"files"`
	TotalFunctions int          `starlark:"total_functions"`
	TotalVariables int          `starlark:"total_variables"`
	TotalLOC       int          `starlark:"total_loc"`
	TotalSLOC      int          `starlark:"total_sloc"`
}

// ParsedFile holds the parsed structure of a single shell script.
type ParsedFile struct {
	Path      string          `starlark:"path"`
	Functions []ShellFunction `starlark:"functions"`
	Variables []ShellVariable `starlark:"variables"`
	Commands  []string        `starlark:"commands"`
	Sources   []string        `starlark:"sources"`
	LOC       int             `starlark:"loc"`
	SLOC      int             `starlark:"sloc"`
	Comments  int             `starlark:"comments"`
	Blanks    int             `starlark:"blanks"`
}

// ShellFunction represents a function defined in a shell script.
type ShellFunction struct {
	Name      string `starlark:"name"`
	Line      int    `starlark:"line"`
	EndLine   int    `starlark:"end_line"`
	BodyLines int    `starlark:"body_lines"`
}

// ShellVariable represents a variable assignment in a shell script.
type ShellVariable struct {
	Name  string `starlark:"name"`
	Line  int    `starlark:"line"`
	Value string `starlark:"value"`
}

// ComplexityResult holds complexity metrics for shell scripts.
type ComplexityResult struct {
	Files           []ComplexityFile `starlark:"files"`
	TotalCyclomatic int              `starlark:"total_cyclomatic"`
	TotalFunctions  int              `starlark:"total_functions"`
	AvgCyclomatic   float64          `starlark:"avg_cyclomatic"`
	MaxCyclomatic   int              `starlark:"max_cyclomatic"`
	MaxCycloFunc    string           `starlark:"max_cyclo_func"`
	Hotspots        []Hotspot        `starlark:"hotspots"`
}

// ComplexityFile holds complexity metrics for a single shell file.
type ComplexityFile struct {
	Path          string               `starlark:"path"`
	Functions     []FunctionComplexity `starlark:"functions"`
	TotalCyclo    int                  `starlark:"total_cyclo"`
	FunctionCount int                  `starlark:"function_count"`
}

// FunctionComplexity holds complexity metrics for a single function.
type FunctionComplexity struct {
	Name          string `starlark:"name"`
	Line          int    `starlark:"line"`
	Cyclomatic    int    `starlark:"cyclomatic"`
	NestingDepth  int    `starlark:"nesting_depth"`
	LOC           int    `starlark:"loc"`
	ParameterRefs int    `starlark:"parameter_refs"`
}

// Hotspot represents a function exceeding complexity thresholds.
type Hotspot struct {
	Name         string `starlark:"name"`
	File         string `starlark:"file"`
	Line         int    `starlark:"line"`
	Cyclomatic   int    `starlark:"cyclomatic"`
	NestingDepth int    `starlark:"nesting_depth"`
	LOC          int    `starlark:"loc"`
}
