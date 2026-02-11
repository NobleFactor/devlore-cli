// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DevloreAPIResult is the JSON response for parse_devlore_api.
type DevloreAPIResult struct {
	Valid      bool                        `json:"valid"`
	Plan       map[string][]MethodEntry    `json:"plan"`
	System     map[string][]MethodEntry    `json:"system"`
	Violations []ViolationEntry            `json:"violations"`
}

// MethodEntry describes a single API method binding.
type MethodEntry struct {
	Name       string            `json:"name"`
	FullName   string            `json:"full_name"`
	Doc        string            `json:"doc"`
	Usage      string            `json:"usage"`
	Slots      []string          `json:"slots"`
	SlotDocs   map[string]string `json:"slot_docs"`
	Operations []string          `json:"operations"`
	Output     string            `json:"output"`
	Returns    string            `json:"returns"`
	File       string            `json:"file"`
	Line       int               `json:"line"`
}

// ViolationEntry describes a contract violation.
type ViolationEntry struct {
	Name  string `json:"name"`
	File  string `json:"file"`
	Line  int    `json:"line"`
	Error string `json:"error"`
}

// PlanBinding represents a discovered binding during AST parsing.
type PlanBinding struct {
	Name       string
	Slots      []string
	SlotDocs   map[string]string
	Operations []string
	Output     string
	Doc        string
	Usage      string
	Returns    string
	File       string
	Line       int
}

// parseDevloreAPI parses devlore-cli's Starlark API from Go source files.
// Port of GoParseDevloreAPI from internal/starlark/devlore/api.go.
func parseDevloreAPI(path string) (*DevloreAPIResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	var files []string
	if info.IsDir() {
		err = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && d.Name() == "testdata" {
				return filepath.SkipDir
			}
			if !d.IsDir() && strings.HasSuffix(d.Name(), ".go") && !strings.HasSuffix(d.Name(), "_test.go") {
				files = append(files, p)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		files = []string{path}
	}

	var allBindings []PlanBinding
	seenBindings := make(map[string]bool)

	for _, file := range files {
		bindings, err := parseDevloreAPIFile(file)
		if err != nil {
			continue
		}
		for i := range bindings {
			b := &bindings[i]
			if !seenBindings[b.Name] {
				seenBindings[b.Name] = true
				allBindings = append(allBindings, *b)
			}
		}
	}

	sort.Slice(allBindings, func(i, j int) bool {
		return allBindings[i].Name < allBindings[j].Name
	})

	// Build hierarchical structure: context → namespace → methods
	plan := make(map[string][]MethodEntry)
	system := make(map[string][]MethodEntry)
	var violations []ViolationEntry

	for i := range allBindings {
		b := &allBindings[i]

		if strings.HasPrefix(b.Output, "VIOLATION:") {
			violations = append(violations, ViolationEntry{
				Name:  b.Name,
				File:  b.File,
				Line:  b.Line,
				Error: "uses StringDict instead of Attr receiver",
			})
			continue
		}

		parts := strings.Split(b.Name, ".")
		if len(parts) < 2 {
			continue
		}

		context := parts[0]
		var namespace, methodName string
		if len(parts) == 2 {
			namespace = "(root)"
			methodName = parts[1]
		} else {
			namespace = parts[1]
			methodName = parts[len(parts)-1]
		}

		method := MethodEntry{
			Name:       methodName,
			FullName:   b.Name,
			Doc:        b.Doc,
			Usage:      b.Usage,
			Slots:      nonNilSlice(b.Slots),
			SlotDocs:   nonNilMap(b.SlotDocs),
			Operations: nonNilSlice(b.Operations),
			Output:     b.Output,
			Returns:    b.Returns,
			File:       b.File,
			Line:       b.Line,
		}

		switch context {
		case "plan":
			plan[namespace] = append(plan[namespace], method)
		case "system":
			system[namespace] = append(system[namespace], method)
		}
	}

	return &DevloreAPIResult{
		Valid:      len(violations) == 0,
		Plan:       plan,
		System:     system,
		Violations: nonNilViolations(violations),
	}, nil
}

// parseDevloreAPIFile parses a single Go file and extracts plan bindings.
func parseDevloreAPIFile(path string) ([]PlanBinding, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	filename := filepath.Base(path)
	var bindings []PlanBinding
	seen := make(map[string]bool)

	// Find bindings via Attr methods (the CORRECT pattern)
	attrBindings := findAttrBindings(node)
	for bindingName, methodName := range attrBindings {
		if !isAPIBinding(bindingName) || seen[bindingName] {
			continue
		}
		binding := extractBindingFromMethod(node, fset, methodName, bindingName, filename)
		if binding != nil {
			seen[bindingName] = true
			bindings = append(bindings, *binding)
		}
	}

	// Detect StringDict violations (the WRONG pattern)
	violations := findStringDictViolations(node, fset)
	for _, v := range violations {
		if !isAPIBinding(v.Name) || seen[v.Name] {
			continue
		}
		bindings = append(bindings, PlanBinding{
			Name:   v.Name,
			Output: "VIOLATION: uses StringDict instead of Attr receiver",
			File:   filename,
			Line:   v.Line,
		})
		seen[v.Name] = true
	}

	return bindings, nil
}

// StringDictViolation represents a binding incorrectly registered via StringDict.
type StringDictViolation struct {
	Name string
	Line int
}

// findStringDictViolations finds plan.* bindings incorrectly registered via StringDict.
func findStringDictViolations(node *ast.File, fset *token.FileSet) []StringDictViolation {
	var violations []StringDictViolation

	ast.Inspect(node, func(n ast.Node) bool {
		comp, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		sel, ok := comp.Type.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "StringDict" {
			return true
		}
		for _, elt := range comp.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			call, ok := kv.Value.(*ast.CallExpr)
			if !ok {
				continue
			}
			callSel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || callSel.Sel.Name != "NewBuiltin" {
				continue
			}
			if len(call.Args) < 2 {
				continue
			}
			bindingLit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || bindingLit.Kind != token.STRING {
				continue
			}
			violations = append(violations, StringDictViolation{
				Name: strings.Trim(bindingLit.Value, `"`),
				Line: fset.Position(call.Pos()).Line,
			})
		}
		return true
	})

	return violations
}

