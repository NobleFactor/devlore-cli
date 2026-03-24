// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

package goast

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"unicode"
)

// =============================================================================
// PARSED FILE CACHE
// =============================================================================

// parsedFile holds a cached parsed Go file.
type parsedFile struct {
	fset *token.FileSet
	node *ast.File
}

// parseFile parses a Go file with caching.
func (p *Provider) parseFile(path string) (*token.FileSet, *ast.File, error) {
	if cached, ok := p.fileCache.Load(path); ok {
		pf := cached.(*parsedFile)
		return pf.fset, pf.node, nil
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, nil, err
	}

	p.fileCache.Store(path, &parsedFile{fset: fset, node: node})

	return fset, node, nil
}

// =============================================================================
// SCOPE ENCODING
// =============================================================================

// encodeScope creates an opaque scope string from a file path and function name.
func encodeScope(filePath, name string) string {
	return filePath + "::" + name
}

// decodeScope splits a scope string into file path and function name.
func decodeScope(scope string) (string, string, error) {
	parts := strings.SplitN(scope, "::", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid scope: %s", scope)
	}

	return parts[0], parts[1], nil
}

// findScopeBody resolves a scope string to the function/method body AST node.
func (p *Provider) findScopeBody(scope string) (*token.FileSet, *ast.BlockStmt, error) {
	filePath, name, err := decodeScope(scope)
	if err != nil {
		return nil, nil, err
	}

	fset, node, err := p.parseFile(filePath)
	if err != nil {
		return nil, nil, err
	}

	parts := strings.SplitN(name, ".", 2)
	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil {
			continue
		}

		if len(parts) == 2 {
			if fn.Name.Name != parts[1] || fn.Recv == nil || len(fn.Recv.List) == 0 {
				continue
			}

			recvType := receiverTypeName(fn.Recv.List[0].Type)
			if strings.TrimPrefix(recvType, "*") == strings.TrimPrefix(parts[0], "*") {
				return fset, fn.Body, nil
			}
		} else {
			if fn.Name.Name == name && fn.Recv == nil {
				return fset, fn.Body, nil
			}
		}
	}

	return nil, nil, fmt.Errorf("scope not found: %s", scope)
}

// =============================================================================
// FILE COLLECTION
// =============================================================================

// collectGoFiles returns Go source files for a path.
//
// If path is a file, returns it directly. If a directory, walks it recursively, skipping vendor, testdata,.git,
// and _test.go files.
func collectGoFiles(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		return []string{path}, nil
	}

	var files []string
	err = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() && (d.Name() == "vendor" || d.Name() == "testdata" || d.Name() == ".git") {
			return filepath.SkipDir
		}

		if !d.IsDir() && strings.HasSuffix(d.Name(), ".go") && !strings.HasSuffix(d.Name(), "_test.go") {
			files = append(files, p)
		}

		return nil
	})

	return files, err
}

// =============================================================================
// MODULE DETECTION
// =============================================================================

// detectModulePath finds the Go module path by walking up from startPath looking for go.mod.
func detectModulePath(startPath string) string {
	dir := startPath

	info, err := os.Stat(dir)
	if err == nil && !info.IsDir() {
		dir = filepath.Dir(dir)
	}

	for {
		modPath := filepath.Join(dir, "go.mod")
		if content, err := os.ReadFile(modPath); err == nil {
			lines := strings.Split(string(content), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "module ") {
					return strings.TrimSpace(strings.TrimPrefix(line, "module "))
				}
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}

		dir = parent
	}

	return ""
}

// isStdlib returns true if the import path belongs to the Go standard library.
func isStdlib(importPath string) bool {
	if !strings.Contains(importPath, ".") {
		return true
	}

	if strings.HasPrefix(importPath, "golang.org/x/") {
		return false
	}

	return false
}

// =============================================================================
// AST TYPE HELPERS
// =============================================================================

