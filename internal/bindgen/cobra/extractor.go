// SPDX-License-Identifier: SSPL-1.0
// Copyright (c) 2025-2026 Noble Factor. All rights reserved.

// Package cobra extracts command metadata from Go source files using Cobra.
// Uses golang.org/x/tools for proper package loading and efficient AST traversal.
package cobra

import (
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ast/inspector"
	"golang.org/x/tools/go/packages"

	"github.com/NobleFactor/devlore-cli/internal/bindgen"
)

// Extractor parses Go source files to find Cobra command definitions.
type Extractor struct {
	commands map[string]*bindgen.Command
	verbose  bool
	baseDir  string // Root directory for relative path calculation
}

// NewExtractor creates a new Cobra extractor.
func NewExtractor(verbose bool) *Extractor {
	return &Extractor{
		commands: make(map[string]*bindgen.Command),
		verbose:  verbose,
	}
}

// ExtractDir extracts commands from all Go packages in a directory.
func (e *Extractor) ExtractDir(dir string) (*bindgen.BindingDef, error) {
	// Store base directory for qualified name generation
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolving path: %w", err)
	}
	e.baseDir = absDir

	// Find all Go packages in the directory tree
	patterns, err := e.findPackages(dir)
	if err != nil {
		return nil, fmt.Errorf("finding packages: %w", err)
	}

	if len(patterns) == 0 {
		return nil, fmt.Errorf("no Go packages found in %s", dir)
	}

	if e.verbose {
		fmt.Fprintf(os.Stderr, "Found %d packages to analyze\n", len(patterns))
	}

	// Load packages - only need syntax, not full type checking
	// This avoids dependency resolution issues with external repos
	cfg := &packages.Config{
		Mode:  packages.NeedName | packages.NeedFiles | packages.NeedSyntax,
		Dir:   dir,
		Tests: false,
	}

	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("loading packages: %w", err)
	}

	// Process each package
	for _, pkg := range pkgs {
		if e.verbose {
			fmt.Fprintf(os.Stderr, "Package %s: %d files, %d syntax trees\n",
				pkg.PkgPath, len(pkg.GoFiles), len(pkg.Syntax))
		}

		if len(pkg.Errors) > 0 {
			if e.verbose {
				for _, err := range pkg.Errors {
					fmt.Fprintf(os.Stderr, "  Warning: %v\n", err)
				}
			}
		}

		e.extractFromPackage(pkg)
	}

	return &bindgen.BindingDef{
		Name:        "extracted",
		Description: fmt.Sprintf("Extracted from %s", dir),
		Commands:    e.commands,
	}, nil
}

// findPackages finds all Go package patterns in a directory tree.
func (e *Extractor) findPackages(dir string) ([]string, error) { //nolint:gocognit
	seen := make(map[string]bool)
	var patterns []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // intentional: skip unreadable entries during walk
		}
		if info.IsDir() {
			// Skip vendor, testdata, hidden dirs
			name := info.Name()
			if name == "vendor" || name == "testdata" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			pkgDir := filepath.Dir(path)
			if !seen[pkgDir] {
				seen[pkgDir] = true
				// Use relative pattern
				rel, err := filepath.Rel(dir, pkgDir)
				if err != nil {
					return nil //nolint:nilerr // intentional: skip packages with non-relative paths
				}
				if rel == "." {
					patterns = append(patterns, ".")
				} else {
					patterns = append(patterns, "./"+rel)
				}
			}
		}
		return nil
	})

	return patterns, err
}

// extractFromPackage extracts commands from a loaded package.
func (e *Extractor) extractFromPackage(pkg *packages.Package) {
	if len(pkg.Syntax) == 0 {
		return
	}

	// Check if package imports cobra
	importsCobra := false
	for _, file := range pkg.Syntax {
		for _, imp := range file.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				continue
			}
			if strings.Contains(path, "spf13/cobra") {
				importsCobra = true
				break
			}
		}
		if importsCobra {
			break
		}
	}

	if !importsCobra {
		return
	}

	if e.verbose {
		fmt.Fprintf(os.Stderr, "Scanning package %s\n", pkg.PkgPath)
	}

	// Calculate prefix from package directory relative to base
	prefix := e.calculatePrefix(pkg)

	// Create inspector for efficient traversal
	inspect := inspector.New(pkg.Syntax)

	// Process each function declaration
	nodeFilter := []ast.Node{(*ast.FuncDecl)(nil)}
	inspect.Preorder(nodeFilter, func(n ast.Node) {
		fn := n.(*ast.FuncDecl)
		e.extractFromFunction(fn, prefix)
	})
}

