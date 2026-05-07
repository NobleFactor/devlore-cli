// SPDX-License-Identifier: MIT
// Copyright Noble Factor. All rights reserved.

// Package goast provides Go AST operations as a Starlark receiver.
//
// +devlore:access=immediate
package goast

import (
	"bytes"
	"fmt"

	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	tmpl "text/template"

	"github.com/NobleFactor/devlore-cli/cmd/star/provider/goast/doctaxonomy"
	"github.com/NobleFactor/devlore-cli/pkg/op"
)

// Provider provides Go AST operations as a Starlark receiver.
//
// +devlore:access=immediate
type Provider struct {
	op.ProviderBase
	fileCache sync.Map // path → *parsedFile (AST cache)
}

// NewProvider creates a new Provider. Validates that all six comment styles have handlers in the merged config. Missing
// styles are repaired from defaults with a warning.
func NewProvider(ctx *op.RuntimeEnvironment) *Provider {
	p := &Provider{ProviderBase: op.NewProviderBase(ctx)}
	return p
}

// region EXPORTED METHODS

// region Behaviors

// Fallible actions

// Callable introspects a named function type declaration and returns its parameter list, return type, and doc comment
// (including directives).
func (p *Provider) Callable(path, name string) (CallableResult, error) {
	files, err := collectGoFiles(path)
	if err != nil {
		return CallableResult{}, fmt.Errorf("goast.callable: %w", err)
	}

	for _, file := range files {
		_, node, err := p.parseFile(file)
		if err != nil {
			continue
		}

		for _, decl := range node.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.TYPE {
				continue
			}

			for _, spec := range genDecl.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || ts.Name.Name != name {
					continue
				}

				ft, ok := ts.Type.(*ast.FuncType)
				if !ok {
					continue
				}

				// Build params list.
				var params []ParamDetail
				if ft.Params != nil {
					for _, field := range ft.Params.List {
						typeStr := typeToString(field.Type)
						if len(field.Names) == 0 {
							params = append(params, ParamDetail{
								Type: typeStr,
							})
						} else {
							for _, ident := range field.Names {
								params = append(params, ParamDetail{
									Name: ident.Name,
									Type: typeStr,
								})
							}
						}
					}
				}

				returns := returnTypeString(ft.Results)

				// Doc comment: prefer TypeSpec.Doc, fall back to GenDecl.Doc. Use
				// commentGroupRaw to preserve directive lines.
				var cg *ast.CommentGroup
				if ts.Doc != nil {
					cg = ts.Doc
				} else if genDecl.Doc != nil {
					cg = genDecl.Doc
				}

				return CallableResult{
					Name:    name,
					Doc:     commentGroupRaw(cg),
					Params:  params,
					Returns: returns,
				}, nil
			}
		}
	}

	return CallableResult{}, fmt.Errorf("goast.callable: function type %q not found in %s", name, path)
}

// Calls returns function/method calls within a scope.
//
// +devlore:defaults name=
func (p *Provider) Calls(scope, name string) ([]CallResult, error) {
	fileSet, body, err := p.findScopeBody(scope)
	if err != nil {
		return nil, fmt.Errorf("goast.calls: %w", err)
	}

	var result []CallResult
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		var funcName, qualifier, fullName string
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			funcName = fn.Name
			fullName = fn.Name
		case *ast.SelectorExpr:
			funcName = fn.Sel.Name
			if x, ok := fn.X.(*ast.Ident); ok {
				qualifier = x.Name
			}
			fullName = typeToString(call.Fun)
		}

		if funcName == "" {
			return true
		}

		if name != "" && funcName != name {
			return true
		}

		var args []CallArg
		for i, arg := range call.Args {
			strVal := ""
			if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				strVal = strings.Trim(lit.Value, `"`)
			}

			identName := ""
			switch a := arg.(type) {
			case *ast.Ident:
				identName = a.Name
			case *ast.SelectorExpr:
				identName = a.Sel.Name
			}

			args = append(args, CallArg{
				Position:    i,
				StringValue: strVal,
				IdentName:   identName,
			})
		}

		result = append(result, CallResult{
			Name:      funcName,
			Qualifier: qualifier,
			FullName:  fullName,
			Line:      fileSet.Position(call.Pos()).Line,
			Args:      args,
		})

		return true
	})

	return result, nil
}