// typeToString formats any Go AST type expression as a string.
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
	case *ast.Ellipsis:
		return "..." + typeToString(t.Elt)
	case *ast.InterfaceType:
		if t.Methods == nil || len(t.Methods.List) == 0 {
			return "any"
		}
		return "interface{...}"
	case *ast.FuncType:
		var params []string
		if t.Params != nil {
			for _, p := range t.Params.List {
				ts := typeToString(p.Type)
				n := len(p.Names)
				if n == 0 {
					n = 1
				}
				for range n {
					params = append(params, ts)
				}
			}
		}

		ret := returnTypeString(t.Results)
		if ret == "" {
			return "func(" + strings.Join(params, ", ") + ")"
		}

		return "func(" + strings.Join(params, ", ") + ") " + ret
	case *ast.ChanType:
		switch t.Dir {
		case ast.SEND:
			return "chan<- " + typeToString(t.Value)
		case ast.RECV:
			return "<-chan " + typeToString(t.Value)
		default:
			return "chan " + typeToString(t.Value)
		}
	case *ast.IndexExpr:
		return typeToString(t.X) + "[" + typeToString(t.Index) + "]"
	case *ast.IndexListExpr:
		var indices []string
		for _, idx := range t.Indices {
			indices = append(indices, typeToString(idx))
		}
		return typeToString(t.X) + "[" + strings.Join(indices, ", ") + "]"
	default:
		return "unknown"
	}
}

// receiverTypeName extracts the type name from a receiver expression.
func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return "*" + ident.Name
		}
	case *ast.Ident:
		return t.Name
	}

	return ""
}

// returnTypeString formats a function's return type list as a string.
func returnTypeString(results *ast.FieldList) string {
	if results == nil || len(results.List) == 0 {
		return ""
	}

	if len(results.List) == 1 {
		return typeToString(results.List[0].Type)
	}

	var types []string
	for _, r := range results.List {
		types = append(types, typeToString(r.Type))
	}

	return "(" + strings.Join(types, ", ") + ")"
}

// =============================================================================
// COMMENT HELPERS
// =============================================================================

