// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package mem

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

// Extract introspects a *starlark.Function and produces a self-contained Callable with synthesized source text. The
// synthetic file inlines all closure bindings as module-level constants, making it independent of the original script.
//
// The function name is derived from fn.Name() (or "<action>.<param>" for lambdas when the caller provides a fallback
// via ExtractWithName). When root is non-nil, source reads are scoped through os.Root.
//
// Parameters:
//   - fn: Starlark function to extract
//   - funcType: Go type name the callable satisfies (e.g., "file.Reducer")
//   - root: OS root for scoped I/O (nil falls back to os.ReadFile)
//
// Returns:
//   - *Callable: the extracted callable with synthesized source
//   - error: any extraction error
func Extract(fn *starlark.Function, funcType string, root *os.Root) (*Callable, error) {

	name := fn.Name()
	if name == "lambda" {
		name = "_lambda"
	}
	return ExtractWithName(fn, funcType, name, root)
}

// ExtractWithName is like Extract but allows the caller to specify the callable name (useful for lambdas where the
// default name is "lambda"). When root is non-nil, source reads are scoped through os.Root.
//
// Parameters:
//   - fn: Starlark function to extract
//   - funcType: Go type name the callable satisfies (e.g., "file.Reducer")
//   - name: Name for the callable (overrides fn.Name())
//   - root: OS root for scoped I/O (nil falls back to os.ReadFile)
//
// Returns:
//   - *Callable: the extracted callable with synthesized source
//   - error: any extraction error
func ExtractWithName(fn *starlark.Function, funcType, name string, root *os.Root) (*Callable, error) {
	c := NewCallable(funcType, name)

	// Introspect parameters.
	params := make([]string, fn.NumParams())
	for i := range fn.NumParams() {
		p, _ := fn.Param(i)
		params[i] = p
	}
	c.ParamNames = params
	c.NumParams = fn.NumParams()

	// Record original position for diagnostics.
	pos := fn.Position()
	if pos.IsValid() {
		c.OriginalPos = pos.String()
	}

	// Build synthetic source.
	source, err := synthesize(fn, params, root)
	if err != nil {
		return nil, fmt.Errorf("extract %s: %w", name, err)
	}
	c.SetSource(source)

	// Set the function name in the synthetic file.
	if fn.Name() == "lambda" {
		c.FuncName = "_callable"
	} else {
		c.FuncName = fn.Name()
	}

	return c, nil
}

// synthesize builds the synthetic source file text. When root is non-nil, source reads are scoped through os.Root.
//
// Parameters:
//   - fn: Starlark function whose source to extract and synthesize
//   - params: Parameter names for the function
//   - root: OS root for scoped I/O (nil falls back to os.ReadFile)
//
// Returns:
//   - []byte: synthetic source text
//   - error: any extraction error
func synthesize(fn *starlark.Function, params []string, root *os.Root) ([]byte, error) {
	var b strings.Builder

	// Header comment.
	pos := fn.Position()
	if pos.IsValid() {
		fmt.Fprintf(&b, "# Extracted callable — from %s\n", pos)
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
			fmt.Fprintf(&b, "%s = %s\n", binding.Name, lit)
		}
		b.WriteString("\n")
	}

	// Function body.
	if fn.Name() == "lambda" {
		// For lambdas, we need to extract the expression from source and
		// wrap it in a def. If source extraction fails, we fall back to
		// a stub that documents the issue.
		body, err := extractLambdaBody(fn, root)
		if err != nil {
			return nil, fmt.Errorf("lambda extraction: %w", err)
		}
		fmt.Fprintf(&b, "def _callable(%s):\n", strings.Join(params, ", "))
		fmt.Fprintf(&b, "    return %s\n", body)
	} else {
		// For named functions, extract the full def from source.
		defText, err := extractDefSource(fn, root)
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
// fn.Position(). When root is non-nil, reads are scoped through os.Root.
//
// Parameters:
//   - fn: Starlark function whose source to extract
//   - root: OS root for scoped I/O (nil falls back to os.ReadFile)
//
// Returns:
//   - string: the lambda body expression text
//   - error: any read or parse error
func extractLambdaBody(fn *starlark.Function, root *os.Root) (string, error) {

	pos := fn.Position()
	if !pos.IsValid() {
		return "", fmt.Errorf("lambda has no source position")
	}

	data, err := readSource(pos.Filename(), root)
	if err != nil {
		return "", fmt.Errorf("read source %s: %w", pos.Filename(), err)
	}

	// Parse the source file to find the lambda expression at the given position.
	f, err := syntax.Parse(pos.Filename(), data, 0)
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
			if leStart.Line == int32(pos.Line) && leStart.Col == int32(pos.Col) {
				lambdaExpr = le
				return false
			}
		}
		return true
	})

	if lambdaExpr == nil {
		return "", fmt.Errorf("lambda not found at %s", pos)
	}

	// Extract the body expression text from source bytes.
	bodyStart, bodyEnd := lambdaExpr.Body.Span()
	body := extractSpan(data, bodyStart, bodyEnd)
	return strings.TrimSpace(body), nil
}

