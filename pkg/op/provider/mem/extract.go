// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package mem

import (
	"fmt"
	"os"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// synthesize builds the synthetic source file text.
//
// Parameters:
//   - fn: starlark function whose source to extract and synthesize.
//   - params: parameter names for the function.
//
// Returns:
//   - []byte: synthetic source text.
//   - error: any extraction error.
func synthesize(fn *starlark.Function, params []string) ([]byte, error) {

	var b strings.Builder

	// Header comment.
	pos := fn.Position()
	if pos.IsValid() {
		_, _ = fmt.Fprintf(&b, "# Extracted callable — from %s\n", pos)
	}

	// Closure bindings as module-level constants.
	if fn.NumFreeVars() > 0 {
		b.WriteString("# Closure bindings:\n")
		for i := range fn.NumFreeVars() {
			binding, val := fn.FreeVar(i)
			lit, err := FormatLiteral(val)
			if err != nil {
				return nil, fmt.Errorf("closure binding %q: %w", binding.Name, err)
			}
			_, _ = fmt.Fprintf(&b, "%s = %s\n", binding.Name, lit)
		}
		b.WriteString("\n")
	}

	// Function body.
	if fn.Name() == "lambda" {
		body, err := extractLambdaBody(fn)
		if err != nil {
			return nil, fmt.Errorf("lambda extraction: %w", err)
		}
		_, _ = fmt.Fprintf(&b, "def _callable(%s):\n", strings.Join(params, ", "))
		_, _ = fmt.Fprintf(&b, "    return %s\n", body)
	} else {
		defText, err := extractDefSource(fn)
		if err != nil {
			return nil, fmt.Errorf("def extraction: %w", err)
		}
		b.WriteString(defText)
		if !strings.HasSuffix(defText, "\n") {
			b.WriteString("\n")
		}
	}

	return []byte(b.String()), nil
}

// extractLambdaBody reads the source file and extracts the lambda expression body from the position indicated by
// fn.Position().
//
// Parameters:
//   - fn: starlark function whose source to extract.
//
// Returns:
//   - string: the lambda body expression text.
//   - error: any read or parse error.
func extractLambdaBody(fn *starlark.Function) (string, error) {

	pos := fn.Position()
	if !pos.IsValid() {
		return "", fmt.Errorf("lambda has no source position")
	}

	data, err := os.ReadFile(pos.Filename())
	if err != nil {
		return "", fmt.Errorf("read source %s: %w", pos.Filename(), err)
	}

	f, err := new(syntax.FileOptions).Parse(pos.Filename(), data, 0)
	if err != nil {
		return "", fmt.Errorf("parse source %s: %w", pos.Filename(), err)
	}

	var lambdaExpr *syntax.LambdaExpr
	syntax.Walk(f, func(n syntax.Node) bool {
		if lambdaExpr != nil {
			return false
		}
		if le, ok := n.(*syntax.LambdaExpr); ok {
			leStart, _ := le.Span()
			if leStart.Line == pos.Line && leStart.Col == pos.Col {
				lambdaExpr = le
				return false
			}
		}
		return true
	})

	if lambdaExpr == nil {
		return "", fmt.Errorf("lambda not found at %s", pos)
	}

	bodyStart, bodyEnd := lambdaExpr.Body.Span()
	body := extractSpan(data, bodyStart, bodyEnd)
	return strings.TrimSpace(body), nil
}

// extractDefSource reads the source file and extracts the full def statement.
//
// Parameters:
//   - fn: starlark function whose source to extract.
//
// Returns:
//   - string: the full def statement text.
//   - error: any read or parse error.
func extractDefSource(fn *starlark.Function) (string, error) {

	pos := fn.Position()
	if !pos.IsValid() {
		return "", fmt.Errorf("function has no source position")
	}

	data, err := os.ReadFile(pos.Filename())
	if err != nil {
		return "", fmt.Errorf("read source %s: %w", pos.Filename(), err)
	}

	f, err := new(syntax.FileOptions).Parse(pos.Filename(), data, 0)
	if err != nil {
		return "", fmt.Errorf("parse source %s: %w", pos.Filename(), err)
	}

	var defStmt *syntax.DefStmt
	syntax.Walk(f, func(n syntax.Node) bool {
		if defStmt != nil {
			return false
		}
		if ds, ok := n.(*syntax.DefStmt); ok {
			dsStart, _ := ds.Span()
			if dsStart.Line == pos.Line && dsStart.Col == pos.Col {
				defStmt = ds
				return false
			}
		}
		return true
	})

	if defStmt == nil {
		return "", fmt.Errorf("def %s not found at %s", fn.Name(), pos)
	}

	start, end := defStmt.Span()
	return extractSpan(data, start, end), nil
}

// extractSpan extracts text from source bytes between two positions.
//
// Parameters:
//   - data: the source file bytes.
//   - start: the start position.
//   - end: the end position.
//
// Returns:
//   - string: the extracted text.
func extractSpan(data []byte, start, end syntax.Position) string {

	lines := strings.Split(string(data), "\n")

	startLine := int(start.Line) - 1
	endLine := int(end.Line) - 1
	startCol := int(start.Col) - 1
	endCol := int(end.Col)

	if startLine < 0 || startLine >= len(lines) {
		return ""
	}
	if endLine < 0 || endLine >= len(lines) {
		endLine = len(lines) - 1
		endCol = len(lines[endLine])
	}

	if startLine == endLine {
		line := lines[startLine]
		if startCol > len(line) {
			startCol = len(line)
		}
		if endCol > len(line) {
			endCol = len(line)
		}
		return line[startCol:endCol]
	}

	var b strings.Builder
	if startCol < len(lines[startLine]) {
		b.WriteString(lines[startLine][startCol:])
	}
	b.WriteString("\n")
	for i := startLine + 1; i < endLine; i++ {
		b.WriteString(lines[i])
		b.WriteString("\n")
	}
	if endLine < len(lines) {
		lastLine := lines[endLine]
		if endCol > len(lastLine) {
			endCol = len(lastLine)
		}
		b.WriteString(lastLine[:endCol])
	}

	return b.String()
}