// commentGroupRaw returns the full text of a comment group, preserving directive lines (//tool:directive) that
// ast.CommentGroup.Text() strips since Go 1.21.
func commentGroupRaw(cg *ast.CommentGroup) string {
	if cg == nil {
		return ""
	}

	var lines []string
	for _, c := range cg.List {
		text := c.Text
		if strings.HasPrefix(text, "//") {
			text = strings.TrimPrefix(text, "//")
			// Strip at most one leading space (standard Go comment style).
			if len(text) > 0 && text[0] == ' ' {
				text = text[1:]
			}
		}
		lines = append(lines, text)
	}

	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// =============================================================================
// TAXONOMY HELPERS
// =============================================================================

// astParamNames extracts parameter names from an AST field list.
func astParamNames(params *ast.FieldList) []string {
	if params == nil {
		return nil
	}
	var names []string
	for _, field := range params.List {
		for _, name := range field.Names {
			names = append(names, name.Name)
		}
	}
	return names
}

// astReturnTypes extracts return type strings from an AST field list.
func astReturnTypes(results *ast.FieldList) []string {
	if results == nil {
		return nil
	}
	var types []string
	for _, field := range results.List {
		t := typeToString(field.Type)
		if len(field.Names) > 0 {
			// Named return: use the names as tokens.
			for _, name := range field.Names {
				types = append(types, name.Name)
			}
		} else {
			types = append(types, t)
		}
	}
	return types
}

// =============================================================================
// PARAMETER EXTRACTION
// =============================================================================

// extractParams converts a function's parameter list to a slice of ParamDetail structs.
func extractParams(params *ast.FieldList, paramDocs map[string]string) []ParamDetail {
	var result []ParamDetail
	if params == nil {
		return result
	}

	for i, field := range params.List {
		isLast := i == len(params.List)-1
		_, isEllipsis := field.Type.(*ast.Ellipsis)
		variadic := isLast && isEllipsis

		typeStr := typeToString(field.Type)
		if variadic {
			typeStr = strings.TrimPrefix(typeStr, "...")
		}

		if len(field.Names) == 0 {
			result = append(result, ParamDetail{
				Type:     typeStr,
				Variadic: variadic,
			})
		} else {
			for _, name := range field.Names {
				doc := ""
				if paramDocs != nil {
					doc = paramDocs[name.Name]
				}

				result = append(result, ParamDetail{
					Name:     name.Name,
					Type:     typeStr,
					Variadic: variadic,
					Doc:      doc,
				})
			}
		}
	}

	return result
}

// =============================================================================
// RETURN VALUE EXTRACTION
// =============================================================================

// extractReturnString extracts the first string literal from a return statement.
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

// extractReturnStrings extracts string elements from a []string{...} return statement.
func extractReturnStrings(body *ast.BlockStmt) []string {
	if body == nil || len(body.List) == 0 {
		return nil
	}

	for _, stmt := range body.List {
		ret, ok := stmt.(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			continue
		}

		comp, ok := ret.Results[0].(*ast.CompositeLit)
		if !ok {
			continue
		}

		// Verify the type is []string.
		arr, ok := comp.Type.(*ast.ArrayType)
		if !ok || arr.Len != nil {
			continue
		}

		ident, ok := arr.Elt.(*ast.Ident)
		if !ok || ident.Name != "string" {
			continue
		}

		var result []string
		for _, elt := range comp.Elts {
			lit, ok := elt.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			result = append(result, strings.Trim(lit.Value, `"`))
		}

		return result
	}

	return nil
}

// =============================================================================
// JSON TAG PARSING
// =============================================================================

// parseJSONTag extracts the JSON field name and required status from a struct tag.
func parseJSONTag(tag string) (string, bool) {
	jsonRe := regexp.MustCompile(`json:"([^"]*)"`)
	match := jsonRe.FindStringSubmatch(tag)
	if match == nil {
		return "", false
	}

	parts := strings.Split(match[1], ",")
	name := parts[0]
	required := true

	for _, part := range parts[1:] {
		if part == "omitempty" {
			required = false
		}
	}

	return name, required
}

// =============================================================================
// METRICS ANALYSIS
// =============================================================================

// analyzeFileMetrics computes code metrics for a single Go file.
func analyzeFileMetrics(path string) (FileMetric, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return FileMetric{}, err
	}

	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, content, parser.ParseComments)
	if err != nil {
		return FileMetric{}, err
	}

	fm := FileMetric{Path: path}

	lines := strings.Split(string(content), "\n")
	fm.LOC = len(lines)

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			fm.Blanks++
		}
	}

	for _, cg := range node.Comments {
		for _, c := range cg.List {
			fm.Comments += strings.Count(c.Text, "\n") + 1
		}
	}

	fm.SLOC = fm.LOC - fm.Blanks - fm.Comments
	if fm.SLOC < 0 {
		fm.SLOC = 0
	}

	fm.Imports = len(node.Imports)

	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			if x.Recv != nil {
				fm.Methods++
			} else {
				fm.Functions++
			}

			if strings.HasPrefix(x.Name.Name, "Test") || strings.HasPrefix(x.Name.Name, "Benchmark") {
				fm.TestFunctions++
			}
		case *ast.GenDecl:
			switch x.Tok {
			case token.TYPE:
				for _, spec := range x.Specs {
					if ts, ok := spec.(*ast.TypeSpec); ok {
						fm.Types++

						switch ts.Type.(type) {
						case *ast.StructType:
							fm.Structs++
						case *ast.InterfaceType:
							fm.Interfaces++
						}
					}
				}
			case token.CONST:
				for _, spec := range x.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						fm.Constants += len(vs.Names)
					}
				}
			case token.VAR:
				for _, spec := range x.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						fm.Variables += len(vs.Names)
					}
				}
			}
		}

		return true
	})

	return fm, nil
}

