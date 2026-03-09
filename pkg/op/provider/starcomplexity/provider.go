// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package starcomplexity computes cyclomatic and cognitive complexity metrics
// for Starlark source files.
package starcomplexity

import (
	"os"
	"path/filepath"

	"github.com/NobleFactor/devlore-cli/pkg/op"
	"go.starlark.net/syntax"
)

// FunctionComplexity holds complexity metrics for a single function.
type FunctionComplexity struct {
	Name       string
	Line       int
	Cyclomatic int
	Cognitive  int
	MaxNesting int
	LOC        int
	Params     int
}

// FileComplexity holds complexity results for a single file.
type FileComplexity struct {
	Path      string
	Functions []FunctionComplexity
}

// ComplexityReport holds complexity results for all analyzed files.
type ComplexityReport struct {
	Files []FileComplexity
}

// complexityWalker computes cyclomatic and cognitive complexity using struct-based state tracking.
type complexityWalker struct {
	cyclomatic int
	cognitive  int
	nesting    int
	maxNesting int
}

// Provider computes cyclomatic and cognitive complexity metrics for Starlark source files.
//
// +devlore:access=immediate
type Provider struct {
	op.ProviderBase
	Root string
}

// ComputeComplexity analyzes the given files for function complexity.
func (p *Provider) ComputeComplexity(files []string) (*ComplexityReport, error) {
	report := &ComplexityReport{
		Files: make([]FileComplexity, 0, len(files)),
	}

	for _, absPath := range files {
		fc, err := analyzeFileComplexity(absPath, p.Root)
		if err != nil {
			return nil, err
		}
		report.Files = append(report.Files, *fc)
	}

	return report, nil
}

// analyzeFileComplexity parses a single file and computes complexity for each function definition.
func analyzeFileComplexity(absPath, root string) (*FileComplexity, error) {
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

	fc := &FileComplexity{Path: relPath}

	for _, stmt := range f.Stmts {
		def, ok := stmt.(*syntax.DefStmt)
		if !ok {
			continue
		}

		w := &complexityWalker{cyclomatic: 1}
		w.walkStmts(def.Body)

		fnLOC := countFunctionLOC(def)

		fc.Functions = append(fc.Functions, FunctionComplexity{
			Name:       def.Name.Name,
			Line:       int(def.Def.Line),
			Cyclomatic: w.cyclomatic,
			Cognitive:  w.cognitive,
			MaxNesting: w.maxNesting,
			LOC:        fnLOC,
			Params:     len(def.Params),
		})
	}

	return fc, nil
}

func (w *complexityWalker) walkStmts(stmts []syntax.Stmt) {
	for _, stmt := range stmts {
		w.walkStmt(stmt)
	}
}

func (w *complexityWalker) walkStmt(stmt syntax.Stmt) {
	switch s := stmt.(type) {
	case *syntax.IfStmt:
		// Each if/elif adds cyclomatic complexity
		w.cyclomatic++
		w.cognitive += 1 + w.nesting

		w.walkExpr(s.Cond)

		w.nesting++
		w.updateMaxNesting()
		w.walkStmts(s.True)
		w.nesting--

		if len(s.False) > 0 {
			// Check if the else branch is another if (elif)
			if len(s.False) == 1 {
				if _, isElif := s.False[0].(*syntax.IfStmt); isElif {
					w.walkStmts(s.False)
					return
				}
			}
			// else branch: cognitive cost but not cyclomatic
			w.cognitive++
			w.nesting++
			w.updateMaxNesting()
			w.walkStmts(s.False)
			w.nesting--
		}

	case *syntax.ForStmt:
		w.cyclomatic++
		w.cognitive += 1 + w.nesting

		w.nesting++
		w.updateMaxNesting()
		w.walkStmts(s.Body)
		w.nesting--

	case *syntax.WhileStmt:
		w.cyclomatic++
		w.cognitive += 1 + w.nesting

		w.walkExpr(s.Cond)

		w.nesting++
		w.updateMaxNesting()
		w.walkStmts(s.Body)
		w.nesting--

	case *syntax.DefStmt:
		// Nested function definition: adds nesting but not complexity
		w.nesting++
		w.updateMaxNesting()
		w.walkStmts(s.Body)
		w.nesting--

	case *syntax.ReturnStmt:
		if s.Result != nil {
			w.walkExpr(s.Result)
		}

	case *syntax.ExprStmt:
		w.walkExpr(s.X)

	case *syntax.AssignStmt:
		w.walkExpr(s.LHS)
		w.walkExpr(s.RHS)
	}
}

