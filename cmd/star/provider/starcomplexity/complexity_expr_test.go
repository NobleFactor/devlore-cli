// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package starcomplexity

import (
	"testing"
)

// captureExpressions returns a complexity report for testdata/expressions.star.
func captureExpressions(t *testing.T) map[string]FunctionComplexity {
	t.Helper()
	root := testdataDir(t)
	files := captureFiles(t, root, "expressions.star")

	report, err := (&Provider{Root: root}).ComputeComplexity(files)
	if err != nil {
		t.Fatalf("ComputeComplexity: %v", err)
	}

	if len(report.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(report.Files))
	}

	funcMap := make(map[string]FunctionComplexity)
	for _, fn := range report.Files[0].Functions {
		funcMap[fn.Name] = fn
	}
	return funcMap
}

// TestComplexityTernary verifies walkCond is exercised by a ternary expression.
func TestComplexityTernary(t *testing.T) {
	funcMap := captureExpressions(t)

	fn, ok := funcMap["use_ternary"]
	if !ok {
		t.Fatal("use_ternary not found")
	}
	// ternary adds +1 cyclomatic
	if fn.Cyclomatic != 2 {
		t.Errorf("use_ternary cyclomatic = %d, want 2", fn.Cyclomatic)
	}
	if fn.Cognitive < 1 {
		t.Errorf("use_ternary cognitive = %d, want >= 1", fn.Cognitive)
	}
}

// TestComplexityLambda verifies the LambdaExpr case in walkExpr is exercised.
func TestComplexityLambda(t *testing.T) {
	funcMap := captureExpressions(t)

	fn, ok := funcMap["use_lambda"]
	if !ok {
		t.Fatal("use_lambda not found")
	}
	// lambda adds nesting but no cyclomatic complexity
	if fn.Cyclomatic != 1 {
		t.Errorf("use_lambda cyclomatic = %d, want 1", fn.Cyclomatic)
	}
	if fn.MaxNesting != 1 {
		t.Errorf("use_lambda maxNesting = %d, want 1", fn.MaxNesting)
	}
}

// TestComplexityDict verifies walkDictEntries is exercised by a dict literal.
func TestComplexityDict(t *testing.T) {
	funcMap := captureExpressions(t)

	fn, ok := funcMap["use_dict"]
	if !ok {
		t.Fatal("use_dict not found")
	}
	// dict literal has no control flow
	if fn.Cyclomatic != 1 {
		t.Errorf("use_dict cyclomatic = %d, want 1", fn.Cyclomatic)
	}
}

// TestComplexitySlice verifies walkSlice is exercised by a slice expression.
func TestComplexitySlice(t *testing.T) {
	funcMap := captureExpressions(t)

	fn, ok := funcMap["use_slice"]
	if !ok {
		t.Fatal("use_slice not found")
	}
	if fn.Cyclomatic != 1 {
		t.Errorf("use_slice cyclomatic = %d, want 1", fn.Cyclomatic)
	}
}

// TestComplexityTuple verifies walkExprs via TupleExpr is exercised.
func TestComplexityTuple(t *testing.T) {
	funcMap := captureExpressions(t)

	fn, ok := funcMap["use_tuple"]
	if !ok {
		t.Fatal("use_tuple not found")
	}
	if fn.Cyclomatic != 1 {
		t.Errorf("use_tuple cyclomatic = %d, want 1", fn.Cyclomatic)
	}
}

// TestComplexityList verifies walkExprs via ListExpr is exercised.
func TestComplexityList(t *testing.T) {
	funcMap := captureExpressions(t)

	fn, ok := funcMap["use_list"]
	if !ok {
		t.Fatal("use_list not found")
	}
	if fn.Cyclomatic != 1 {
		t.Errorf("use_list cyclomatic = %d, want 1", fn.Cyclomatic)
	}
}

// TestComplexityIndex verifies IndexExpr is exercised.
func TestComplexityIndex(t *testing.T) {
	funcMap := captureExpressions(t)

	fn, ok := funcMap["use_index"]
	if !ok {
		t.Fatal("use_index not found")
	}
	if fn.Cyclomatic != 1 {
		t.Errorf("use_index cyclomatic = %d, want 1", fn.Cyclomatic)
	}
}

