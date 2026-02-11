// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// ExecutionOpsResult is the JSON response for parse_execution_ops.
type ExecutionOpsResult struct {
	Operations []ExecutionOpEntry `json:"operations"`
	Count      int                `json:"count"`
}

// ExecutionOpEntry represents an execution operation.
type ExecutionOpEntry struct {
	Name     string `json:"name"`
	TypeName string `json:"type_name"`
	File     string `json:"file"`
	Line     int    `json:"line"`
}

// ExecutionSchemaResult is the JSON response for parse_execution_schema.
// Fields are dynamic — struct definitions and const groups become top-level keys.
type ExecutionSchemaResult map[string]any

// StructDefEntry represents a Go struct definition.
type StructDefEntry struct {
	Name   string            `json:"name"`
	Fields []StructFieldEntry `json:"fields"`
}

// StructFieldEntry represents a field in a Go struct.
type StructFieldEntry struct {
	Name        string `json:"name"`
	JSONName    string `json:"json_name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

// parseExecutionOps parses execution operations from Go source.
// Port of GoReceiver.parseExecutionOps from noblefactor-ops.
func parseExecutionOps(path string) (*ExecutionOpsResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("parse_execution_ops: %w", err)
	}

	var files []string
	if info.IsDir() {
		opsPath := filepath.Join(path, "ops.go")
		if _, err := os.Stat(opsPath); err == nil {
			files = append(files, opsPath)
		}
		entries, err := os.ReadDir(path)
		if err != nil {
			return nil, fmt.Errorf("parse_execution_ops: reading dir: %w", err)
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") &&
				!strings.HasSuffix(entry.Name(), "_test.go") &&
				entry.Name() != "ops.go" {
				files = append(files, filepath.Join(path, entry.Name()))
			}
		}
	} else {
		files = []string{path}
	}

	var ops []ExecutionOpEntry
	seen := make(map[string]bool)

	for _, file := range files {
		fileOps, err := parseOpsFile(file)
		if err != nil {
			continue
		}
		for _, op := range fileOps {
			if !seen[op.Name] {
				seen[op.Name] = true
				ops = append(ops, op)
			}
		}
	}

	sort.Slice(ops, func(i, j int) bool {
		return ops[i].Name < ops[j].Name
	})

	return &ExecutionOpsResult{
		Operations: ops,
		Count:      len(ops),
	}, nil
}

func parseOpsFile(path string) ([]ExecutionOpEntry, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	filename := filepath.Base(path)
	var ops []ExecutionOpEntry

	ast.Inspect(node, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
			return true
		}
		if fn.Name == nil || fn.Name.Name != "Name" {
			return true
		}

		recvType := ""
		switch t := fn.Recv.List[0].Type.(type) {
		case *ast.StarExpr:
			if ident, ok := t.X.(*ast.Ident); ok {
				recvType = ident.Name
			}
		case *ast.Ident:
			recvType = t.Name
		}

		if recvType == "" || !strings.HasSuffix(recvType, "Op") {
			return true
		}
		if fn.Type.Results == nil || len(fn.Type.Results.List) != 1 {
			return true
		}
		if ident, ok := fn.Type.Results.List[0].Type.(*ast.Ident); !ok || ident.Name != "string" {
			return true
		}

		opName := extractReturnString(fn.Body)
		if opName == "" {
			return true
		}

		ops = append(ops, ExecutionOpEntry{
			Name:     opName,
			TypeName: recvType,
			Line:     fset.Position(fn.Pos()).Line,
			File:     filename,
		})
		return true
	})

	return ops, nil
}

func extractReturnString(body *ast.BlockStmt) string {
	if body == nil || len(body.List) == 0 {
		return ""
	}
	for _, stmt := range body.List {
		ret, ok := stmt.(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			continue
		}
		lit, ok := ret.Results[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			continue
		}
		return strings.Trim(lit.Value, `"`)
	}
	return ""
}