// CheckLineWidth checks content for line-width violations.
//
// Reports over-long lines and under-filled comment lines (where the next word would fit on the current line without
// exceeding width).
func (p *Provider) CheckLineWidth(content string, width int) ([]LineViolation, error) {
	return checkLineWidth(content, width), nil
}

// Composites returns composite literals within a scope.
//
// +devlore:defaults typeName=
func (p *Provider) Composites(scope, typeName string) ([]CompositeResult, error) {
	fileSet, body, err := p.findScopeBody(scope)
	if err != nil {
		return nil, fmt.Errorf("goast.composites: %w", err)
	}

	var result []CompositeResult
	ast.Inspect(body, func(n ast.Node) bool {
		comp, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}

		tn := ""
		if comp.Type != nil {
			tn = typeToString(comp.Type)
		}

		if typeName != "" && tn != typeName {
			return true
		}

		fields := map[string]any{}
		for _, elt := range comp.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}

			key, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}

			switch v := kv.Value.(type) {
			case *ast.BasicLit:
				if v.Kind == token.STRING {
					fields[key.Name] = strings.Trim(v.Value, `"`)
				} else {
					fields[key.Name] = v.Value
				}
			case *ast.CompositeLit:
				var elems []string
				for _, elem := range v.Elts {
					if lit, ok := elem.(*ast.BasicLit); ok && lit.Kind == token.STRING {
						elems = append(elems, strings.Trim(lit.Value, `"`))
					} else {
						elems = append(elems, typeToString(elem))
					}
				}
				fields[key.Name] = elems
			default:
				fields[key.Name] = typeToString(kv.Value)
			}
		}

		result = append(result, CompositeResult{
			TypeName: tn,
			Line:     fileSet.Position(comp.Pos()).Line,
			Fields:   fields,
		})

		return true
	})

	return result, nil
}

// ConstGroups returns typed const groups from Go source files.
//
// +devlore:defaults typeName=
func (p *Provider) ConstGroups(path, typeName string) ([]ConstGroupResult, error) {
	files, err := collectGoFiles(path)
	if err != nil {
		return nil, fmt.Errorf("goast.const_groups: %w", err)
	}

	type constEntry struct {
		name  string
		value string
		line  int
	}
	type group struct {
		typeName string
		file     string
		consts   []constEntry
	}

	var groups []group
	for _, file := range files {
		fileSet, node, err := p.parseFile(file)
		if err != nil {
			continue
		}

		ast.Inspect(node, func(n ast.Node) bool {
			genDecl, ok := n.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.CONST {
				return true
			}

			var currentType string
			var currentConsts []constEntry

			for _, spec := range genDecl.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}

				if vs.Type != nil {
					if ident, ok := vs.Type.(*ast.Ident); ok {
						if currentType != "" && currentType != ident.Name && len(currentConsts) > 0 {
							if typeName == "" || typeName == currentType {
								groups = append(groups, group{typeName: currentType, file: filepath.Base(file), consts: currentConsts})
							}
							currentConsts = nil
						}
						currentType = ident.Name
					}
				}

				if currentType == "" {
					continue
				}

				for i, n := range vs.Names {
					var value string
					if i < len(vs.Values) {
						if lit, ok := vs.Values[i].(*ast.BasicLit); ok {
							value = strings.Trim(lit.Value, `"`)
						}
					}

					currentConsts = append(currentConsts, constEntry{
						name:  n.Name,
						value: value,
						line:  fileSet.Position(n.Pos()).Line,
					})
				}
			}

			if currentType != "" && len(currentConsts) > 0 {
				if typeName == "" || typeName == currentType {
					groups = append(groups, group{typeName: currentType, file: filepath.Base(file), consts: currentConsts})
				}
			}

			return true
		})
	}

	var result []ConstGroupResult
	for _, g := range groups {
		var consts []ConstDetail
		for _, c := range g.consts {
			consts = append(consts, ConstDetail{
				Name:  c.name,
				Value: c.value,
				Line:  c.line,
			})
		}

		result = append(result, ConstGroupResult{
			TypeName:  g.typeName,
			File:      g.file,
			Constants: consts,
		})
	}

	return result, nil
}