// TestComplexityDot verifies DotExpr is exercised.
func TestComplexityDot(t *testing.T) {
	funcMap := captureExpressions(t)

	fn, ok := funcMap["use_dot"]
	if !ok {
		t.Fatal("use_dot not found")
	}
	if fn.Cyclomatic != 1 {
		t.Errorf("use_dot cyclomatic = %d, want 1", fn.Cyclomatic)
	}
}

// TestComplexityParen verifies ParenExpr is exercised.
func TestComplexityParen(t *testing.T) {
	funcMap := captureExpressions(t)

	fn, ok := funcMap["use_paren"]
	if !ok {
		t.Fatal("use_paren not found")
	}
	if fn.Cyclomatic != 1 {
		t.Errorf("use_paren cyclomatic = %d, want 1", fn.Cyclomatic)
	}
}

// TestComplexityUnary verifies UnaryExpr (not) is exercised.
func TestComplexityUnary(t *testing.T) {
	funcMap := captureExpressions(t)

	fn, ok := funcMap["use_unary"]
	if !ok {
		t.Fatal("use_unary not found")
	}
	if fn.Cyclomatic != 1 {
		t.Errorf("use_unary cyclomatic = %d, want 1", fn.Cyclomatic)
	}
}

// TestComplexityCall verifies CallExpr with arguments is exercised.
func TestComplexityCall(t *testing.T) {
	funcMap := captureExpressions(t)

	fn, ok := funcMap["use_call"]
	if !ok {
		t.Fatal("use_call not found")
	}
	if fn.Cyclomatic != 1 {
		t.Errorf("use_call cyclomatic = %d, want 1", fn.Cyclomatic)
	}
}

// TestComplexityWhile verifies WhileStmt is exercised.
func TestComplexityWhile(t *testing.T) {
	funcMap := captureExpressions(t)

	fn, ok := funcMap["use_while"]
	if !ok {
		t.Fatal("use_while not found")
	}
	// while adds +1 cyclomatic
	if fn.Cyclomatic != 2 {
		t.Errorf("use_while cyclomatic = %d, want 2", fn.Cyclomatic)
	}
	if fn.Cognitive < 1 {
		t.Errorf("use_while cognitive = %d, want >= 1", fn.Cognitive)
	}
	if fn.MaxNesting != 1 {
		t.Errorf("use_while maxNesting = %d, want 1", fn.MaxNesting)
	}
}

// TestComplexityNestedDef verifies nested DefStmt is exercised.
func TestComplexityNestedDef(t *testing.T) {
	funcMap := captureExpressions(t)

	fn, ok := funcMap["use_nested_def"]
	if !ok {
		t.Fatal("use_nested_def not found")
	}
	// nested def adds nesting but no cyclomatic complexity
	if fn.Cyclomatic != 1 {
		t.Errorf("use_nested_def cyclomatic = %d, want 1", fn.Cyclomatic)
	}
	if fn.MaxNesting != 1 {
		t.Errorf("use_nested_def maxNesting = %d, want 1", fn.MaxNesting)
	}
}

// TestComplexityReturnExpr verifies ReturnStmt with a non-nil result expression is walked.
func TestComplexityReturnExpr(t *testing.T) {
	funcMap := captureExpressions(t)

	fn, ok := funcMap["use_return_expr"]
	if !ok {
		t.Fatal("use_return_expr not found")
	}
	if fn.Cyclomatic != 1 {
		t.Errorf("use_return_expr cyclomatic = %d, want 1", fn.Cyclomatic)
	}
	if fn.Params != 2 {
		t.Errorf("use_return_expr params = %d, want 2", fn.Params)
	}
}

// TestComplexityAssignExpr verifies AssignStmt with expressions is walked.
func TestComplexityAssignExpr(t *testing.T) {
	funcMap := captureExpressions(t)

	fn, ok := funcMap["use_assign_expr"]
	if !ok {
		t.Fatal("use_assign_expr not found")
	}
	if fn.Cyclomatic != 1 {
		t.Errorf("use_assign_expr cyclomatic = %d, want 1", fn.Cyclomatic)
	}
}