// calculatePrefix determines the command prefix from the package directory.
// For example, "cli/command/container" relative to "cli/command" yields "container".
func (e *Extractor) calculatePrefix(pkg *packages.Package) string {
	if len(pkg.GoFiles) == 0 {
		return ""
	}

	// Get the directory containing the Go files
	pkgDir := filepath.Dir(pkg.GoFiles[0])

	// Calculate relative path from base directory
	relPath, err := filepath.Rel(e.baseDir, pkgDir)
	if err != nil || relPath == "." {
		return ""
	}

	// Use the first path component as the prefix
	// e.g., "container/internal" -> "container"
	parts := strings.Split(relPath, string(filepath.Separator))
	if len(parts) > 0 && parts[0] != ".." {
		return parts[0]
	}
	return ""
}

// extractFromFunction extracts commands and their flags from a single function.
// prefix is the parent command name derived from the package directory (e.g., "container").
func (e *Extractor) extractFromFunction(fn *ast.FuncDecl, prefix string) { //nolint:gocognit,gocyclo
	var currentCmd *bindgen.Command

	ast.Inspect(fn, func(n ast.Node) bool {
		// Look for cmd := &cobra.Command{...}
		if assign, ok := n.(*ast.AssignStmt); ok {
			for _, rhs := range assign.Rhs {
				if unary, ok := rhs.(*ast.UnaryExpr); ok && unary.Op == token.AND {
					if comp, ok := unary.X.(*ast.CompositeLit); ok {
						if e.isCobraCommand(comp) {
							cmd := e.extractCommand(comp)
							if cmd != nil && cmd.Name != "" {
								currentCmd = cmd
							}
						}
					}
				}
			}
		}

		// Look for flag definitions
		if call, ok := n.(*ast.CallExpr); ok {
			if flag := e.extractFlagFromCall(call); flag != nil {
				if currentCmd != nil {
					currentCmd.Flags = append(currentCmd.Flags, flag)
				}
			}
		}

		return true
	})

	// Register the command with qualified name
	if currentCmd != nil && currentCmd.Name != "" {
		// Generate qualified key to avoid collisions
		// e.g., "container" + "create" -> "container_create"
		key := currentCmd.Name
		if prefix != "" && prefix != currentCmd.Name {
			key = prefix + "_" + currentCmd.Name
		}

		if existing, ok := e.commands[key]; ok {
			// Still prefer command with more flags if names collide
			if len(currentCmd.Flags) > len(existing.Flags) {
				e.commands[key] = currentCmd
			}
		} else {
			e.commands[key] = currentCmd
		}

		if e.verbose {
			fmt.Fprintf(os.Stderr, "  Found command: %s (key=%s, %d flags)\n",
				currentCmd.Name, key, len(currentCmd.Flags))
		}
	}
}

// isCobraCommand checks if a composite literal is a cobra.Command.
func (e *Extractor) isCobraCommand(comp *ast.CompositeLit) bool {
	if sel, ok := comp.Type.(*ast.SelectorExpr); ok {
		if ident, ok := sel.X.(*ast.Ident); ok {
			return ident.Name == "cobra" && sel.Sel.Name == "Command"
		}
	}
	return false
}

// extractCommand extracts command metadata from a cobra.Command composite literal.
func (e *Extractor) extractCommand(comp *ast.CompositeLit) *bindgen.Command {
	cmd := &bindgen.Command{
		Flags: []*bindgen.Flag{},
		Args:  []*bindgen.Arg{},
		Returns: &bindgen.Return{
			Type:   "result",
			Fields: []string{"ok", "stdout", "stderr", "code"},
		},
	}

	for _, elt := range comp.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}

		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}

		switch key.Name {
		case "Use":
			use := e.stringValue(kv.Value)
			cmd.Name = e.extractCmdName(use)
		case "Short":
			cmd.Description = e.stringValue(kv.Value)
		case "Long":
			if cmd.Description == "" {
				cmd.Description = e.stringValue(kv.Value)
			}
		case "Deprecated":
			dep := e.stringValue(kv.Value)
			if dep != "" {
				cmd.Description = "[DEPRECATED: " + dep + "] " + cmd.Description
			}
		case "Hidden":
			if e.boolValue(kv.Value) {
				return nil
			}
		}
	}

	return cmd
}