// parseExecutionSchema parses execution graph types from Go source.
// Port of GoReceiver.parseExecutionSchema from noblefactor-ops.
func parseExecutionSchema(path string) (ExecutionSchemaResult, error) {
	graphPath := filepath.Join(path, "graph.go")
	structs, consts, err := parseStructDefs(graphPath)
	if err != nil {
		return nil, fmt.Errorf("parse_execution_schema: parsing graph.go: %w", err)
	}

	var ops []ExecutionOpEntry
	seen := make(map[string]bool)

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("parse_execution_schema: reading dir: %w", err)
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "ops") && strings.HasSuffix(entry.Name(), ".go") &&
			!strings.HasSuffix(entry.Name(), "_test.go") {
			fileOps, err := parseOpsFile(filepath.Join(path, entry.Name()))
			if err != nil {
				continue
			}
			for _, op := range fileOps {
				if !seen[op.Name] {
					seen[op.Name] = true
					ops = append(ops, op)
				}
			}
		}
	}

	sort.Slice(ops, func(i, j int) bool {
		return ops[i].Name < ops[j].Name
	})

	result := make(ExecutionSchemaResult)

	for name, def := range structs {
		result[toSnakeCase(name)] = def
	}

	for typeName, values := range consts {
		enumList := make([]string, len(values))
		for i, v := range values {
			enumList[i] = v.Value
		}
		result[toSnakeCase(typeName)+"s"] = enumList
	}

	opNames := make([]string, len(ops))
	for i, op := range ops {
		opNames[i] = op.Name
	}
	result["operations"] = opNames

	return result, nil
}

// ConstValue represents a const value extracted from Go source.
type ConstValue struct {
	Name  string
	Value string
}

func parseStructDefs(path string) (map[string]StructDefEntry, map[string][]ConstValue, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, nil, err
	}

	structs := make(map[string]StructDefEntry)
	consts := make(map[string][]ConstValue)
	var currentConstType string

	ast.Inspect(node, func(n ast.Node) bool {
		x, ok := n.(*ast.GenDecl)
		if !ok {
			return true
		}

		if x.Tok == token.TYPE {
			for _, spec := range x.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				structType, ok := typeSpec.Type.(*ast.StructType)
				if !ok {
					continue
				}

				def := StructDefEntry{Name: typeSpec.Name.Name}
				for _, field := range structType.Fields.List {
					if len(field.Names) == 0 {
						continue
					}
					sf := StructFieldEntry{
						Name: field.Names[0].Name,
						Type: typeToString(field.Type),
					}
					if field.Tag != nil {
						tag := strings.Trim(field.Tag.Value, "`")
						sf.JSONName, sf.Required = parseJSONTag(tag)
					}
					if field.Comment != nil {
						sf.Description = strings.TrimSpace(field.Comment.Text())
					} else if field.Doc != nil {
						sf.Description = strings.TrimSpace(field.Doc.Text())
					}
					if sf.JSONName == "-" {
						continue
					}
					def.Fields = append(def.Fields, sf)
				}
				structs[def.Name] = def
			}
		} else if x.Tok == token.CONST {
			for _, spec := range x.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				if valueSpec.Type != nil {
					if ident, ok := valueSpec.Type.(*ast.Ident); ok {
						currentConstType = ident.Name
					}
				}
				for i, name := range valueSpec.Names {
					if currentConstType == "" {
						continue
					}
					var value string
					if i < len(valueSpec.Values) {
						if lit, ok := valueSpec.Values[i].(*ast.BasicLit); ok {
							value = strings.Trim(lit.Value, `"`)
						}
					}
					if value != "" {
						consts[currentConstType] = append(consts[currentConstType], ConstValue{
							Name:  name.Name,
							Value: value,
						})
					}
				}
			}
		}
		return true
	})

	return structs, consts, nil
}

func typeToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return typeToString(t.X) + "." + t.Sel.Name
	case *ast.StarExpr:
		return "*" + typeToString(t.X)
	case *ast.ArrayType:
		return "[]" + typeToString(t.Elt)
	case *ast.MapType:
		return "map[" + typeToString(t.Key) + "]" + typeToString(t.Value)
	default:
		return "unknown"
	}
}

var jsonTagRe = regexp.MustCompile(`json:"([^"]*)"`)

func parseJSONTag(tag string) (name string, required bool) {
	match := jsonTagRe.FindStringSubmatch(tag)
	if match == nil {
		return "", false
	}
	parts := strings.Split(match[1], ",")
	name = parts[0]
	required = true
	for _, part := range parts[1:] {
		if part == "omitempty" {
			required = false
		}
	}
	return name, required
}

func toSnakeCase(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteRune('_')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}
