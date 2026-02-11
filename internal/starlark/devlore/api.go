// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

// Package devlore provides Starlark builtins specific to devlore-cli analysis.
// These functions parse devlore-cli's Go source to extract API information.
package devlore

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// PlanBinding represents a Starlark plan binding with its slots and output.
type PlanBinding struct {
	Name       string            // Full binding name (e.g., "plan.file.configure")
	Slots      []string          // Slot names (can be filled with promise or immediate)
	SlotDocs   map[string]string // Slot documentation (slot name -> description)
	Operations []string          // Graph operations (e.g., ["render", "copy"])
	Output     string            // Output type: "promise" or "none"
	Doc        string            // Description from doc comment
	Usage      string            // Usage example from doc comment
	Returns    string            // Returns description from doc comment
	File       string            // Source file
	Line       int               // Line number
}

// StringDictViolation represents a binding incorrectly registered via StringDict.
type StringDictViolation struct {
	Name string
	Line int
}

// GoParseDevloreAPI is the Starlark builtin for parsing devlore-cli's API.
// It parses Go source files in devlore-cli's starlark package
// and extracts the plan API: bindings, slots, and output types.
//
// It looks for:
//   - NewBuiltin("plan.xxx.yyy", ...) to find binding names
//   - FillSlot(node, graph, "slotName", ...) to find slot names
//   - NewOutput(...) returns to detect promise output
//
// Args:
//   - path: Path to the devlore-cli starlark directory (e.g., "internal/starlark")
//
// Returns:
//   - A struct with:
//   - bindings: List of {name, slots, output, file, line}
//   - namespaces: List of namespace strings
func GoParseDevloreAPI(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackArgs("go.parse_devlore_api", args, kwargs, "path", &path); err != nil {
		return nil, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	var files []string
	if info.IsDir() {
		// Collect all .go files in directory (including platform subdirs)
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
	namespaces := make(map[string]bool)

	for _, file := range files {
		bindings, err := parseDevloreAPIFile(file)
		if err != nil {
			continue // Skip files that fail to parse
		}

		for i := range bindings {
			b := &bindings[i]
			if !seenBindings[b.Name] {
				seenBindings[b.Name] = true
				allBindings = append(allBindings, *b)

				// Extract namespace from binding name
				if idx := strings.LastIndex(b.Name, "."); idx > 0 {
					ns := b.Name[:idx]
					namespaces[ns] = true
					// Also add parent namespaces
					for {
						if idx := strings.LastIndex(ns, "."); idx > 0 {
							ns = ns[:idx]
							namespaces[ns] = true
						} else {
							break
						}
					}
				}
			}
		}
	}

	// Sort bindings by name
	sortPlanBindings(allBindings)

	// Build hierarchical structure: context → namespace → methods
	// This is THE canonical representation of the Starlark API
	plan := make(map[string][]starlark.Value)   // plan.file → [...methods]
	system := make(map[string][]starlark.Value) // system.file → [...methods]
	var violations []starlark.Value

	for i := range allBindings {
		b := &allBindings[i]
		// Check for violations
		if strings.HasPrefix(b.Output, "VIOLATION:") {
			violations = append(violations, starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
				"name":  starlark.String(b.Name),
				"file":  starlark.String(b.File),
				"line":  starlark.MakeInt(b.Line),
				"error": starlark.String("uses StringDict instead of Attr receiver"),
			}))
			continue
		}

		// Parse binding name: context.namespace.method or context.method
		parts := strings.Split(b.Name, ".")
		if len(parts) < 2 {
			continue
		}

		context := parts[0] // "plan" or "system"
		var namespace, methodName string
		if len(parts) == 2 {
			namespace = "(root)"
			methodName = parts[1]
		} else {
			namespace = parts[1]
			methodName = parts[len(parts)-1]
		}

		// Build method entry
		method := bindingToHierarchicalStarlark(*b, methodName)

		// Add to appropriate context
		switch context {
		case "plan":
			plan[namespace] = append(plan[namespace], method)
		case "system":
			system[namespace] = append(system[namespace], method)
		}
	}

	// Convert maps to Starlark dicts
	planDict := starlark.StringDict{}
	for ns, methods := range plan {
		planDict[ns] = starlark.NewList(methods)
	}

	systemDict := starlark.StringDict{}
	for ns, methods := range system {
		systemDict[ns] = starlark.NewList(methods)
	}

	return starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
		"valid":      starlark.Bool(len(violations) == 0),
		"plan":       starlarkstruct.FromStringDict(starlarkstruct.Default, planDict),
		"system":     starlarkstruct.FromStringDict(starlarkstruct.Default, systemDict),
		"violations": starlark.NewList(violations),
	}), nil
}