func (w *complexityWalker) walkExpr(expr syntax.Expr) {
	if expr == nil {
		return
	}

	switch e := expr.(type) {
	case *syntax.BinaryExpr:
		w.walkBinary(e)

	case *syntax.UnaryExpr:
		w.walkExpr(e.X)

	case *syntax.CondExpr:
		w.walkCond(e)

	case *syntax.CallExpr:
		w.walkExpr(e.Fn)
		w.walkExprs(e.Args)

	case *syntax.Comprehension:
		w.walkComprehension(e)

	case *syntax.IndexExpr:
		w.walkExpr(e.X)
		w.walkExpr(e.Y)

	case *syntax.SliceExpr:
		w.walkSlice(e)

	case *syntax.DotExpr:
		w.walkExpr(e.X)

	case *syntax.TupleExpr:
		w.walkExprs(e.List)

	case *syntax.ListExpr:
		w.walkExprs(e.List)

	case *syntax.DictExpr:
		w.walkDictEntries(e.List)

	case *syntax.ParenExpr:
		w.walkExpr(e.X)

	case *syntax.LambdaExpr:
		w.nesting++
		w.updateMaxNesting()
		w.walkExpr(e.Body)
		w.nesting--
	}
}

func (w *complexityWalker) walkExprs(exprs []syntax.Expr) {
	for _, expr := range exprs {
		w.walkExpr(expr)
	}
}

func (w *complexityWalker) walkDictEntries(entries []syntax.Expr) {
	for _, entry := range entries {
		if de, ok := entry.(*syntax.DictEntry); ok {
			w.walkExpr(de.Key)
			w.walkExpr(de.Value)
		}
	}
}

func (w *complexityWalker) walkSlice(e *syntax.SliceExpr) {
	w.walkExpr(e.X)
	w.walkExpr(e.Lo)
	w.walkExpr(e.Hi)
	w.walkExpr(e.Step)
}

func (w *complexityWalker) walkCond(e *syntax.CondExpr) {
	w.cyclomatic++
	w.cognitive += 1 + w.nesting
	w.walkExpr(e.Cond)
	w.walkExpr(e.True)
	w.walkExpr(e.False)
}

func (w *complexityWalker) walkBinary(e *syntax.BinaryExpr) {
	if e.Op == syntax.AND || e.Op == syntax.OR {
		w.cyclomatic++
		w.cognitive++
	}
	w.walkExpr(e.X)
	w.walkExpr(e.Y)
}

func (w *complexityWalker) walkComprehension(e *syntax.Comprehension) {
	w.cyclomatic++
	w.cognitive += 1 + w.nesting
	w.nesting++
	w.updateMaxNesting()
	w.walkExpr(e.Body)
	for _, clause := range e.Clauses {
		switch c := clause.(type) {
		case *syntax.ForClause:
			w.walkExpr(c.X)
		case *syntax.IfClause:
			w.cyclomatic++
			w.cognitive++
			w.walkExpr(c.Cond)
		}
	}
	w.nesting--
}

func (w *complexityWalker) updateMaxNesting() {
	if w.nesting > w.maxNesting {
		w.maxNesting = w.nesting
	}
}

// countFunctionLOC counts the lines occupied by a function definition, from the def keyword to the end of its body.
func countFunctionLOC(def *syntax.DefStmt) int {
	startLine := int(def.Def.Line)
	endLine := startLine

	walkStmtsForEndLine(def.Body, &endLine)

	return endLine - startLine + 1
}

// walkStmtsForEndLine recursively finds the maximum line number in a slice of statements.
func walkStmtsForEndLine(stmts []syntax.Stmt, maxLine *int) {
	for _, stmt := range stmts {
		walkStmtForEndLine(stmt, maxLine)
	}
}

func walkStmtForEndLine(stmt syntax.Stmt, maxLine *int) {
	_, end := stmt.Span()
	endLine := int(end.Line)
	if endLine > *maxLine {
		*maxLine = endLine
	}

	switch s := stmt.(type) {
	case *syntax.IfStmt:
		walkStmtsForEndLine(s.True, maxLine)
		walkStmtsForEndLine(s.False, maxLine)
	case *syntax.ForStmt:
		walkStmtsForEndLine(s.Body, maxLine)
	case *syntax.WhileStmt:
		walkStmtsForEndLine(s.Body, maxLine)
	case *syntax.DefStmt:
		walkStmtsForEndLine(s.Body, maxLine)
	}
}