// Deps analyzes import dependencies for Go source files at the given path.
func (p *Provider) Deps(path string) (DepsResult, error) {
	files, err := collectGoFiles(path)
	if err != nil {
		return DepsResult{}, fmt.Errorf("goast.deps: %w", err)
	}

	modulePath := detectModulePath(path)

	var allFiles []FileDep
	allImports := make(map[string]bool)
	allInternal := make(map[string]bool)
	allExternal := make(map[string]bool)
	allStdlib := make(map[string]bool)

	for _, file := range files {
		fd, err := analyzeFileDeps(file, modulePath)
		if err != nil {
			continue
		}

		allFiles = append(allFiles, fd)

		for _, imp := range fd.Imports {
			allImports[imp.Path] = true
		}

		for _, dep := range fd.InternalDeps {
			allInternal[dep] = true
		}

		for _, dep := range fd.ExternalDeps {
			allExternal[dep] = true
		}

		for _, dep := range fd.StdlibDeps {
			allStdlib[dep] = true
		}
	}

	return DepsResult{
		Files:         allFiles,
		ModulePath:    modulePath,
		AllImports:    mapKeys(allImports),
		InternalDeps:  mapKeys(allInternal),
		ExternalDeps:  mapKeys(allExternal),
		StdlibDeps:    mapKeys(allStdlib),
		InternalCount: len(allInternal),
		ExternalCount: len(allExternal),
		StdlibCount:   len(allStdlib),
	}, nil
}

// Format formats Go source code via go/format.
func (p *Provider) Format(code string) (string, error) {
	formatted, err := format.Source([]byte(code))
	if err != nil {
		return "", fmt.Errorf("goast.format: %w", err)
	}

	return string(formatted), nil
}

// Funcs returns function declarations (non-method) from Go source.
//
// The path parameter accepts either a file/directory path or Go source content directly.
//
// +devlore:defaults name=
func (p *Provider) Funcs(path, name string) ([]FuncResult, error) {
	sources, err := resolveGoSources(path)
	if err != nil {
		return nil, fmt.Errorf("goast.funcs: %w", err)
	}

	var result []FuncResult
	for _, src := range sources {
		fileSet := token.NewFileSet()
		node, err := parser.ParseFile(fileSet, src.name, src.content, parser.ParseComments)
		if err != nil {
			continue
		}

		for _, decl := range node.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Name == nil {
				continue
			}

			if name != "" && fn.Name.Name != name {
				continue
			}

			returns := returnTypeString(fn.Type.Results)
			rawDoc := ""
			if fn.Doc != nil {
				rawDoc = commentGroupRaw(fn.Doc)
			}

			result = append(result, FuncResult{
				Name:    fn.Name.Name,
				Returns: returns,
				Params:  extractParams(fn.Type.Params, nil),
				File:    src.name,
				Line:    fileSet.Position(fn.Pos()).Line,
				Doc:     rawDoc,
			})
		}
	}

	return result, nil
}