// =============================================================================
// DEPENDENCY ANALYSIS
// =============================================================================

// analyzeFileDeps analyzes import dependencies for a single Go file.
func analyzeFileDeps(path, modulePath string) (FileDep, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
	if err != nil {
		return FileDep{}, err
	}

	fd := FileDep{
		Path:    path,
		Package: node.Name.Name,
	}

	for _, imp := range node.Imports {
		importPath := strings.Trim(imp.Path.Value, `"`)
		line := fset.Position(imp.Pos()).Line

		alias := ""
		if imp.Name != nil {
			alias = imp.Name.Name
		}

		fd.Imports = append(fd.Imports, ImportDetail{
			Path:  importPath,
			Alias: alias,
			Line:  line,
		})

		switch {
		case isStdlib(importPath):
			fd.StdlibDeps = append(fd.StdlibDeps, importPath)
		case modulePath != "" && strings.HasPrefix(importPath, modulePath):
			fd.InternalDeps = append(fd.InternalDeps, importPath)
		default:
			fd.ExternalDeps = append(fd.ExternalDeps, importPath)
		}
	}

	return fd, nil
}

// =============================================================================
// TEMPLATE FUNCTIONS
// =============================================================================

// renderFuncs provides general-purpose template functions for Render.
var renderFuncs = template.FuncMap{
	"camelToSnake": renderCamelToSnake,
	"lcFirst":      renderLCFirst,
	"join":         strings.Join,
}

// renderCamelToSnake converts CamelCase Go names to snake_case.
func renderCamelToSnake(s string) string {
	runes := []rune(s)
	var result []rune

	for i, r := range runes {
		if unicode.IsUpper(r) {
			if i > 0 {
				prev := runes[i-1]
				if unicode.IsLower(prev) {
					result = append(result, '_')
				} else if unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
					result = append(result, '_')
				}
			}
			result = append(result, unicode.ToLower(r))
		} else {
			result = append(result, r)
		}
	}

	return string(result)
}

// renderLCFirst lowercases the first character of a string.
func renderLCFirst(s string) string {
	if s == "" {
		return s
	}

	return strings.ToLower(s[:1]) + s[1:]
}

// =============================================================================
// SCOPE RANGE PARSING
// =============================================================================

// parseScopeRange parses a scope string into a line range.
//
// Supports "file" for the entire file and "lines:START-END" for a specific line range (1-indexed, inclusive).
func parseScopeRange(scope string, totalLines int) (int, int, error) {

	if scope == "file" || scope == "" {
		return 1, totalLines, nil
	}

	if strings.HasPrefix(scope, "lines:") {
		rangeStr := strings.TrimPrefix(scope, "lines:")
		parts := strings.SplitN(rangeStr, "-", 2)
		if len(parts) != 2 {
			return 0, 0, fmt.Errorf("invalid line range: %s", scope)
		}

		start, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid start line: %s", parts[0])
		}

		end, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid end line: %s", parts[1])
		}

		if start < 1 || end < start || end > totalLines {
			return 0, 0, fmt.Errorf("line range out of bounds: %d-%d (file has %d lines)", start, end, totalLines)
		}

		return start, end, nil
	}

	return 0, 0, fmt.Errorf("invalid scope: %s (expected \"file\" or \"lines:START-END\")", scope)
}

// =============================================================================
// SOURCE RESOLUTION
// =============================================================================

// goSource pairs a filename with its content.
type goSource struct {
	name    string
	content string
}

// resolveGoSource resolves a path-or-content string to a single goSource.
//
// If the string contains a newline it is treated as Go source content. Otherwise it is treated as a file path and read
// from disk.
func resolveGoSource(pathOrContent string) (goSource, error) {
	if strings.Contains(pathOrContent, "\n") {
		return goSource{name: "", content: pathOrContent}, nil
	}

	data, err := os.ReadFile(pathOrContent)
	if err != nil {
		return goSource{}, err
	}

	return goSource{name: pathOrContent, content: string(data)}, nil
}

