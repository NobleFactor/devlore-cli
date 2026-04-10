// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package starindex provides AST-based indexing of Starlark source files, extracting functions, loads, globals, and
// line statistics.
package starindex

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/syntax"
)

// IndexedFunction describes a function definition found in a Starlark file.
type IndexedFunction struct {
	Name         string
	Line         int
	Params       int
	HasDocstring bool
	Docstring    string
}

// IndexedLoad describes a load statement found in a Starlark file.
type IndexedLoad struct {
	Module string
	Names  []string
	Line   int
}

// IndexedGlobal describes a top-level assignment found in a Starlark file.
type IndexedGlobal struct {
	Name string
	Line int
}

// IndexedFile holds the index results for a single file.
type IndexedFile struct {
	Path                        string
	Functions                   []IndexedFunction
	Loads                       []IndexedLoad
	Globals                     []IndexedGlobal
	LOC, SLOC, Comments, Blanks int
}

// IndexTotals aggregates index counts across all files.
type IndexTotals struct {
	FileCount, Functions, Loads, Globals int
	LOC, SLOC, Comments, Blanks          int
}

// Index holds the index results for all captured files.
type Index struct {
	Files  []IndexedFile
	Totals IndexTotals
}

// Provider performs AST-based indexing of Starlark source files.
//
// +devlore:access=immediate
type Provider struct {
	op.ProviderBase
	Root string
}

func NewProvider(ctx *op.ExecutionContext) *Provider {
	p := &Provider{ProviderBase: op.NewProviderBase(ctx)}
	if ctx.Root != nil {
		p.Root = ctx.Root.Name()
	}
	return p
}

// IndexFiles parses all files and extracts functions, loads, and globals.
// If withDocstrings is true, function docstrings are extracted.
// If withGlobals is true, top-level assignments are captured.
func (p *Provider) IndexFiles(files []string, withDocstrings, withGlobals bool) (*Index, error) {
	result := &Index{
		Files: make([]IndexedFile, 0, len(files)),
	}

	for _, absPath := range files {
		indexed, err := indexFile(absPath, p.Root, withDocstrings, withGlobals)
		if err != nil {
			return nil, err
		}
		result.Files = append(result.Files, *indexed)
		result.Totals.FileCount++
		result.Totals.Functions += len(indexed.Functions)
		result.Totals.Loads += len(indexed.Loads)
		result.Totals.Globals += len(indexed.Globals)
		result.Totals.LOC += indexed.LOC
		result.Totals.SLOC += indexed.SLOC
		result.Totals.Comments += indexed.Comments
		result.Totals.Blanks += indexed.Blanks
	}

	return result, nil
}

// indexFile parses a single file and extracts its index information.
func indexFile(absPath, root string, withDocstrings, withGlobals bool) (*IndexedFile, error) {
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, err
	}

	relPath, err := filepath.Rel(root, absPath)
	if err != nil {
		relPath = absPath
	}

	opts := syntax.FileOptions{}
	f, err := opts.Parse(relPath, data, 0)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, fmt.Errorf("parse returned nil file for %s", relPath)
	}

	loc, sloc, comments, blanks := countLines(data)

	indexed := &IndexedFile{
		Path:     relPath,
		LOC:      loc,
		SLOC:     sloc,
		Comments: comments,
		Blanks:   blanks,
	}

	indexStmts(indexed, f.Stmts, withDocstrings, withGlobals)

	return indexed, nil
}

// indexStmts processes parsed statements to populate function, load, and global entries.
func indexStmts(indexed *IndexedFile, stmts []syntax.Stmt, withDocstrings, withGlobals bool) {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *syntax.DefStmt:
			fn := IndexedFunction{
				Name:   s.Name.Name,
				Line:   int(s.Def.Line),
				Params: len(s.Params),
			}
			if withDocstrings {
				fn.Docstring = extractDocstring(s)
				fn.HasDocstring = fn.Docstring != ""
			}
			indexed.Functions = append(indexed.Functions, fn)

		case *syntax.LoadStmt:
			module, ok := s.Module.Value.(string)
			if !ok {
				continue
			}
			names := make([]string, len(s.From))
			for i, ident := range s.From {
				names[i] = ident.Name
			}
			indexed.Loads = append(indexed.Loads, IndexedLoad{
				Module: module,
				Names:  names,
				Line:   int(s.Load.Line),
			})

		case *syntax.AssignStmt:
			if withGlobals {
				if name := extractAssignName(s); name != "" {
					indexed.Globals = append(indexed.Globals, IndexedGlobal{
						Name: name,
						Line: int(s.OpPos.Line),
					})
				}
			}
		}
	}
}

// extractDocstring extracts the docstring from a function definition.
// A docstring is the first statement in the function body if it's a string literal expression.
func extractDocstring(def *syntax.DefStmt) string {
	if len(def.Body) == 0 {
		return ""
	}

	exprStmt, ok := def.Body[0].(*syntax.ExprStmt)
	if !ok {
		return ""
	}

	lit, ok := exprStmt.X.(*syntax.Literal)
	if !ok || lit.Token != syntax.STRING {
		return ""
	}

	s, ok := lit.Value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

// extractAssignName returns the name of a top-level assignment if the LHS is a simple identifier.
// Returns empty string for tuple unpacking or attribute assignments.
func extractAssignName(assign *syntax.AssignStmt) string {
	ident, ok := assign.LHS.(*syntax.Ident)
	if !ok {
		return ""
	}
	return ident.Name
}

// countLines classifies each line as blank, comment, or code (SLOC).
func countLines(data []byte) (loc, sloc, comments, blanks int) {
	if len(data) == 0 {
		return 0, 0, 0, 0
	}

	lines := bytes.Split(data, []byte("\n"))

	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}

	loc = len(lines)
	for _, line := range lines {
		trimmed := strings.TrimSpace(string(line))
		switch {
		case trimmed == "":
			blanks++
		case strings.HasPrefix(trimmed, "#"):
			comments++
		default:
			sloc++
		}
	}
	return loc, sloc, comments, blanks
}