// extractCmdName extracts the command name from a Use string.
func (e *Extractor) extractCmdName(use string) string {
	parts := strings.Fields(use)
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// stringValue extracts a string value from an AST expression.
func (e *Extractor) stringValue(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.BasicLit:
		if v.Kind == token.STRING {
			s, err := strconv.Unquote(v.Value)
			if err != nil {
				return ""
			}
			return s
		}
	case *ast.BinaryExpr:
		if v.Op == token.ADD {
			return e.stringValue(v.X) + e.stringValue(v.Y)
		}
	}
	return ""
}

// boolValue extracts a bool value from an AST expression.
func (e *Extractor) boolValue(expr ast.Expr) bool {
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name == "true"
	}
	return false
}

// extractFlagFromCall extracts flag metadata from method calls.
func (e *Extractor) extractFlagFromCall(call *ast.CallExpr) *bindgen.Flag {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}

	methodName := sel.Sel.Name
	flagType, hasShort := e.parseFlagMethod(methodName)
	if flagType == "" {
		return nil
	}

	if !e.isFlagReceiver(sel.X) {
		return nil
	}

	return e.extractFlagArgs(call.Args, flagType, hasShort)
}

// isFlagReceiver checks if the expression is a flag-related receiver.
func (e *Extractor) isFlagReceiver(expr ast.Expr) bool {
	switch v := expr.(type) {
	case *ast.Ident:
		return v.Name == "flags" || v.Name == "f"
	case *ast.CallExpr:
		if sel, ok := v.Fun.(*ast.SelectorExpr); ok {
			name := sel.Sel.Name
			return name == "Flags" || name == "PersistentFlags" || name == "LocalFlags"
		}
	}
	return false
}

// parseFlagMethod determines the flag type and whether it has a short form.
func (e *Extractor) parseFlagMethod(method string) (string, bool) {
	hasShort := strings.HasSuffix(method, "P")
	base := strings.TrimSuffix(strings.TrimSuffix(method, "P"), "Var")

	switch base {
	case "Bool":
		return "bool", hasShort
	case "String":
		return "string", hasShort
	case "Int", "Int32", "Int64":
		return "int", hasShort
	case "Float32", "Float64":
		return "float", hasShort
	case "Duration":
		return "duration", hasShort
	case "StringSlice", "StringArray":
		return "string_list", hasShort
	case "IntSlice":
		return "int_list", hasShort
	case "StringToString":
		return "string_map", hasShort
	default:
		return "", false
	}
}

// extractFlagArgs extracts flag details from call arguments.
func (e *Extractor) extractFlagArgs(args []ast.Expr, flagType string, hasShort bool) *bindgen.Flag {
	if len(args) < 3 {
		return nil
	}

	flag := &bindgen.Flag{
		Type: flagType,
	}

	idx := 1 // Skip pointer arg

	if idx < len(args) {
		flag.Name = e.stringValue(args[idx])
		idx++
	}

	if hasShort && idx < len(args) {
		flag.Short = e.stringValue(args[idx])
		idx++
	}

	if idx < len(args) {
		flag.Default = e.defaultValue(args[idx])
		idx++
	}

	if idx < len(args) {
		flag.Description = e.stringValue(args[idx])
	}

	if flag.Name == "" {
		return nil
	}

	return flag
}

// defaultValue extracts a default value as string.
func (e *Extractor) defaultValue(expr ast.Expr) string {
	switch v := expr.(type) {
	case *ast.BasicLit:
		s, err := strconv.Unquote(v.Value)
		if err != nil {
			return v.Value
		}
		if s != "" {
			return s
		}
		return v.Value
	case *ast.Ident:
		if v.Name == "true" || v.Name == "false" {
			return v.Name
		}
	}
	return ""
}

// Stats returns extraction statistics.
func (e *Extractor) Stats() (commands, flags int) {
	for _, cmd := range e.commands {
		commands++
		flags += len(cmd.Flags)
	}
	return
}