// bindingToHierarchicalStarlark converts a PlanBinding to a Starlark struct for hierarchical output.
func bindingToHierarchicalStarlark(b PlanBinding, methodName string) starlark.Value {
	var slotsList []starlark.Value
	for _, s := range b.Slots {
		slotsList = append(slotsList, starlark.String(s))
	}

	var opsList []starlark.Value
	for _, op := range b.Operations {
		opsList = append(opsList, starlark.String(op))
	}

	slotDocsDict := starlark.StringDict{}
	for slotName, slotDoc := range b.SlotDocs {
		slotDocsDict[slotName] = starlark.String(slotDoc)
	}

	return starlarkstruct.FromStringDict(starlarkstruct.Default, starlark.StringDict{
		"name":       starlark.String(methodName),
		"full_name":  starlark.String(b.Name),
		"doc":        starlark.String(b.Doc),
		"usage":      starlark.String(b.Usage),
		"slots":      starlark.NewList(slotsList),
		"slot_docs":  starlarkstruct.FromStringDict(starlarkstruct.Default, slotDocsDict),
		"operations": starlark.NewList(opsList),
		"output":     starlark.String(b.Output),
		"returns":    starlark.String(b.Returns),
		"file":       starlark.String(b.File),
		"line":       starlark.MakeInt(b.Line),
	})
}

// parseDevloreAPIFile parses a single Go file and extracts plan bindings using AST.
// It also detects violations where bindings are registered via StringDict instead of Attr methods.
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
	attrBindings := findAttrBindings(node, fset)
	for bindingName, methodName := range attrBindings {
		if !isAPIBinding(bindingName) {
			continue
		}
		if seen[bindingName] {
			continue
		}

		binding := extractBindingFromMethod(node, fset, methodName, bindingName, filename)
		if binding != nil {
			seen[bindingName] = true
			bindings = append(bindings, *binding)
		}
	}

	// Detect StringDict violations (the WRONG pattern)
	violations := findStringDictViolations(node, fset, filename)
	for _, v := range violations {
		if !isAPIBinding(v.Name) {
			continue
		}
		if seen[v.Name] {
			continue
		}
		// Mark as violation - this binding uses the wrong pattern
		bindings = append(bindings, PlanBinding{
			Name:       v.Name,
			Slots:      nil,
			Operations: nil,
			Output:     "VIOLATION: uses StringDict instead of Attr receiver",
			File:       filename,
			Line:       v.Line,
		})
		seen[v.Name] = true
	}

	return bindings, nil
}

// findStringDictViolations finds plan.* bindings incorrectly registered via StringDict.
// These are CONTRACT VIOLATIONS - all plan bindings must use Attr receiver methods.
func findStringDictViolations(node *ast.File, fset *token.FileSet, _ string) []StringDictViolation {
	var violations []StringDictViolation

	ast.Inspect(node, func(n ast.Node) bool {
		// Look for composite literals (map/struct creation)
		comp, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}

		// Check if this is a starlark.StringDict
		sel, ok := comp.Type.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "StringDict" {
			return true
		}

		// Walk through the map entries
		for _, elt := range comp.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}

			// The value should be a NewBuiltin call
			call, ok := kv.Value.(*ast.CallExpr)
			if !ok {
				continue
			}

			// Check if this is a NewBuiltin call
			callSel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || callSel.Sel.Name != "NewBuiltin" {
				continue
			}

			// Extract binding name (first argument)
			if len(call.Args) < 2 {
				continue
			}

			bindingLit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || bindingLit.Kind != token.STRING {
				continue
			}
			bindingName := strings.Trim(bindingLit.Value, `"`)

			violations = append(violations, StringDictViolation{
				Name: bindingName,
				Line: fset.Position(call.Pos()).Line,
			})
		}

		return true
	})

	return violations
}

// findAttrBindings finds all NewBuiltin calls in Attr methods and returns a map
// of binding name -> handler method name.
func findAttrBindings(node *ast.File, _ *token.FileSet) map[string]string {
	bindings := make(map[string]string)

	ast.Inspect(node, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Body == nil || fn.Name == nil || fn.Name.Name != "Attr" {
			return true
		}

		// Walk the Attr method body looking for NewBuiltin calls
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			// Check if this is a NewBuiltin call
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "NewBuiltin" {
				return true
			}

			// Extract binding name (first argument)
			if len(call.Args) < 2 {
				return true
			}

			bindingLit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || bindingLit.Kind != token.STRING {
				return true
			}
			bindingName := strings.Trim(bindingLit.Value, `"`)

			// Extract handler method name (second argument)
			// Pattern: f.methodName or s.methodName
			handlerSel, ok := call.Args[1].(*ast.SelectorExpr)
			if !ok {
				return true
			}
			methodName := handlerSel.Sel.Name

			bindings[bindingName] = methodName
			return true
		})

		return true
	})

	return bindings
}