// extractDefSource reads the source file and extracts the full def statement. When root is non-nil, reads are scoped
// through os.Root.
//
// Parameters:
//   - fn: Starlark function whose source to extract
//   - root: OS root for scoped I/O (nil falls back to os.ReadFile)
//
// Returns:
//   - string: the full def statement text
//   - error: any read or parse error
func extractDefSource(fn *starlark.Function, root *os.Root) (string, error) {

	pos := fn.Position()
	if !pos.IsValid() {
		return "", fmt.Errorf("function has no source position")
	}

	data, err := readSource(pos.Filename(), root)
	if err != nil {
		return "", fmt.Errorf("read source %s: %w", pos.Filename(), err)
	}

	f, err := syntax.Parse(pos.Filename(), data, 0)
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
			if dsStart.Line == int32(pos.Line) && dsStart.Col == int32(pos.Col) {
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
func extractSpan(data []byte, start, end syntax.Position) string {
	lines := strings.Split(string(data), "\n")

	startLine := int(start.Line) - 1 // 1-indexed to 0-indexed
	endLine := int(end.Line) - 1
	startCol := int(start.Col) - 1 // 1-indexed inclusive → 0-indexed inclusive (slice start)
	endCol := int(end.Col)         // 1-indexed inclusive → 0-indexed exclusive (slice end)

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
	// First line from startCol to end.
	if startCol < len(lines[startLine]) {
		b.WriteString(lines[startLine][startCol:])
	}
	b.WriteString("\n")
	// Middle lines in full.
	for i := startLine + 1; i < endLine; i++ {
		b.WriteString(lines[i])
		b.WriteString("\n")
	}
	// Last line from start to endCol.
	if endLine < len(lines) {
		lastLine := lines[endLine]
		if endCol > len(lastLine) {
			endCol = len(lastLine)
		}
		b.WriteString(lastLine[:endCol])
	}

	return b.String()
}

// readSource reads a source file, using root-scoped I/O when root is available and the file is within root. Falls back
// to os.ReadFile otherwise.
//
// Parameters:
//   - filename: Absolute path to the source file
//   - root: OS root for scoped I/O (nil falls back to os.ReadFile)
//
// Returns:
//   - []byte: file contents
//   - error: any read error
func readSource(filename string, root *os.Root) ([]byte, error) {

	if root != nil {
		rel, err := filepath.Rel(root.Name(), filename)
		if err == nil && rel != ".." && (len(rel) <= 2 || rel[:3] != "../") {
			return root.ReadFile(rel)
		}
	}

	return os.ReadFile(filename)
}

// ValidateArity checks that a function's arity is compatible with the
// target action's expected parameter range.
func ValidateArity(fn *starlark.Function, minParams, maxParams int) error {
	numRequired := 0
	for i := range fn.NumParams() {
		if fn.ParamDefault(i) == nil {
			numRequired++
		}
	}
	if numRequired > maxParams {
		return fmt.Errorf("%s requires %d args but target accepts at most %d",
			fn.Name(), numRequired, maxParams)
	}
	if fn.NumParams() < minParams && !fn.HasVarargs() {
		return fmt.Errorf("%s accepts %d args but target requires at least %d",
			fn.Name(), fn.NumParams(), minParams)
	}
	return nil
}