// Methods returns method declarations from Go source.
//
// The path parameter accepts either a file/directory path or Go source content directly.
//
// +devlore:defaults name=,receiverType=,returns=
func (p *Provider) Methods(path, name, receiverType, returns string) ([]MethodResult, error) {
	sources, err := resolveGoSources(path)
	if err != nil {
		return nil, fmt.Errorf("goast.methods: %w", err)
	}

	var result []MethodResult
	for _, src := range sources {
		fileSet := token.NewFileSet()
		node, err := parser.ParseFile(fileSet, src.name, src.content, parser.ParseComments)
		if err != nil {
			continue
		}

		for _, decl := range node.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 || fn.Name == nil {
				continue
			}

			if name != "" && fn.Name.Name != name {
				continue
			}

			typeName := receiverTypeName(fn.Recv.List[0].Type)
			if receiverType != "" {
				if strings.HasPrefix(receiverType, "*") {
					if typeName != receiverType {
						continue
					}
				} else {
					if strings.TrimPrefix(typeName, "*") != receiverType {
						continue
					}
				}
			}

			retStr := returnTypeString(fn.Type.Results)
			if returns != "" && retStr != returns {
				continue
			}

			rawDoc := ""
			if fn.Doc != nil {
				rawDoc = commentGroupRaw(fn.Doc)
			}

			scope := src.name + "::" + typeName + "." + fn.Name.Name

			result = append(result, MethodResult{
				Name:         fn.Name.Name,
				ReceiverType: typeName,
				Returns:      retStr,
				Params:       extractParams(fn.Type.Params, nil),
				File:         src.name,
				Line:         fileSet.Position(fn.Pos()).Line,
				Doc:          rawDoc,
				Scope:        scope,
			})
		}
	}

	return result, nil
}

// Metrics computes code metrics for Go source files at the given path.
func (p *Provider) Metrics(path string) (MetricsResult, error) {
	files, err := collectGoFiles(path)
	if err != nil {
		return MetricsResult{}, fmt.Errorf("goast.metrics: %w", err)
	}

	var allFiles []FileMetric
	var totals FileMetric

	for _, file := range files {
		fm, err := analyzeFileMetrics(file)
		if err != nil {
			continue
		}

		allFiles = append(allFiles, fm)

		totals.LOC += fm.LOC
		totals.SLOC += fm.SLOC
		totals.Comments += fm.Comments
		totals.Blanks += fm.Blanks
		totals.Functions += fm.Functions
		totals.Methods += fm.Methods
		totals.Structs += fm.Structs
		totals.Interfaces += fm.Interfaces
		totals.Types += fm.Types
		totals.Constants += fm.Constants
		totals.Variables += fm.Variables
		totals.Imports += fm.Imports
		totals.TestFunctions += fm.TestFunctions
	}

	return MetricsResult{
		Files:              allFiles,
		FileCount:          len(files),
		TotalLOC:           totals.LOC,
		TotalSLOC:          totals.SLOC,
		TotalComments:      totals.Comments,
		TotalBlanks:        totals.Blanks,
		TotalFunctions:     totals.Functions,
		TotalMethods:       totals.Methods,
		TotalStructs:       totals.Structs,
		TotalInterfaces:    totals.Interfaces,
		TotalTypes:         totals.Types,
		TotalConstants:     totals.Constants,
		TotalVariables:     totals.Variables,
		TotalImports:       totals.Imports,
		TotalTestFunctions: totals.TestFunctions,
	}, nil
}

// RawString extracts the first backtick string literal from a scope.
func (p *Provider) RawString(scope string) (string, error) {
	_, body, err := p.findScopeBody(scope)
	if err != nil {
		return "", fmt.Errorf("goast.raw_string: %w", err)
	}

	var rawStr string
	ast.Inspect(body, func(n ast.Node) bool {
		if rawStr != "" {
			return false
		}

		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}

		if strings.HasPrefix(lit.Value, "`") {
			rawStr = strings.Trim(lit.Value, "`")
			return false
		}

		return true
	})

	return rawStr, nil
}

// Render executes a Go text/template against data and returns go/format-formatted Go source code.
func (p *Provider) Render(template string, data any) (string, error) {
	t, err := tmpl.New("render").Funcs(renderFuncs).Parse(template)
	if err != nil {
		return "", fmt.Errorf("goast.render: template parse: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("goast.render: template execution: %w", err)
	}

	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		return "", fmt.Errorf("goast.render: format error: %w\nraw output:\n%s", err, buf.String())
	}

	return string(formatted), nil
}