// resolveGoSources resolves a path-or-content string to one or more goSource entries.
//
// Content strings produce a single entry. File paths produce one entry. Directory paths produce one entry per.go file.
func resolveGoSources(pathOrContent string) ([]goSource, error) {
	if strings.Contains(pathOrContent, "\n") {
		return []goSource{{name: "", content: pathOrContent}}, nil
	}

	files, err := collectGoFiles(pathOrContent)
	if err != nil {
		return nil, err
	}

	sources := make([]goSource, 0, len(files))
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		sources = append(sources, goSource{name: f, content: string(data)})
	}

	return sources, nil
}

// =============================================================================
// LINE WIDTH CHECKS
// =============================================================================

// checkLineWidth checks content for line-width violations.
//
// Reports over-long lines and under-filled comment lines where the next word would fit on the current line without
// exceeding width.
func checkLineWidth(content string, width int) []LineViolation {
	lines := strings.Split(content, "\n")
	var violations []LineViolation

	for i, line := range lines {
		if len(line) > width {
			violations = append(violations, LineViolation{
				Line:    i + 1,
				Message: fmt.Sprintf("line is %d columns (max %d)", len(line), width),
			})
		}
	}

	// Under-filled comment lines.
	for i := 0; i < len(lines)-1; i++ {
		curr := lines[i]
		next := lines[i+1]

		currBody, currOK := commentBodyText(curr)
		nextBody, nextOK := commentBodyText(next)
		if !currOK || !nextOK {
			continue
		}

		// Skip blank separator lines.
		if strings.TrimSpace(currBody) == "" || strings.TrimSpace(nextBody) == "" {
			continue
		}

		// Skip delineators.
		if isDelineatorLine(currBody) || isDelineatorLine(nextBody) {
			continue
		}

		// Skip SPDX/copyright.
		if strings.HasPrefix(currBody, "SPDX-") || strings.HasPrefix(currBody, "Copyright") {
			continue
		}

		// Skip indented code blocks (4+ spaces after //).
		if strings.HasPrefix(currBody, "    ") || strings.HasPrefix(nextBody, "    ") {
			continue
		}

		// Skip bullet items.
		ct := strings.TrimSpace(currBody)
		nt := strings.TrimSpace(nextBody)
		if strings.HasPrefix(ct, "- ") || strings.HasPrefix(nt, "- ") {
			continue
		}

		// Skip section headers and directives.
		if strings.HasSuffix(ct, ":") || strings.HasPrefix(ct, "+") {
			continue
		}
		if strings.HasSuffix(nt, ":") || strings.HasPrefix(nt, "+") || strings.HasPrefix(nt, "- ") {
			continue
		}

		// Check if first word of next line fits on current line.
		words := strings.Fields(nt)
		if len(words) == 0 {
			continue
		}
		firstWord := words[0]
		if len(curr)+1+len(firstWord) <= width {
			violations = append(violations, LineViolation{
				Line: i + 1,
				Message: fmt.Sprintf("under-filled comment ('%s' fits on previous line, %d columns available)",
					firstWord, width-len(curr)),
			})
		}
	}

	return violations
}

// commentBodyText extracts the text after // from a comment line.
func commentBodyText(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if strings.HasPrefix(trimmed, "// ") {
		return trimmed[3:], true
	}
	if trimmed == "//" {
		return "", true
	}
	if strings.HasPrefix(trimmed, "//") {
		return trimmed[2:], true
	}
	return "", false
}

// isDelineatorLine returns true if text is a delineator (3+ repeated =, -, ~, or *).
func isDelineatorLine(text string) bool {
	s := strings.TrimSpace(text)
	if len(s) < 3 {
		return false
	}
	first := s[0]
	if first != '=' && first != '-' && first != '~' && first != '*' {
		return false
	}
	for i := 1; i < len(s); i++ {
		if s[i] != first {
			return false
		}
	}
	return true
}
