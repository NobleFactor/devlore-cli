// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

package lore

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoDictBasedReceivers enforces the architectural ban on using
// starlarkstruct.FromStringDict to create namespace receivers. Dict-based
// receivers hide the backing implementation and make code generation impossible.
//
// ALLOWED: FromStringDict for data structs (platform info, result structs)
// BANNED:  FromStringDict containing NewBuiltin values (namespace receivers)
//
// Every Starlark namespace must use the receiver base type with Attr/AttrNames.
func TestNoDictBasedReceivers(t *testing.T) {
	pkgDir := "."

	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}

	type goFile struct {
		name string
		path string
	}
	var files []goFile
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".go") && !strings.HasSuffix(e.Name(), "_test.go") {
			files = append(files, goFile{name: e.Name(), path: filepath.Join(pkgDir, e.Name())})
		}
	}

	for _, f := range files {
		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, f.path, nil, parser.ParseComments)
		if err != nil {
			t.Errorf("parse %s: %v", f.name, err)
			continue
		}

		ast.Inspect(node, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			// Match starlarkstruct.FromStringDict or just FromStringDict
			if !isFromStringDictCall(call) {
				return true
			}

			// Check if any value in the dict literal is a NewBuiltin call
			if containsNewBuiltin(call) {
				pos := fset.Position(call.Pos())
				t.Errorf(
					"%s:%d: FromStringDict contains NewBuiltin — "+
						"dict-based receivers are banned. "+
						"Use the receiver base type with Attr/AttrNames instead.",
					f.name, pos.Line,
				)
			}
			return true
		})
	}
}

// isFromStringDictCall checks if a CallExpr is a call to FromStringDict.
func isFromStringDictCall(call *ast.CallExpr) bool {
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		return fn.Sel.Name == "FromStringDict"
	case *ast.Ident:
		return fn.Name == "FromStringDict"
	}
	return false
}

// containsNewBuiltin checks if a FromStringDict call's dict argument
// contains any NewBuiltin calls (indicating a namespace receiver).
func containsNewBuiltin(call *ast.CallExpr) bool {
	found := false
	// Walk all arguments looking for NewBuiltin calls anywhere in the tree
	for _, arg := range call.Args {
		ast.Inspect(arg, func(n ast.Node) bool {
			if found {
				return false
			}
			innerCall, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fn := innerCall.Fun.(type) {
			case *ast.SelectorExpr:
				if fn.Sel.Name == "NewBuiltin" {
					found = true
					return false
				}
			case *ast.Ident:
				if fn.Name == "NewBuiltin" {
					found = true
					return false
				}
			}
			return true
		})
	}
	return found
}