// ReturnString extracts the string literal from a return statement in a scope.
func (p *Provider) ReturnString(scope string) (string, error) {
	_, body, err := p.findScopeBody(scope)
	if err != nil {
		return "", fmt.Errorf("goast.return_string: %w", err)
	}

	return extractReturnString(body), nil
}

// ReturnStrings extracts string elements from a []string{...} return statement in a scope.
func (p *Provider) ReturnStrings(scope string) ([]string, error) {
	_, body, err := p.findScopeBody(scope)
	if err != nil {
		return nil, fmt.Errorf("goast.return_strings: %w", err)
	}

	return extractReturnStrings(body), nil
}

// LoadSourceFile reads a Go source file from disk and parses it into a semantic tree organized by declaration kind.
// The returned SourceFile supports iteration, name-based lookup, and style operations (Reformat, Save, CheckStyle).
// Styling config (schemas, spacing rules, line width) is read from context.
//
// Parameters:
//   - path: the file path to read.
//
// Returns:
//   - *SourceFile: the semantic tree.
//   - error: non-nil if the file cannot be read or parsed.
func (p *Provider) LoadSourceFile(path string) (*SourceFile, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("goast.load_source_file: %w", err)
	}
	sf, err := LoadSourceFile(string(content))
	if err != nil {
		return nil, fmt.Errorf("goast.load_source_file: %w", err)
	}
	sf.filename = path
	ctx := p.RuntimeEnvironment()
	ctx.Data["schema_registry"] = p.schemaRegistry()
	ctx.Data["spacing_rules"] = p.spacingRules()
	ctx.Data["line_width"] = p.configLineWidth()
	sf.ctx = ctx
	return sf, nil
}

// schemaRegistry returns the schema registry the provider operates on: defaults overlaid with any
// project-config-supplied schemas.
//
// Project config can supply zero, some, or all schema types under `lint.go_style.comment_schemas`.
// The resolution rule is "merge by (NodeType, Format) key, config wins":
//
//   - Schemas not present in config keep their default form.
//   - Schemas present in both default and config are replaced by the config form (full replacement
//     of the schema entry — config-side schemas are not deep-merged into default-side schemas;
//     authors who want partial overrides must spell out the full entry).
//   - Schemas present only in config are added.
//
// Returns the embedded defaults unchanged when no `lint.go_style.comment_schemas` config block is
// present at all.
func (p *Provider) schemaRegistry() *doctaxonomy.SchemaRegistry {
	reg := doctaxonomy.DefaultRegistry()
	overlay := p.configSchemas()
	if overlay == nil {
		return reg
	}
	for _, schema := range overlay.All() {
		reg.Register(schema)
	}
	return reg
}

// configSchemas attempts to build a SchemaRegistry from the config stored in the provider's context data.
func (p *Provider) configSchemas() *doctaxonomy.SchemaRegistry {
	cfgVal, ok := p.RuntimeEnvironment().Data["config"]
	if !ok || cfgVal == nil {
		return nil
	}

	cfg, ok := cfgVal.(configNavigator)
	if !ok {
		return nil
	}

	schemasVal := cfg.Navigate("lint.go_style.comment_schemas")
	if schemasVal == nil {
		return nil
	}

	return schemasFromConfig(schemasVal)
}

// spacingRules reads SpacingRules from config, falling back to defaults.
func (p *Provider) spacingRules() SpacingRules {
	cfgVal, ok := p.RuntimeEnvironment().Data["config"]
	if !ok || cfgVal == nil {
		return DefaultSpacingRules()
	}

	cfg, ok := cfgVal.(configNavigator)
	if !ok {
		return DefaultSpacingRules()
	}

	val := cfg.Navigate("lint.go_style.spacing_rules")
	if val == nil {
		return DefaultSpacingRules()
	}

	return spacingRulesFromConfig(val)
}