// findAttrBindings finds all NewBuiltin calls in Attr methods.
func findAttrBindings(node *ast.File) map[string]string {
	bindings := make(map[string]string)

	ast.Inspect(node, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Body == nil || fn.Name == nil || fn.Name.Name != "Attr" {
			return true
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "NewBuiltin" {
				return true
			}
			if len(call.Args) < 2 {
				return true
			}
			bindingLit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || bindingLit.Kind != token.STRING {
				return true
			}
			handlerSel, ok := call.Args[1].(*ast.SelectorExpr)
			if !ok {
				return true
			}
			bindings[strings.Trim(bindingLit.Value, `"`)] = handlerSel.Sel.Name
			return true
		})
		return true
	})

	return bindings
}

// extractBindingFromMethod extracts binding info from a handler method.
func extractBindingFromMethod(node *ast.File, fset *token.FileSet, methodName, bindingName, filename string) *PlanBinding {
	var binding *PlanBinding

	ast.Inspect(node, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Body == nil || fn.Name == nil || fn.Name.Name != methodName {
			return true
		}

		slots := extractSlotsFromAST(fn.Body)
		operations := extractOperationsFromAST(fn.Body)
		output := extractOutputFromAST(fn.Body)
		doc, usage, slotDocs, returns := parseDocComment(fn.Doc)

		binding = &PlanBinding{
			Name:       bindingName,
			Slots:      slots,
			SlotDocs:   slotDocs,
			Operations: operations,
			Output:     output,
			Doc:        doc,
			Usage:      usage,
			Returns:    returns,
			File:       filename,
			Line:       fset.Position(fn.Pos()).Line,
		}
		return false
	})

	return binding
}

// parseDocComment extracts structured documentation from a Go doc comment.
func parseDocComment(doc *ast.CommentGroup) (description, usage string, slotDocs map[string]string, returns string) {
	slotDocs = make(map[string]string)
	if doc == nil {
		return
	}

	lines := strings.Split(doc.Text(), "\n")
	var descLines []string
	inSlots := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "Usage:") {
			usage = strings.TrimSpace(strings.TrimPrefix(line, "Usage:"))
			inSlots = false
			continue
		}
		if strings.HasPrefix(line, "Slots:") {
			inSlots = true
			continue
		}
		if strings.HasPrefix(line, "Returns:") {
			returns = strings.TrimSpace(strings.TrimPrefix(line, "Returns:"))
			inSlots = false
			continue
		}
		if inSlots && strings.HasPrefix(line, "- ") {
			slotLine := strings.TrimPrefix(line, "- ")
			if colonIdx := strings.Index(slotLine, ":"); colonIdx > 0 {
				slotDocs[strings.TrimSpace(slotLine[:colonIdx])] = strings.TrimSpace(slotLine[colonIdx+1:])
			}
			continue
		}
		if inSlots && line == "" {
			inSlots = false
			continue
		}
		if usage == "" && !inSlots && returns == "" && line != "" {
			descLines = append(descLines, line)
		}
	}

	description = strings.Join(descLines, " ")
	return
}

// extractSlotsFromAST extracts slot names from FillSlot calls.
func extractSlotsFromAST(body *ast.BlockStmt) []string {
	var slots []string
	seen := make(map[string]bool)

	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		ident, ok := call.Fun.(*ast.Ident)
		if !ok || ident.Name != "FillSlot" || len(call.Args) < 4 {
			return true
		}
		slotLit, ok := call.Args[2].(*ast.BasicLit)
		if !ok || slotLit.Kind != token.STRING {
			return true
		}
		slotName := strings.Trim(slotLit.Value, `"`)
		if !seen[slotName] {
			seen[slotName] = true
			slots = append(slots, slotName)
		}
		return true
	})

	return slots
}

// extractOperationsFromAST extracts operations from execution.Node literals.
func extractOperationsFromAST(body *ast.BlockStmt) []string {
	var operations []string

	ast.Inspect(body, func(n ast.Node) bool {
		comp, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		t, ok := comp.Type.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := t.X.(*ast.Ident)
		if !ok || ident.Name != "execution" || t.Sel.Name != "Node" {
			return true
		}
		for _, elt := range comp.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok || key.Name != "Operations" {
				continue
			}
			compLit, ok := kv.Value.(*ast.CompositeLit)
			if !ok {
				continue
			}
			for _, elem := range compLit.Elts {
				lit, ok := elem.(*ast.BasicLit)
				if ok && lit.Kind == token.STRING {
					operations = append(operations, strings.Trim(lit.Value, `"`))
				}
			}
		}
		return true
	})

	return operations
}

// extractOutputFromAST detects if the method returns a promise (NewOutput).
func extractOutputFromAST(body *ast.BlockStmt) string {
	output := "none"
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		ident, ok := call.Fun.(*ast.Ident)
		if ok && ident.Name == "NewOutput" {
			output = "promise"
			return false
		}
		return true
	})
	return output
}

func isAPIBinding(name string) bool {
	return strings.HasPrefix(name, "plan.") || strings.HasPrefix(name, "system.")
}

func nonNilSlice(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func nonNilMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func nonNilViolations(v []ViolationEntry) []ViolationEntry {
	if v == nil {
		return []ViolationEntry{}
	}
	return v
}
