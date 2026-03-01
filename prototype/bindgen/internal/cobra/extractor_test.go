// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

//go:build ignore
// +build ignore

package cobra

import (
	"go/ast"
	"go/token"
	"testing"
)

func TestExtractCmdName(t *testing.T) {
	e := NewExtractor(false)

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"command with flag", "create --help", "create"},
		{"two-word use string", "container create", "container"},
		{"single word", "cmd", "cmd"},
		{"empty string", "", ""},
		{"multiple spaces", "  spaced  args ", "spaced"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := e.extractCmdName(tt.input)
			if result != tt.expected {
				t.Errorf("extractCmdName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestStringValue(t *testing.T) {
	e := NewExtractor(false)

	tests := []struct {
		name     string
		expr     ast.Expr
		expected string
	}{
		{
			"basic string literal",
			&ast.BasicLit{Kind: token.STRING, Value: `"hello"`},
			"hello",
		},
		{
			"integer literal returns empty",
			&ast.BasicLit{Kind: token.INT, Value: "42"},
			"",
		},
		{
			"binary add of two strings",
			&ast.BinaryExpr{
				X:  &ast.BasicLit{Kind: token.STRING, Value: `"hello"`},
				Op: token.ADD,
				Y:  &ast.BasicLit{Kind: token.STRING, Value: `"world"`},
			},
			"helloworld",
		},
		{
			"ident returns empty",
			&ast.Ident{Name: "someVar"},
			"",
		},
		{
			"binary subtract returns empty",
			&ast.BinaryExpr{
				X:  &ast.BasicLit{Kind: token.STRING, Value: `"hello"`},
				Op: token.SUB,
				Y:  &ast.BasicLit{Kind: token.STRING, Value: `"world"`},
			},
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := e.stringValue(tt.expr)
			if result != tt.expected {
				t.Errorf("stringValue() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestBoolValue(t *testing.T) {
	e := NewExtractor(false)

	tests := []struct {
		name     string
		expr     ast.Expr
		expected bool
	}{
		{"ident true", &ast.Ident{Name: "true"}, true},
		{"ident false", &ast.Ident{Name: "false"}, false},
		{"ident other", &ast.Ident{Name: "other"}, false},
		{"non-ident returns false", &ast.BasicLit{Kind: token.INT, Value: "1"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := e.boolValue(tt.expr)
			if result != tt.expected {
				t.Errorf("boolValue() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestParseFlagMethod(t *testing.T) {
	e := NewExtractor(false)

	tests := []struct {
		name         string
		method       string
		expectedType string
		expectedP    bool
	}{
		{"StringP", "StringP", "string", true},
		{"String", "String", "string", false},
		{"BoolP", "BoolP", "bool", true},
		{"IntSlice", "IntSlice", "int_list", false},
		{"StringToString", "StringToString", "string_map", false},
		{"DurationP", "DurationP", "duration", true},
		{"Float64", "Float64", "float", false},
		{"Float32", "Float32", "float", false},
		{"StringSliceP", "StringSliceP", "string_list", true},
		{"StringArray", "StringArray", "string_list", false},
		{"Int32", "Int32", "int", false},
		{"Int64", "Int64", "int", false},
		{"Unknown", "Unknown", "", false},
		{"StringVar", "StringVar", "string", false},
		{"BoolVarP", "BoolVarP", "bool", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flagType, hasShort := e.parseFlagMethod(tt.method)
			if flagType != tt.expectedType {
				t.Errorf("parseFlagMethod(%q) type = %q, want %q", tt.method, flagType, tt.expectedType)
			}
			if hasShort != tt.expectedP {
				t.Errorf("parseFlagMethod(%q) hasShort = %v, want %v", tt.method, hasShort, tt.expectedP)
			}
		})
	}
}

func TestIsCobraCommand(t *testing.T) {
	e := NewExtractor(false)

	tests := []struct {
		name     string
		comp     *ast.CompositeLit
		expected bool
	}{
		{
			"cobra.Command",
			&ast.CompositeLit{
				Type: &ast.SelectorExpr{
					X:   &ast.Ident{Name: "cobra"},
					Sel: &ast.Ident{Name: "Command"},
				},
			},
			true,
		},
		{
			"foo.Bar",
			&ast.CompositeLit{
				Type: &ast.SelectorExpr{
					X:   &ast.Ident{Name: "foo"},
					Sel: &ast.Ident{Name: "Bar"},
				},
			},
			false,
		},
		{
			"cobra.Bar",
			&ast.CompositeLit{
				Type: &ast.SelectorExpr{
					X:   &ast.Ident{Name: "cobra"},
					Sel: &ast.Ident{Name: "Bar"},
				},
			},
			false,
		},
		{
			"foo.Command",
			&ast.CompositeLit{
				Type: &ast.SelectorExpr{
					X:   &ast.Ident{Name: "foo"},
					Sel: &ast.Ident{Name: "Command"},
				},
			},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := e.isCobraCommand(tt.comp)
			if result != tt.expected {
				t.Errorf("isCobraCommand() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIsFlagReceiver(t *testing.T) {
	e := NewExtractor(false)

	tests := []struct {
		name     string
		expr     ast.Expr
		expected bool
	}{
		{
			"ident flags",
			&ast.Ident{Name: "flags"},
			true,
		},
		{
			"ident f",
			&ast.Ident{Name: "f"},
			true,
		},
		{
			"ident cmd",
			&ast.Ident{Name: "cmd"},
			false,
		},
		{
			"call Flags()",
			&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   &ast.Ident{Name: "cmd"},
					Sel: &ast.Ident{Name: "Flags"},
				},
			},
			true,
		},
		{
			"call PersistentFlags()",
			&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   &ast.Ident{Name: "cmd"},
					Sel: &ast.Ident{Name: "PersistentFlags"},
				},
			},
			true,
		},
		{
			"call LocalFlags()",
			&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   &ast.Ident{Name: "cmd"},
					Sel: &ast.Ident{Name: "LocalFlags"},
				},
			},
			true,
		},
		{
			"call Run()",
			&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   &ast.Ident{Name: "cmd"},
					Sel: &ast.Ident{Name: "Run"},
				},
			},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := e.isFlagReceiver(tt.expr)
			if result != tt.expected {
				t.Errorf("isFlagReceiver() = %v, want %v", result, tt.expected)
			}
		})
	}
}