// spacingRulesFromConfig extracts SpacingRules from a config value using reflection.
func spacingRulesFromConfig(val interface{}) SpacingRules {
	rv := reflect.ValueOf(val)
	if rv.Kind() == reflect.Ptr {
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return DefaultSpacingRules()
	}

	rules := DefaultSpacingRules()
	if f := rv.FieldByName("AfterPackage"); f.IsValid() && f.CanInt() {
		rules.AfterPackage = int(f.Int())
	}
	if f := rv.FieldByName("AfterImports"); f.IsValid() && f.CanInt() {
		rules.AfterImports = int(f.Int())
	}
	if f := rv.FieldByName("BetweenFunctions"); f.IsValid() && f.CanInt() {
		rules.BetweenFunctions = int(f.Int())
	}
	if f := rv.FieldByName("BetweenMethods"); f.IsValid() && f.CanInt() {
		rules.BetweenMethods = int(f.Int())
	}
	if f := rv.FieldByName("BeforeTypeMethods"); f.IsValid() && f.CanInt() {
		rules.BeforeTypeMethods = int(f.Int())
	}
	if f := rv.FieldByName("AroundRegionMarkers"); f.IsValid() && f.CanInt() {
		rules.AroundRegionMarkers = int(f.Int())
	}
	if f := rv.FieldByName("AroundDelineators"); f.IsValid() && f.CanInt() {
		rules.AroundDelineators = int(f.Int())
	}
	return rules
}

// configLineWidth reads the line width from config, defaulting to 120.
func (p *Provider) configLineWidth() int {
	cfgVal, ok := p.RuntimeEnvironment().Data["config"]
	if !ok || cfgVal == nil {
		return 120
	}
	cfg, ok := cfgVal.(configNavigator)
	if !ok {
		return 120
	}
	val := cfg.Navigate("lint.go_style.line_width")
	if val == nil {
		return 120
	}
	rv := reflect.ValueOf(val)
	if rv.CanInt() {
		return int(rv.Int())
	}
	return 120
}

// SortDeclarations reorders function/method declarations within a scope of a Go file.
//
// Preserves doc comments and blank lines attached to each declaration. Returns the modified file content.
func (p *Provider) SortDeclarations(path, scope, order string) (string, error) {

	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("goast.sort_declarations: %w", err)
	}

	fileSet := token.NewFileSet()
	node, err := parser.ParseFile(fileSet, path, content, parser.ParseComments)
	if err != nil {
		return "", fmt.Errorf("goast.sort_declarations: %w", err)
	}

	startLine, endLine, err := parseScopeRange(scope, len(strings.Split(string(content), "\n")))
	if err != nil {
		return "", fmt.Errorf("goast.sort_declarations: %w", err)
	}

	lines := strings.Split(string(content), "\n")

	// Collect function declarations within the scope.
	type declBlock struct {
		name      string
		startLine int // 1-indexed, inclusive
		endLine   int // 1-indexed, inclusive
	}

	var blocks []declBlock
	for _, decl := range node.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name == nil {
			continue
		}

		dStart := fileSet.Position(fn.Pos()).Line
		dEnd := fileSet.Position(fn.End()).Line

		// Include doc comment.
		if fn.Doc != nil {
			docStart := fileSet.Position(fn.Doc.Pos()).Line
			if docStart < dStart {
				dStart = docStart
			}
		}

		// Skip declarations outside the scope.
		if dStart < startLine || dEnd > endLine {
			continue
		}

		blocks = append(blocks, declBlock{
			name:      fn.Name.Name,
			startLine: dStart,
			endLine:   dEnd,
		})
	}

	if len(blocks) <= 1 {
		return string(content), nil
	}

	// Record the overall range of all blocks.
	overallStart := blocks[0].startLine
	overallEnd := blocks[0].endLine
	for _, b := range blocks {
		if b.startLine < overallStart {
			overallStart = b.startLine
		}
		if b.endLine > overallEnd {
			overallEnd = b.endLine
		}
	}

	// Extract text for each block before sorting.
	blockTexts := make(map[string]string, len(blocks))
	for _, b := range blocks {
		text := strings.Join(lines[b.startLine-1:b.endLine], "\n")
		blockTexts[b.name] = strings.TrimRight(text, " \t\n")
	}

	// Sort blocks.
	switch order {
	case "alphabetical", "":
		sort.Slice(blocks, func(i, j int) bool {
			return blocks[i].name < blocks[j].name
		})
	default:
		return "", fmt.Errorf("goast.sort_declarations: unknown order: %s", order)
	}

	// Build sorted content.
	var sortedParts []string
	for _, b := range blocks {
		sortedParts = append(sortedParts, blockTexts[b.name])
	}
	replacement := strings.Join(sortedParts, "\n\n")

	// Replace the overall range in the file.
	before := lines[:overallStart-1]
	after := lines[overallEnd:]

	var resultLines []string
	resultLines = append(resultLines, before...)
	resultLines = append(resultLines, strings.Split(replacement, "\n")...)
	resultLines = append(resultLines, after...)

	return strings.Join(resultLines, "\n"), nil
}

