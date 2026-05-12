// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package function

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

	var builder strings.Builder

	// Header comment

	pos := fn.Position()
	if pos.IsValid() {
		_, _ = fmt.Fprintf(&builder, "# Extracted callable — from %s\n", pos)
	}

	// Closure bindings

	if fn.NumFreeVars() > 0 {

		builder.WriteString("# Closure bindings:\n")

		for i := range fn.NumFreeVars() {

			binding, val := fn.FreeVar(i)

			lit, err := FormatLiteral(val)
			if err != nil {
				return nil, fmt.Errorf("closure binding %q: %w", binding.Name, err)
			}

			_, _ = fmt.Fprintf(&builder, "%s = %s\n", binding.Name, lit)
		}

		builder.WriteString("\n")
	}

	// Function body

	if fn.Name() == "lambda" {

		body, err := extractLambdaExpr(fn)
		if err != nil {
			return nil, fmt.Errorf("lambda extraction: %w", err)
		}

		_, _ = fmt.Fprintf(&builder, "def _callable(%s):\n", strings.Join(params, ", "))
		_, _ = fmt.Fprintf(&builder, "    return %s\n", body)

	} else {

		defText, err := extractDefStmt(fn)
		if err != nil {
			return nil, fmt.Errorf("def extraction: %w", err)
		}

		builder.WriteString(defText)

		if !strings.HasSuffix(defText, "\n") {
			builder.WriteString("\n")
		}
	}

	return []byte(builder.String()), nil
}

// extractLambdaExpr returns the body text of fn's lambda expression.
//
// Reads the source at fn.Position(), parses it, and looks up the [*syntax.LambdaExpr] anchored at that position via
// [extractNodeAt]. Returns the text between the lambda body's span endpoints, with surrounding whitespace trimmed.
//
// Parameters:
//   - fn: starlark function whose source position is a lambda.
//
// Returns:
//   - string: the lambda body expression text, whitespace-trimmed.
//   - error: invalid source position, read failure, parse failure, or "not found" when no [*syntax.LambdaExpr]
//     anchors at fn.Position().
func extractLambdaExpr(fn *starlark.Function) (string, error) {

	pos := fn.Position()
	if !pos.IsValid() {
		return "", fmt.Errorf("lambda has no source position")
	}

	data, lambdaExpr, err := extractNodeAt[*syntax.LambdaExpr](pos)
	if err != nil {
		return "", err
	}

	if lambdaExpr == nil {
		return "", fmt.Errorf("lambda not found at %s", pos)
	}

	bodyStart, bodyEnd := lambdaExpr.Body.Span()
	body := extractSpan(data, bodyStart, bodyEnd)

	return strings.TrimSpace(body), nil
}

// extractDefStmt reads the source file and extracts the full def statement.
//
// Parameters:
//   - fn: starlark function whose source to extract.
//
// Returns:
//   - string: the full def statement text.
//   - error: any read or parse error.
func extractDefStmt(fn *starlark.Function) (string, error) {

	pos := fn.Position()
	if !pos.IsValid() {
		return "", fmt.Errorf("function has no source position")
	}

	data, defStmt, err := extractNodeAt[*syntax.DefStmt](pos)
	if err != nil {
		return "", err
	}

	if defStmt == nil {
		return "", fmt.Errorf("def %s not found at %s", fn.Name(), pos)
	}

	start, end := defStmt.Span()
	return extractSpan(data, start, end), nil
}

// extractNodeAt returns the AST node of type T at pos.
//
// Reads pos.Filename(), parses it with [syntax.FileOptions.Parse], and walks the resulting tree for the first node
// satisfying T whose Span starts at the given Line and Col. The walk stops on the first match.
//
// Shared by [extractLambdaExpr] and [extractDefStmt]; each caller instantiates T with the concrete pointer type it
// wants ([*syntax.LambdaExpr], [*syntax.DefStmt]). The constraint `T syntax.Node` admits any type satisfying
// [syntax.Node]; callers in practice use pointer types so the zero value is nil.
//
// When no matching node is found, returns the parsed source bytes, the zero value of T, and a nil error. Callers
// distinguish "not found" from "I/O failure" by checking the returned T against its zero value.
//
// Parameters:
//   - pos: source position whose Filename names the file to read and whose Line+Col anchor the node to find.
//
// Returns:
//   - []byte: the file's bytes (returned even when no match is found, so callers can use [extractSpan] on the result).
//   - T: the matched node, or the zero value of T when no node at pos has type T.
//   - error: read failure or parse failure; nil otherwise (including the no-match case).
func extractNodeAt[T syntax.Node](pos syntax.Position) ([]byte, T, error) {

	var result T

	data, err := os.ReadFile(pos.Filename())
	if err != nil {
		return nil, result, fmt.Errorf("read source %s: %w", pos.Filename(), err)
	}

	f, err := new(syntax.FileOptions).Parse(pos.Filename(), data, 0)
	if err != nil {
		return nil, result, fmt.Errorf("parse source %s: %w", pos.Filename(), err)
	}

	syntax.Walk(f, func(node syntax.Node) bool {

		if typed, ok := node.(T); ok {
			start, _ := typed.Span()
			if start.Line == pos.Line && start.Col == pos.Col {
				result = typed
				return false
			}
		}

		return true
	})

	return data, result, nil
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