// extractBindingFromMethod extracts binding info from a handler method using AST.
func extractBindingFromMethod(node *ast.File, fset *token.FileSet, methodName, bindingName, filename string) *PlanBinding {
	var binding *PlanBinding

	ast.Inspect(node, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Body == nil || fn.Name == nil || fn.Name.Name != methodName {
			return true
		}

		// Found the method - extract info
		slots := extractSlotsFromAST(fn.Body)
		operations := extractOperationsFromAST(fn.Body)
		output := extractOutputFromAST(fn.Body)

		// Extract documentation from doc comment
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

		return false // Found it, stop searching
	})

	return binding
}

// parseDocComment parses a Go doc comment and extracts structured documentation.
// Expected format:
//
//	// description line(s)
//	// Usage: plan.namespace.method(args)
//	//
//	// Slots:
//	//   - slot_name: description
//	//
//	// Returns: description
func parseDocComment(doc *ast.CommentGroup) (description, usage string, slotDocs map[string]string, returns string) {
	slotDocs = make(map[string]string)

	if doc == nil {
		return
	}

	// Get the full doc text
	text := doc.Text()
	lines := strings.Split(text, "\n")

	var descLines []string
	inSlots := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Check for Usage:
		if strings.HasPrefix(line, "Usage:") {
			usage = strings.TrimSpace(strings.TrimPrefix(line, "Usage:"))
			inSlots = false
			continue
		}

		// Check for Slots:
		if strings.HasPrefix(line, "Slots:") {
			inSlots = true
			continue
		}

		// Check for Returns:
		if strings.HasPrefix(line, "Returns:") {
			returns = strings.TrimSpace(strings.TrimPrefix(line, "Returns:"))
			inSlots = false
			continue
		}

		// Parse slot documentation
		if inSlots && strings.HasPrefix(line, "- ") {
			// Format: "- slot_name: description"
			slotLine := strings.TrimPrefix(line, "- ")
			if colonIdx := strings.Index(slotLine, ":"); colonIdx > 0 {
				slotName := strings.TrimSpace(slotLine[:colonIdx])
				slotDesc := strings.TrimSpace(slotLine[colonIdx+1:])
				slotDocs[slotName] = slotDesc
			}
			continue
		}

		// Empty line ends slots section
		if inSlots && line == "" {
			inSlots = false
			continue
		}

		// Accumulate description lines (before any section)
		if usage == "" && !inSlots && returns == "" && line != "" {
			descLines = append(descLines, line)
		}
	}

	description = strings.Join(descLines, " ")
	return
}

// extractSlotsFromAST extracts slot names from FillSlot calls using AST.
func extractSlotsFromAST(body *ast.BlockStmt) []string {
	var slots []string
	seen := make(map[string]bool)

	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Check if this is a FillSlot call
		ident, ok := call.Fun.(*ast.Ident)
		if !ok || ident.Name != "FillSlot" {
			return true
		}

		// FillSlot(node, graph, "slotName", value) - third argument is slot name
		if len(call.Args) < 4 {
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

// extractOperationsFromAST extracts operations from execution.Node literals using AST.
func extractOperationsFromAST(body *ast.BlockStmt) []string {
	var operations []string

	ast.Inspect(body, func(n ast.Node) bool {
		// Look for composite literals (struct creation)
		comp, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}

		// Check if this is an execution.Node or &execution.Node
		isExecutionNode := false
		if t, ok := comp.Type.(*ast.SelectorExpr); ok {
			// execution.Node
			if ident, ok := t.X.(*ast.Ident); ok && ident.Name == "execution" && t.Sel.Name == "Node" {
				isExecutionNode = true
			}
		}

		if !isExecutionNode {
			return true
		}

		// Find the Operations field
		for _, elt := range comp.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}

			key, ok := kv.Key.(*ast.Ident)
			if !ok || key.Name != "Operations" {
				continue
			}

			// Extract string values from the slice literal
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

		// Check if this is a NewOutput call
		ident, ok := call.Fun.(*ast.Ident)
		if ok && ident.Name == "NewOutput" {
			output = "promise"
			return false // Found it
		}

		return true
	})

	return output
}

// isAPIBinding returns true if the binding name is part of the devlore API.
func isAPIBinding(name string) bool {
	return strings.HasPrefix(name, "plan.") || strings.HasPrefix(name, "system.")
}

// sortPlanBindings sorts bindings alphabetically by name.
func sortPlanBindings(bindings []PlanBinding) {
	for i := 0; i < len(bindings)-1; i++ {
		for j := i + 1; j < len(bindings); j++ {
			if bindings[i].Name > bindings[j].Name {
				bindings[i], bindings[j] = bindings[j], bindings[i]
			}
		}
	}
}