// Structs returns struct definitions from Go source files.
func (p *Provider) Structs(path string) ([]StructResult, error) {
	files, err := collectGoFiles(path)
	if err != nil {
		return nil, fmt.Errorf("goast.structs: %w", err)
	}

	var result []StructResult
	for _, file := range files {
		fileSet, node, err := p.parseFile(file)
		if err != nil {
			continue
		}

		ast.Inspect(node, func(n ast.Node) bool {
			genDecl, ok := n.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.TYPE {
				return true
			}

			for _, spec := range genDecl.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}

				st, ok := ts.Type.(*ast.StructType)
				if !ok {
					continue
				}

				var fields []FieldDetail
				for _, field := range st.Fields.List {
					if len(field.Names) == 0 {
						// Embedded field.
						fieldType := typeToString(field.Type)
						fields = append(fields, FieldDetail{
							Name:     fieldType,
							Type:     fieldType,
							Embedded: true,
						})
						continue
					}

					jsonName := ""
					required := false
					if field.Tag != nil {
						tag := strings.Trim(field.Tag.Value, "`")
						jsonName, required = parseJSONTag(tag)
					}

					if jsonName == "-" {
						continue
					}

					desc := ""
					if field.Comment != nil {
						desc = strings.TrimSpace(field.Comment.Text())
					} else if field.Doc != nil {
						desc = strings.TrimSpace(field.Doc.Text())
					}

					fieldType := typeToString(field.Type)
					for _, ident := range field.Names {
						fields = append(fields, FieldDetail{
							Name:        ident.Name,
							JSONName:    jsonName,
							Type:        fieldType,
							Required:    required,
							Description: desc,
						})
					}
				}

				// Parse type doc with taxonomy.
				result = append(result, StructResult{
					Name:   ts.Name.Name,
					File:   filepath.Base(file),
					Line:   fileSet.Position(ts.Pos()).Line,
					Fields: fields,
				})
			}

			return true
		})
	}

	return result, nil
}

// TypeDoc returns the doc comment for a named type declaration.
//
// +devlore:defaults name=
func (p *Provider) TypeDoc(path, name string) (string, error) {
	if name == "" {
		name = "Provider"
	}

	files, err := collectGoFiles(path)
	if err != nil {
		return "", fmt.Errorf("goast.type_doc: %w", err)
	}

	for _, file := range files {
		_, node, err := p.parseFile(file)
		if err != nil {
			continue
		}

		for _, decl := range node.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.TYPE {
				continue
			}

			for _, spec := range genDecl.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || ts.Name.Name != name {
					continue
				}

				var cg *ast.CommentGroup
				if ts.Doc != nil {
					cg = ts.Doc
				} else if genDecl.Doc != nil {
					cg = genDecl.Doc
				}

				return commentGroupRaw(cg), nil
			}
		}
	}

	return "", nil
}

// endregion

// endregion

// =============================================================================
// UNEXPORTED HELPERS
// =============================================================================

// mapKeys returns the keys of a map as a string slice.
func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	return keys
}
